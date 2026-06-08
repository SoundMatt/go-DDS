// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package dds_test

//fusa:test REQ-CODEC-001
//fusa:test REQ-CODEC-002
//fusa:test REQ-CODEC-003

// Fuzz tests for dds.ProtoCodec[T].
//
// Run the fuzzer with:
//
//	go test -fuzz=FuzzProtoCodecRoundTrip   -fuzztime=60s .
//	go test -fuzz=FuzzProtoCodecOpenArbitrary -fuzztime=60s .

import (
	"bytes"
	"testing"

	dds "github.com/SoundMatt/go-DDS"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// FuzzProtoCodecRoundTrip verifies that Marshal→Unmarshal is an identity for
// any StringValue message (covers the reflect.New instantiation path).
func FuzzProtoCodecRoundTrip(f *testing.F) {
	f.Add("")
	f.Add("hello world")
	f.Add("unicode: 日本語")
	f.Add(string(make([]byte, 1024)))

	codec := dds.ProtoCodec[*wrapperspb.StringValue]{}

	f.Fuzz(func(t *testing.T, s string) {
		msg := wrapperspb.String(s)

		data, err := codec.Marshal(msg)
		if err != nil {
			t.Fatalf("Marshal(%q): %v", s, err)
		}

		got, err := codec.Unmarshal(data)
		if err != nil {
			t.Fatalf("Unmarshal: %v", err)
		}

		if got.GetValue() != s {
			t.Errorf("round-trip: got %q, want %q", got.GetValue(), s)
		}

		// Re-marshal and compare bytes to verify determinism.
		data2, err := codec.Marshal(got)
		if err != nil {
			t.Fatalf("re-Marshal: %v", err)
		}
		if !bytes.Equal(data, data2) {
			t.Errorf("re-marshal produced different bytes")
		}
	})
}

// FuzzProtoCodecOpenArbitrary feeds arbitrary bytes to ProtoCodec.Unmarshal.
// proto.Unmarshal is lenient with many invalid inputs (it returns partial
// results rather than errors); we only require it does not panic.
func FuzzProtoCodecOpenArbitrary(f *testing.F) {
	f.Add([]byte(""))
	f.Add([]byte{0x00})
	f.Add([]byte{0xFF, 0xFE})
	f.Add([]byte("not proto"))
	f.Add(make([]byte, 512))

	codec := dds.ProtoCodec[*wrapperspb.StringValue]{}

	f.Fuzz(func(t *testing.T, data []byte) {
		// Must not panic.
		_, _ = codec.Unmarshal(data)
	})
}
