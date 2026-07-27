// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package idl_test

// Fuzz tests for the IDL parser and code generator.
//
// Run the fuzzer with e.g.:
//
//	go test -fuzz=FuzzIDLParser   -fuzztime=60s ./idl/...
//	go test -fuzz=FuzzIDLGenerate -fuzztime=60s ./idl/...

import (
	"testing"

	"github.com/SoundMatt/go-DDS/tools/idl"
)

// FuzzIDLParser ensures ParseString never panics on arbitrary input.
func FuzzIDLParser(f *testing.F) {
	// Seed corpus — valid IDL fragments.
	f.Add(`struct Foo { long x; };`)
	f.Add(`module M { struct S { double d; boolean b; }; };`)
	f.Add(`enum Color { RED, GREEN, BLUE };`)
	f.Add(`typedef unsigned long ID; struct Node { ID id; string name; };`)
	f.Add(`struct Arr { float data[8]; };`)
	f.Add(`struct Seq { sequence<double> values; };`)
	f.Add(`struct Nested { M::S s; long z; };`)
	f.Add(`struct Key { @key string id; double v; };`)
	f.Add(``)
	f.Add(`@annotation struct Bad {`)
	f.Add(`module {}`)
	f.Add("struct \x00 { long x; };")

	f.Fuzz(func(t *testing.T, src string) {
		m, parseErr := idl.ParseString(src)
		if parseErr != nil || m == nil {
			return
		}
	})
}

// FuzzIDLGenerate ensures Generate never panics on arbitrary IDL input.
func FuzzIDLGenerate(f *testing.F) {
	f.Add(`struct Pt { double x; double y; };`)
	f.Add(`module V { enum G { P, N, D }; struct T { G g; long r; }; };`)
	f.Add(`typedef long Meters; struct D { Meters dist; };`)
	f.Add(`struct A { float buf[4]; sequence<octet> raw; };`)
	f.Add(``)

	f.Fuzz(func(t *testing.T, src string) {
		m, parseErr := idl.ParseString(src)
		if parseErr != nil || m == nil {
			return
		}
		// Must not panic; formatting errors are acceptable for malformed ASTs.
		out, genErr := idl.Generate(m)
		if genErr != nil || out == "" {
			return
		}
	})
}
