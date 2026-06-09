// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Package idl provides an OMG IDL parser and Go source code generator for
// go-DDS (Milestone M2 — Developer Experience).
//
// ParseFile and ParseString parse a subset of OMG IDL 4.x:
//   - module declarations (may be nested)
//   - struct declarations with nested struct references
//   - enum declarations
//   - basic types: boolean, octet, short, unsigned short, long, unsigned long,
//     long long, unsigned long long, float, double, string
//   - sequence types: sequence<T> and bounded sequence<T, N>
//   - bounded strings: string<N>
//   - fixed-size arrays: T name[N]
//   - qualified type names: Module::TypeName
//
// Generate produces a Go source file containing:
//   - A Go struct for every IDL struct (fields exported, names converted to Go style)
//   - A codec type (e.g., SpeedCodec) implementing dds.Codec[T] using CDR/XCDR1 encoding
//   - A package declaration derived from the top-level module name (or "idlgen" if absent)
//
// # Usage
//
//	m, err := idl.ParseFile("vehicle.idl")
//	if err != nil {
//	    log.Fatal(err)
//	}
//	src, err := idl.Generate(m)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	os.WriteFile("vehicle_gen.go", []byte(src), 0o644)
package idl

//fusa:req REQ-IDL-001
//fusa:req REQ-IDL-002
//fusa:req REQ-IDL-003
//fusa:req REQ-IDL-004

import (
	"fmt"
	"os"
)

// ParseFile parses path as an IDL file and returns the top-level Module.
func ParseFile(path string) (*Module, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("idl: read %s: %w", path, err)
	}
	return ParseString(string(data))
}

// ParseString parses src as IDL source and returns the top-level Module.
func ParseString(src string) (*Module, error) {
	p := newParser(src)
	return p.parseModule()
}

// Generate produces a Go source file from m. The returned string is a complete
// Go source file ready to be written to disk (run gofmt over it for canonical
// formatting).
func Generate(m *Module) (string, error) {
	g := newGenerator(m)
	return g.generate()
}
