// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package xtypes_test

import (
	"testing"

	"github.com/SoundMatt/go-DDS/xtypes"
)

// FuzzDynamicDataFromJSON feeds arbitrary JSON blobs into DynamicData.FromJSON
// to check it never panics. Unknown fields, mismatched types, and non-JSON input
// must be handled gracefully.
func FuzzDynamicDataFromJSON(f *testing.F) {
	td := xtypes.TypeDescriptor{
		Name:    "Speed",
		Version: 1,
		Fields: []xtypes.FieldDescriptor{
			{Name: "kmh", Kind: xtypes.KindFloat64},
			{Name: "label", Kind: xtypes.KindString, Optional: true},
		},
	}

	f.Add([]byte(`{"kmh":60.0}`))
	f.Add([]byte(`{"kmh":0,"label":"slow"}`))
	f.Add([]byte(`{}`))
	f.Add([]byte(`{"unknown_field":"x"}`))
	f.Add([]byte(`not json`))
	f.Add([]byte(`null`))
	f.Add([]byte{})

	f.Fuzz(func(t *testing.T, data []byte) {
		dd := xtypes.NewDynamicData(&td)
		_ = dd.FromJSON(data) // must never panic
	})
}

// FuzzTypeIdentifier feeds arbitrary TypeDescriptor names into Identify to
// check that hash generation is stable and doesn't panic on edge-case inputs.
func FuzzTypeIdentifier(f *testing.F) {
	f.Add("Speed", uint32(1))
	f.Add("", uint32(0))
	f.Add("a/b/c::Type", uint32(99))
	f.Add(string(make([]byte, 1024)), uint32(0xFFFF))

	f.Fuzz(func(t *testing.T, name string, version uint32) {
		td := &xtypes.TypeDescriptor{Name: name, Version: version}
		id1 := xtypes.Identify(td)
		id2 := xtypes.Identify(td) // must be deterministic
		if id1 != id2 {
			t.Fatalf("Identify is not deterministic: got %v then %v", id1, id2)
		}
	})
}
