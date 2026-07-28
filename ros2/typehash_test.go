// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package ros2

import (
	"strings"
	"testing"
)

func TestTypeHash_Deterministic(t *testing.T) {
	a := TypeHash("std_msgs::msg::dds_::String_", "string data")
	b := TypeHash("std_msgs::msg::dds_::String_", "string data")
	if a != b {
		t.Errorf("TypeHash not deterministic: %q != %q", a, b)
	}
}

func TestTypeHash_Prefix(t *testing.T) {
	h := TypeHash("std_msgs::msg::dds_::String_", "string data")
	if !strings.HasPrefix(h, TypeHashPrefix) {
		t.Errorf("TypeHash() = %q, missing prefix %q", h, TypeHashPrefix)
	}
	wantLen := len(TypeHashPrefix) + 64 // SHA-256 hex
	if len(h) != wantLen {
		t.Errorf("len(TypeHash()) = %d, want %d", len(h), wantLen)
	}
}

func TestTypeHash_DiffersOnTypeNameOrFields(t *testing.T) {
	base := TypeHash("pkg::msg::dds_::A_", "int32 x")
	diffType := TypeHash("pkg::msg::dds_::B_", "int32 x")
	diffFields := TypeHash("pkg::msg::dds_::A_", "int32 y")
	if base == diffType {
		t.Error("TypeHash unaffected by type name change")
	}
	if base == diffFields {
		t.Error("TypeHash unaffected by field descriptor change")
	}
}

func TestTypeHash_EmptyFieldDescriptorStable(t *testing.T) {
	// A caller with no field description available still gets a stable,
	// type-name-only hash.
	a := TypeHash("pkg::msg::dds_::A_", "")
	b := TypeHash("pkg::msg::dds_::A_", "")
	if a != b {
		t.Error("TypeHash with empty fieldDescriptor not stable")
	}
}
