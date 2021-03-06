// Copyright 2018 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// This file implements instantiation of generic types
// through substitution of type parameters by actual
// types.

package types

import (
	"bytes"
	"fmt"
	"github.com/tdakkota/go2go/golib/token"
)

type substMap struct {
	// The targs field is currently needed for *Named type substitution.
	// TODO(gri) rewrite that code, get rid of this field, and make this
	//           struct just the map (proj)
	targs []Type
	proj  map[*TypeParam]Type
}

func makeSubstMap(tpars []*TypeName, targs []Type) *substMap {
	assert(len(tpars) == len(targs))
	proj := make(map[*TypeParam]Type, len(tpars))
	for i, tpar := range tpars {
		// We must expand type arguments otherwise *Instance
		// types end up as components in composite types.
		// TODO(gri) explain why this causes problems, if it does
		targ := expand(targs[i])
		targs[i] = targ
		assert(targ != nil)
		proj[tpar.typ.(*TypeParam)] = targs[i]
	}
	return &substMap{targs, proj}
}

func (m *substMap) String() string {
	return fmt.Sprintf("%s", m.proj)
}

func (m *substMap) empty() bool {
	return len(m.proj) == 0
}

func (m *substMap) lookup(typ *TypeParam) Type {
	if t := m.proj[typ]; t != nil {
		return t
	}
	return typ
}

func (check *Checker) instantiate(pos token.Pos, typ Type, targs []Type, poslist []token.Pos) (res Type) {
	if check.conf.Trace {
		check.trace(pos, "-- instantiating %s with %s", typ, typeListString(targs))
		check.indent++
		defer func() {
			check.indent--
			var under Type
			if res != nil {
				under = res.Under()
			}
			check.trace(pos, "=> %s (under = %s)", res, under)
		}()
	}

	assert(poslist == nil || len(poslist) == len(targs))

	// TODO(gri) What is better here: work with TypeParams, or work with TypeNames?
	var tparams []*TypeName
	switch t := typ.(type) {
	case *Named:
		tparams = t.tparams
	case *Signature:
		tparams = t.tparams
		defer func() {
			// If we had an unexpected failure somewhere don't
			// panic below when asserting res.(*Signature).
			if res == nil {
				return
			}
			// If the signature doesn't use its type parameters, subst
			// will not make a copy. In that case, make a copy now (so
			// we can set tparams to nil w/o causing side-effects).
			if t == res {
				copy := *t
				res = &copy
			}
			// After instantiating a generic signature, it is not generic
			// anymore; we need to set tparams to nil.
			res.(*Signature).tparams = nil
		}()

	default:
		check.dump("%v: cannot instantiate %v", pos, typ)
		unreachable() // only defined types and (defined) functions can be generic

	}

	// the number of supplied types must match the number of type parameters
	if len(targs) != len(tparams) {
		// TODO(gri) provide better error message
		check.errorf(pos, "got %d arguments but %d type parameters", len(targs), len(tparams))
		return Typ[Invalid]
	}

	if len(tparams) == 0 {
		return typ // nothing to do (minor optimization)
	}

	smap := makeSubstMap(tparams, targs)

	// check bounds
	for i, tname := range tparams {
		tpar := tname.typ.(*TypeParam)
		iface := tpar.Bound()
		if iface.Empty() {
			continue // no type bound
		}

		targ := targs[i]

		// best position for error reporting
		pos := pos
		if i < len(poslist) {
			pos = poslist[i]
		}

		// The type parameter bound is parameterized with the same type parameters
		// as the instantiated type; before we can use it for bounds checking we
		// need to instantiate it with the type arguments with which we instantiate
		// the parameterized type.
		iface = check.subst(pos, iface, smap).(*Interface)

		// targ must implement iface (methods)
		//
		// Assume targ is addressable, per the draft design: "In a generic function
		// body all method calls will be pointer method calls. If necessary, the
		// function body will insert temporary variables, not seen by the user, in
		// order to get an addressable variable to use to call the method."
		//
		// TODO(gri) Instead of the addressable (= true) flag, could we encode the
		// same information by making targ a pointer type (and then get rid of the
		// need for that extra flag)?
		if m, _ := check.missingMethod(targ, true, iface, true); m != nil {
			// TODO(gri) needs to print updated name to avoid major confusion in error message!
			//           (print warning for now)
			// check.softErrorf(pos, "%s does not satisfy %s (warning: name not updated) = %s (missing method %s)", targ, tpar.bound, iface, m)
			if m.name == "==" {
				// We don't want to report "missing method ==".
				check.softErrorf(pos, "%s does not satisfy comparable", targ)
			} else {
				check.softErrorf(pos, "%s does not satisfy %s (missing method %s)", targ, tpar.bound, m.name)
			}
			break
		}

		// targ's underlying type must also be one of the interface types listed, if any
		if len(iface.allTypes) == 0 {
			break // nothing to do
		}
		// len(iface.allTypes) > 0

		// If targ is itself a type parameter, each of its possible types, but at least one, must be in the
		// list of iface types (i.e., the targ type list must be a non-empty subset of the iface types).
		if targ := targ.TypeParam(); targ != nil {
			targBound := targ.Bound()
			if len(targBound.allTypes) == 0 {
				check.softErrorf(pos, "%s does not satisfy %s (%s has no type constraints)", targ, tpar.bound, targ)
				break
			}
			for _, t := range targBound.allTypes {
				if !iface.includes(t.Under()) {
					// TODO(gri) match this error message with the one below (or vice versa)
					check.softErrorf(pos, "%s does not satisfy %s (%s type constraint %s not found in %s)", targ, tpar.bound, targ, t, iface.allTypes)
					break
				}
			}
			break
		}

		// Otherwise, targ's underlying type must also be one of the interface types listed, if any.
		// TODO(gri) must it be the underlying type, or should it just be the type? (spec question)
		if !iface.includes(targ.Under()) {
			check.softErrorf(pos, "%s does not satisfy %s (%s not found in %s)", targ, tpar.bound, targ.Under(), iface.allTypes)
			break
		}
	}

	return check.subst(pos, typ, smap)
}

// subst returns the type typ with its type parameters tpars replaced by
// the corresponding type arguments targs, recursively.
func (check *Checker) subst(pos token.Pos, typ Type, smap *substMap) Type {
	if smap.empty() {
		return typ
	}

	// common cases
	switch t := typ.(type) {
	case *Basic:
		return typ // nothing to do
	case *TypeParam:
		return smap.lookup(t)
	}

	// general case
	subst := subster{check, pos, make(map[Type]Type), smap}
	return subst.typ(typ)
}

type subster struct {
	check *Checker
	pos   token.Pos
	cache map[Type]Type
	smap  *substMap
}

func (subst *subster) typ(typ Type) Type {
	switch t := typ.(type) {
	case nil:
		panic("nil typ")

	case *Basic:
		// nothing to do

	case *Array:
		elem := subst.typ(t.elem)
		if elem != t.elem {
			return &Array{len: t.len, elem: elem}
		}

	case *Slice:
		elem := subst.typ(t.elem)
		if elem != t.elem {
			return &Slice{elem: elem}
		}

	case *Struct:
		if fields, copied := subst.varList(t.fields); copied {
			return &Struct{fields: fields, tags: t.tags}
		}

	case *Pointer:
		base := subst.typ(t.base)
		if base != t.base {
			return &Pointer{base: base}
		}

	case *Tuple:
		return subst.tuple(t)

	case *Signature:
		// TODO(gri) rethink the recv situation with respect to methods on parameterized types
		// recv := subst.var_(t.recv) // TODO(gri) this causes a stack overflow - explain
		recv := t.recv
		params := subst.tuple(t.params)
		results := subst.tuple(t.results)
		if recv != t.recv || params != t.params || results != t.results {
			return &Signature{
				rparams:  t.rparams,
				tparams:  t.tparams,
				scope:    t.scope,
				recv:     recv,
				params:   params,
				results:  results,
				variadic: t.variadic,
			}
		}

	case *Interface:
		methods, mcopied := subst.funcList(t.methods)
		types, tcopied := subst.typeList(t.types)
		embeddeds, ecopied := subst.typeList(t.embeddeds)
		if mcopied || tcopied || ecopied {
			iface := &Interface{methods: methods, types: types, embeddeds: embeddeds}
			subst.check.posMap[iface] = subst.check.posMap[t] // satisfy completeInterface requirement
			subst.check.completeInterface(token.NoPos, iface)
			return iface
		}

	case *Map:
		key := subst.typ(t.key)
		elem := subst.typ(t.elem)
		if key != t.key || elem != t.elem {
			return &Map{key: key, elem: elem}
		}

	case *Chan:
		elem := subst.typ(t.elem)
		if elem != t.elem {
			return &Chan{dir: t.dir, elem: elem}
		}

	case *Named:
		subst.check.indent++
		defer func() {
			subst.check.indent--
		}()
		dump := func(format string, args ...interface{}) {
			if subst.check.conf.Trace {
				subst.check.trace(subst.pos, format, args...)
			}
		}

		assert(t.underlying != nil)

		if t.tparams == nil {
			dump(">>> %s is not parameterized", t)
			return t // type is not parameterized
		}

		var new_targs []Type

		if len(t.targs) > 0 {

			// already instantiated
			dump(">>> %s already instantiated", t)
			assert(len(t.targs) == len(t.tparams))
			// For each (existing) type argument targ, determine if it needs
			// to be substituted; i.e., if it is or contains a type parameter
			// that has a type argument for it.
			for i, targ := range t.targs {
				dump(">>> %d targ = %s", i, targ)
				new_targ := subst.typ(targ)
				if new_targ != targ {
					dump(">>> substituted %d targ %s => %s", i, targ, new_targ)
					if new_targs == nil {
						new_targs = make([]Type, len(t.tparams))
						copy(new_targs, t.targs)
					}
					new_targs[i] = new_targ
				}
			}

			if new_targs == nil {
				dump(">>> nothing to substitute in %s", t)
				return t // nothing to substitute
			}

		} else {

			// not yet instantiated
			dump(">>> first instantiation of %s", t)
			new_targs = subst.smap.targs

		}

		// before creating a new named type, check if we have this one already
		h := instantiatedHash(t, new_targs)
		dump(">>> new type hash: %s", h)
		if named, found := subst.check.typMap[h]; found {
			dump(">>> found %s", named)
			subst.cache[t] = named
			return named
		}

		// create a new named type and populate caches to avoid endless recursion
		tname := NewTypeName(subst.pos, t.obj.pkg, t.obj.name, nil)
		named := subst.check.NewNamed(tname, t.underlying, t.methods) // method signatures are updated lazily
		named.tparams = t.tparams                                     // new type is still parameterized
		named.targs = new_targs
		subst.check.typMap[h] = named
		subst.cache[t] = named

		// do the substitution
		dump(">>> subst %s with %s (new: %s)", t.underlying, subst.smap, new_targs)
		named.underlying = subst.typ(t.underlying)
		named.orig = named.underlying // for cycle detection (Checker.validType)

		return named

	case *TypeParam:
		return subst.smap.lookup(t)

	case *instance:
		// TODO(gri) can we avoid the expansion here and just substitute the type parameters?
		return subst.typ(t.expand())

	default:
		panic("unimplemented")
	}

	return typ
}

// TODO(gri) Eventually, this should be more sophisticated.
//           It won't work correctly for locally declared types.
func instantiatedHash(typ *Named, targs []Type) string {
	var buf bytes.Buffer
	writeTypeName(&buf, typ.obj, nil)
	buf.WriteByte('(')
	writeTypeList(&buf, targs, nil, nil)
	buf.WriteByte(')')
	return buf.String()
}

func typeListString(list []Type) string {
	var buf bytes.Buffer
	writeTypeList(&buf, list, nil, nil)
	return buf.String()
}

func (subst *subster) var_(v *Var) *Var {
	if v != nil {
		if typ := subst.typ(v.typ); typ != v.typ {
			copy := *v
			copy.typ = typ
			return &copy
		}
	}
	return v
}

func (subst *subster) tuple(t *Tuple) *Tuple {
	if t != nil {
		if vars, copied := subst.varList(t.vars); copied {
			return &Tuple{vars: vars}
		}
	}
	return t
}

func (subst *subster) varList(in []*Var) (out []*Var, copied bool) {
	out = in
	for i, v := range in {
		if w := subst.var_(v); w != v {
			if !copied {
				// first variable that got substituted => allocate new out slice
				// and copy all variables
				new := make([]*Var, len(in))
				copy(new, out)
				out = new
				copied = true
			}
			out[i] = w
		}
	}
	return
}

func (subst *subster) func_(f *Func) *Func {
	if f != nil {
		if typ := subst.typ(f.typ); typ != f.typ {
			copy := *f
			copy.typ = typ
			return &copy
		}
	}
	return f
}

func (subst *subster) funcList(in []*Func) (out []*Func, copied bool) {
	out = in
	for i, f := range in {
		if g := subst.func_(f); g != f {
			if !copied {
				// first function that got substituted => allocate new out slice
				// and copy all functions
				new := make([]*Func, len(in))
				copy(new, out)
				out = new
				copied = true
			}
			out[i] = g
		}
	}
	return
}

func (subst *subster) typeList(in []Type) (out []Type, copied bool) {
	out = in
	for i, t := range in {
		if u := subst.typ(t); u != t {
			if !copied {
				// first function that got substituted => allocate new out slice
				// and copy all functions
				new := make([]Type, len(in))
				copy(new, out)
				out = new
				copied = true
			}
			out[i] = u
		}
	}
	return
}
