// Copyright 2011 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// This file sets up the universe scope and the unsafe package.

package types

import (
	"github.com/tdakkota/go2go/golib/constant"
	"github.com/tdakkota/go2go/golib/token"
	"strings"
)

// The Universe scope contains all predeclared objects of Go.
// It is the outermost scope of any chain of nested scopes.
var Universe *Scope

// The Unsafe package is the package returned by an importer
// for the import path "unsafe".
var Unsafe *Package

var (
	universeIota *Const
	universeByte *Basic // uint8 alias, but has name "byte"
	universeRune *Basic // int32 alias, but has name "rune"
)

// Typ contains the predeclared *Basic types indexed by their
// corresponding BasicKind.
//
// The *Basic type for Typ[Byte] will have the name "uint8".
// Use Universe.Lookup("byte").Type() to obtain the specific
// alias basic type named "byte" (and analogous for "rune").
var Typ = []*Basic{
	Invalid: {Invalid, 0, "invalid type", aType{}},

	Bool:          {Bool, IsBoolean, "bool", aType{}},
	Int:           {Int, IsInteger, "int", aType{}},
	Int8:          {Int8, IsInteger, "int8", aType{}},
	Int16:         {Int16, IsInteger, "int16", aType{}},
	Int32:         {Int32, IsInteger, "int32", aType{}},
	Int64:         {Int64, IsInteger, "int64", aType{}},
	Uint:          {Uint, IsInteger | IsUnsigned, "uint", aType{}},
	Uint8:         {Uint8, IsInteger | IsUnsigned, "uint8", aType{}},
	Uint16:        {Uint16, IsInteger | IsUnsigned, "uint16", aType{}},
	Uint32:        {Uint32, IsInteger | IsUnsigned, "uint32", aType{}},
	Uint64:        {Uint64, IsInteger | IsUnsigned, "uint64", aType{}},
	Uintptr:       {Uintptr, IsInteger | IsUnsigned, "uintptr", aType{}},
	Float32:       {Float32, IsFloat, "float32", aType{}},
	Float64:       {Float64, IsFloat, "float64", aType{}},
	Complex64:     {Complex64, IsComplex, "complex64", aType{}},
	Complex128:    {Complex128, IsComplex, "complex128", aType{}},
	String:        {String, IsString, "string", aType{}},
	UnsafePointer: {UnsafePointer, 0, "Pointer", aType{}},

	UntypedBool:    {UntypedBool, IsBoolean | IsUntyped, "untyped bool", aType{}},
	UntypedInt:     {UntypedInt, IsInteger | IsUntyped, "untyped int", aType{}},
	UntypedRune:    {UntypedRune, IsInteger | IsUntyped, "untyped rune", aType{}},
	UntypedFloat:   {UntypedFloat, IsFloat | IsUntyped, "untyped float", aType{}},
	UntypedComplex: {UntypedComplex, IsComplex | IsUntyped, "untyped complex", aType{}},
	UntypedString:  {UntypedString, IsString | IsUntyped, "untyped string", aType{}},
	UntypedNil:     {UntypedNil, IsUntyped, "untyped nil", aType{}},
}

var aliases = [...]*Basic{
	{Byte, IsInteger | IsUnsigned, "byte", aType{}},
	{Rune, IsInteger, "rune", aType{}},
}

func defPredeclaredTypes() {
	for _, t := range Typ {
		def(NewTypeName(token.NoPos, nil, t.name, t))
	}
	for _, t := range aliases {
		def(NewTypeName(token.NoPos, nil, t.name, t))
	}

	// Error has a nil package in its qualified name since it is in no package
	res := NewVar(token.NoPos, nil, "", Typ[String])
	sig := &Signature{results: NewTuple(res)}
	err := NewFunc(token.NoPos, nil, "Error", sig)
	typ := &Named{underlying: NewInterfaceType([]*Func{err}, nil).Complete()}
	sig.recv = NewVar(token.NoPos, nil, "", typ)
	def(NewTypeName(token.NoPos, nil, "error", typ))
}

var predeclaredConsts = [...]struct {
	name string
	kind BasicKind
	val  constant.Value
}{
	{"true", UntypedBool, constant.MakeBool(true)},
	{"false", UntypedBool, constant.MakeBool(false)},
	{"iota", UntypedInt, constant.MakeInt64(0)},
}

func defPredeclaredConsts() {
	for _, c := range predeclaredConsts {
		def(NewConst(token.NoPos, nil, c.name, Typ[c.kind], c.val))
	}
}

func defPredeclaredNil() {
	def(&Nil{object{name: "nil", typ: Typ[UntypedNil], color_: black}})
}

// A builtinId is the id of a builtin function.
type builtinId int

const (
	// universe scope
	_Append builtinId = iota
	_Cap
	_Close
	_Complex
	_Copy
	_Delete
	_Imag
	_Len
	_Make
	_New
	_Panic
	_Print
	_Println
	_Real
	_Recover

	// package unsafe
	_Alignof
	_Offsetof
	_Sizeof

	// testing support
	_Assert
	_Trace
)

var predeclaredFuncs = [...]struct {
	name     string
	nargs    int
	variadic bool
	kind     exprKind
}{
	_Append:  {"append", 1, true, expression},
	_Cap:     {"cap", 1, false, expression},
	_Close:   {"close", 1, false, statement},
	_Complex: {"complex", 2, false, expression},
	_Copy:    {"copy", 2, false, statement},
	_Delete:  {"delete", 2, false, statement},
	_Imag:    {"imag", 1, false, expression},
	_Len:     {"len", 1, false, expression},
	_Make:    {"make", 1, true, expression},
	_New:     {"new", 1, false, expression},
	_Panic:   {"panic", 1, false, statement},
	_Print:   {"print", 0, true, statement},
	_Println: {"println", 0, true, statement},
	_Real:    {"real", 1, false, expression},
	_Recover: {"recover", 0, false, statement},

	_Alignof:  {"Alignof", 1, false, expression},
	_Offsetof: {"Offsetof", 1, false, expression},
	_Sizeof:   {"Sizeof", 1, false, expression},

	_Assert: {"assert", 1, false, statement},
	_Trace:  {"trace", 0, true, statement},
}

func defPredeclaredFuncs() {
	for i := range predeclaredFuncs {
		id := builtinId(i)
		if id == _Assert || id == _Trace {
			continue // only define these in testing environment
		}
		def(newBuiltin(id))
	}
}

// DefPredeclaredTestFuncs defines the assert and trace built-ins.
// These built-ins are intended for debugging and testing of this
// package only.
func DefPredeclaredTestFuncs() {
	if Universe.Lookup("assert") != nil {
		return // already defined
	}
	def(newBuiltin(_Assert))
	def(newBuiltin(_Trace))
}

func defPredeclaredContracts() {
	// The "comparable" contract can be envisioned as defined like
	//
	// contract comparable(T) {
	//         == (T) untyped bool
	//         != (T) untyped bool
	// }
	//
	// == and != cannot be user-declared but we can declare
	// a magic method == and check for its presence when needed.
	// (A simpler approach that simply looks for a magic type
	// bound interface is problematic: comparable might be embedded,
	// which in turn leads to the embedding of the magic type bound
	// interface and then we cannot easily look for that interface
	// anymore.)

	// Define interface { ==() }. We don't care about the signature
	// for == so leave it empty except for the receiver, which is
	// set up later to match the usual interface method assumptions.
	sig := new(Signature)
	eql := NewFunc(token.NoPos, nil, "==", sig)
	iface := NewInterfaceType([]*Func{eql}, nil).Complete()

	// The interface is parameterized with a single
	// type parameter to match the comparable contract.
	pname := NewTypeName(token.NoPos, nil, "T", nil)
	pname.typ = &TypeParam{0, pname, 0, &emptyInterface, aType{}}

	// The type bound interface needs a name so we can attach the
	// type parameter and to match the usual set up of contracts.
	iname := NewTypeName(token.NoPos, nil, "comparable_bound", nil)
	named := NewNamed(iname, iface, nil)
	named.tparams = []*TypeName{pname}
	sig.recv = NewVar(token.NoPos, nil, "", named) // complete == signature

	// set up the contract
	obj := NewContract(token.NoPos, nil, "comparable")
	obj.typ = new(contractType) // mark contract as fully set up
	obj.color_ = black
	obj.TParams = named.tparams
	obj.Bounds = []*Named{named}

	def(obj)
}

func init() {
	Universe = NewScope(nil, token.NoPos, token.NoPos, "universe")
	Unsafe = NewPackage("unsafe", "unsafe")
	Unsafe.complete = true

	defPredeclaredTypes()
	defPredeclaredConsts()
	defPredeclaredNil()
	defPredeclaredFuncs()
	defPredeclaredContracts()

	universeIota = Universe.Lookup("iota").(*Const)
	universeByte = Universe.Lookup("byte").(*TypeName).typ.(*Basic)
	universeRune = Universe.Lookup("rune").(*TypeName).typ.(*Basic)
}

// Objects with names containing blanks are internal and not entered into
// a scope. Objects with exported names are inserted in the unsafe package
// scope; other objects are inserted in the universe scope.
//
func def(obj Object) {
	assert(obj.color() == black)
	name := obj.Name()
	if strings.Contains(name, " ") {
		return // nothing to do
	}
	// fix Obj link for named types
	if typ := obj.Type().Named(); typ != nil {
		typ.obj = obj.(*TypeName)
	}
	// exported identifiers go into package unsafe
	scope := Universe
	if obj.Exported() {
		scope = Unsafe.scope
		// set Pkg field
		switch obj := obj.(type) {
		case *TypeName:
			obj.pkg = Unsafe
		case *Builtin:
			obj.pkg = Unsafe
		default:
			unreachable()
		}
	}
	if scope.Insert(obj) != nil {
		panic("internal error: double declaration")
	}
}
