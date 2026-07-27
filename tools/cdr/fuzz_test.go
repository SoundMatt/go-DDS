// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package cdr_test

import (
	"testing"

	"github.com/SoundMatt/go-DDS/tools/cdr"
)

// FuzzCDRDecode feeds arbitrary bytes into the CDR decoder to check that it
// never panics regardless of input. Malformed encapsulation headers, truncated
// fields, and misaligned reads must all be handled gracefully.
func FuzzCDRDecode(f *testing.F) {
	// Seed with valid CDR_LE frames for coverage breadth.
	e := cdr.NewEncoder()
	e.WriteBool(true)
	e.WriteInt32(42)
	e.WriteString("hello")
	f.Add(e.Bytes())

	// Seed with a bare CDR_LE header to exercise empty-body path.
	f.Add([]byte{0x00, 0x01, 0x00, 0x00})

	// Seed with garbage to drive error-path coverage.
	f.Add([]byte{0xFF, 0xFE, 0x00})
	f.Add([]byte{})

	f.Fuzz(func(t *testing.T, data []byte) {
		d, err := cdr.NewDecoder(data)
		if err != nil {
			return
		}
		// Attempt all read operations — none should panic.
		_, _ = d.ReadBool()
		_, _ = d.ReadInt8()
		_, _ = d.ReadUint8()
		_, _ = d.ReadInt16()
		_, _ = d.ReadUint16()
		_, _ = d.ReadInt32()
		_, _ = d.ReadUint32()
		_, _ = d.ReadInt64()
		_, _ = d.ReadUint64()
		_, _ = d.ReadFloat32()
		_, _ = d.ReadFloat64()
		_, _ = d.ReadString()
		_, _ = d.ReadBytes()
	})
}

// FuzzCDRRoundtrip feeds structured values through encode→decode and checks
// that the decoded value equals the encoded one. Detects alignment bugs and
// off-by-one errors in the padding logic.
func FuzzCDRRoundtrip(f *testing.F) {
	f.Add(int32(0), "")
	f.Add(int32(42), "hello")
	f.Add(int32(-1), "x")
	f.Add(int32(1<<30), "longer string value with spaces")

	f.Fuzz(func(t *testing.T, n int32, s string) {
		e := cdr.NewEncoder()
		e.WriteInt32(n)
		e.WriteString(s)

		d, err := cdr.NewDecoder(e.Bytes())
		if err != nil {
			t.Skip()
		}
		gotN, err := d.ReadInt32()
		if err != nil {
			t.Fatalf("ReadInt32: %v", err)
		}
		if gotN != n {
			t.Fatalf("int32 roundtrip: got %d want %d", gotN, n)
		}
		gotS, err := d.ReadString()
		if err != nil {
			t.Fatalf("ReadString: %v", err)
		}
		if gotS != s {
			t.Fatalf("string roundtrip: got %q want %q", gotS, s)
		}
	})
}
