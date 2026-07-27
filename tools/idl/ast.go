// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package idl

// TypeKind identifies a primitive or compound IDL type.
type TypeKind int

const (
	KindBoolean   TypeKind = iota // IDL: boolean           → Go: bool
	KindOctet                     // IDL: octet             → Go: uint8
	KindShort                     // IDL: short             → Go: int16
	KindUShort                    // IDL: unsigned short    → Go: uint16
	KindLong                      // IDL: long              → Go: int32
	KindULong                     // IDL: unsigned long     → Go: uint32
	KindLongLong                  // IDL: long long         → Go: int64
	KindULongLong                 // IDL: unsigned long long → Go: uint64
	KindFloat                     // IDL: float             → Go: float32
	KindDouble                    // IDL: double            → Go: float64
	KindString                    // IDL: string            → Go: string
	KindSequence                  // IDL: sequence<T>       → Go: []ElemType
	KindStruct                    // IDL: struct T (cross-reference by name)
	KindArray                     // IDL: T name[N]         → Go: [N]T
	KindEnum                      // IDL: enum E { A, B }   → Go: type E int32
)

// TypeSpec describes the type of a field.
type TypeSpec struct {
	Kind      TypeKind
	ElemType  *TypeSpec // non-nil for KindSequence and KindArray
	ArraySize int       // non-zero for KindArray: number of elements
	RefName   string    // non-empty for KindStruct and KindEnum: IDL name (may be Module::Name)
}

// Field is one field within an IDL struct.
type Field struct {
	Name string   // IDL identifier (original case)
	Type TypeSpec // field type
	Key  bool     // true when the field carries an @key annotation
}

// Struct is an IDL struct declaration.
type Struct struct {
	Name   string  // struct name
	Fields []Field // ordered fields
}

// Enum is an IDL enum declaration.
type Enum struct {
	Name   string   // enum name
	Values []string // enumerator names in declaration order
}

// Typedef is an IDL typedef declaration: typedef BaseType AliasName.
type Typedef struct {
	Name string   // alias name
	Type TypeSpec // underlying type
}

// Module is the top-level container returned by ParseString / ParseFile.
// It may contain both structs defined directly at module scope and sub-modules.
type Module struct {
	Name     string    // module name (empty for file-level scope)
	Typedefs []Typedef // typedef aliases defined in this module
	Enums    []Enum    // enums defined in this module
	Structs  []Struct  // structs defined in this module
	Modules  []*Module // nested sub-modules
}
