// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package xtypes_test

//fusa:test REQ-XTYPE-001
//fusa:test REQ-XTYPE-002
//fusa:test REQ-XTYPE-003
//fusa:test REQ-XTYPE-004
//fusa:test REQ-XTYPE-005
//fusa:test REQ-XTYPE-006
//fusa:test REQ-TREG-001
//fusa:test REQ-TREG-002
//fusa:test REQ-TREG-003

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/SoundMatt/go-DDS/xtypes"
)

// ── Helpers ───────────────────────────────────────────────────────────────────

func speedDesc() xtypes.TypeDescriptor {
	return xtypes.TypeDescriptor{
		Name:    "VehicleSpeed",
		Version: 1,
		Fields: []xtypes.FieldDescriptor{
			{Name: "kmh", Kind: xtypes.KindFloat64},
			{Name: "unit", Kind: xtypes.KindString, Optional: true},
		},
	}
}

// ── TypeKind.String ───────────────────────────────────────────────────────────

func TestTypeKind_String(t *testing.T) {
	cases := []struct {
		k    xtypes.TypeKind
		want string
	}{
		{xtypes.KindBool, "bool"},
		{xtypes.KindInt32, "int32"},
		{xtypes.KindInt64, "int64"},
		{xtypes.KindFloat64, "float64"},
		{xtypes.KindString, "string"},
		{xtypes.KindBytes, "bytes"},
		{xtypes.KindStruct, "struct"},
		{xtypes.KindSeq, "seq"},
		{xtypes.TypeKind(99), "TypeKind(99)"},
	}
	for _, c := range cases {
		if got := c.k.String(); got != c.want {
			t.Errorf("TypeKind(%d).String() = %q, want %q", c.k, got, c.want)
		}
	}
}

// ── Identify ──────────────────────────────────────────────────────────────────

func TestIdentify_Deterministic(t *testing.T) {
	td := speedDesc()
	a := xtypes.Identify(&td)
	b := xtypes.Identify(&td)
	if a != b {
		t.Error("Identify: same descriptor should produce same identifier")
	}
}

func TestIdentify_DifferentTypes_DifferentHash(t *testing.T) {
	a := xtypes.TypeDescriptor{Name: "TypeA", Fields: []xtypes.FieldDescriptor{{Name: "x", Kind: xtypes.KindInt32}}}
	b := xtypes.TypeDescriptor{Name: "TypeB", Fields: []xtypes.FieldDescriptor{{Name: "x", Kind: xtypes.KindInt32}}}
	idA := xtypes.Identify(&a)
	idB := xtypes.Identify(&b)
	if idA.Hash == idB.Hash {
		t.Error("Identify: different type names should produce different hashes")
	}
}

func TestIdentify_FieldOrderIndependent(t *testing.T) {
	// Field order should not affect the hash (canonical sort is applied).
	t1 := xtypes.TypeDescriptor{
		Name: "Same",
		Fields: []xtypes.FieldDescriptor{
			{Name: "a", Kind: xtypes.KindInt32},
			{Name: "b", Kind: xtypes.KindString},
		},
	}
	t2 := xtypes.TypeDescriptor{
		Name: "Same",
		Fields: []xtypes.FieldDescriptor{
			{Name: "b", Kind: xtypes.KindString},
			{Name: "a", Kind: xtypes.KindInt32},
		},
	}
	if xtypes.Identify(&t1) != xtypes.Identify(&t2) {
		t.Error("Identify: field order should not affect hash")
	}
}

// TestIdentify_StructField exercises the canonicalField branch for KindStruct
// fields (fields that contain nested sub-fields).
func TestIdentify_StructField(t *testing.T) {
	nested := xtypes.TypeDescriptor{
		Name: "Point",
		Fields: []xtypes.FieldDescriptor{
			{
				Name: "position",
				Kind: xtypes.KindStruct,
				Fields: []xtypes.FieldDescriptor{
					{Name: "x", Kind: xtypes.KindFloat64},
					{Name: "y", Kind: xtypes.KindFloat64},
				},
			},
		},
	}
	// Hash must be stable.
	id1 := xtypes.Identify(&nested)
	id2 := xtypes.Identify(&nested)
	if id1 != id2 {
		t.Error("Identify with KindStruct field: not deterministic")
	}
	// Nested sub-fields must affect the hash.
	different := xtypes.TypeDescriptor{
		Name: "Point",
		Fields: []xtypes.FieldDescriptor{
			{
				Name: "position",
				Kind: xtypes.KindStruct,
				Fields: []xtypes.FieldDescriptor{
					{Name: "x", Kind: xtypes.KindFloat64},
					{Name: "z", Kind: xtypes.KindFloat64}, // different name
				},
			},
		},
	}
	if xtypes.Identify(&nested) == xtypes.Identify(&different) {
		t.Error("Identify: different nested field names should produce different hash")
	}
}

// TestIdentify_SeqField exercises the canonicalField branch for KindSeq fields
// (fields that carry an Element descriptor for the sequence element type).
func TestIdentify_SeqField(t *testing.T) {
	withSeq := xtypes.TypeDescriptor{
		Name: "Batch",
		Fields: []xtypes.FieldDescriptor{
			{
				Name:    "items",
				Kind:    xtypes.KindSeq,
				Element: &xtypes.FieldDescriptor{Name: "item", Kind: xtypes.KindFloat64},
			},
		},
	}
	id1 := xtypes.Identify(&withSeq)
	id2 := xtypes.Identify(&withSeq)
	if id1 != id2 {
		t.Error("Identify with KindSeq field: not deterministic")
	}
	// Changing the element type must change the hash.
	withDiffElem := xtypes.TypeDescriptor{
		Name: "Batch",
		Fields: []xtypes.FieldDescriptor{
			{
				Name:    "items",
				Kind:    xtypes.KindSeq,
				Element: &xtypes.FieldDescriptor{Name: "item", Kind: xtypes.KindString},
			},
		},
	}
	if xtypes.Identify(&withSeq) == xtypes.Identify(&withDiffElem) {
		t.Error("Identify: different element Kind in KindSeq field should produce different hash")
	}
}

// ── NewTypeObject ─────────────────────────────────────────────────────────────

func TestNewTypeObject_SetsID(t *testing.T) {
	td := speedDesc()
	to := xtypes.NewTypeObject(td)
	if to.ID.Name != td.Name {
		t.Errorf("TypeObject.ID.Name: got %q, want %q", to.ID.Name, td.Name)
	}
	if to.ID.Hash == [8]byte{} {
		t.Error("TypeObject.ID.Hash should not be zero")
	}
}

// ── DynamicData ───────────────────────────────────────────────────────────────

func TestDynamicData_SetGet(t *testing.T) {
	td := speedDesc()
	d := xtypes.NewDynamicData(&td)

	if err := d.Set("kmh", 120.0); err != nil {
		t.Fatalf("Set kmh: %v", err)
	}
	v, ok := d.Get("kmh")
	if !ok {
		t.Fatal("Get kmh: not found")
	}
	if v != 120.0 {
		t.Errorf("Get kmh: got %v, want 120.0", v)
	}
}

func TestDynamicData_UnknownField_Error(t *testing.T) {
	td := speedDesc()
	d := xtypes.NewDynamicData(&td)
	if err := d.Set("nonexistent", 1); err == nil {
		t.Error("Set nonexistent field should return error")
	}
}

func TestDynamicData_Get_UnsetField_ReturnsFalse(t *testing.T) {
	td := speedDesc()
	d := xtypes.NewDynamicData(&td)
	ignoredRet, ok := d.Get("kmh") // declared but not set
	_ = ignoredRet
	if ok {
		t.Error("Get on unset field should return false")
	}
}

func TestDynamicData_ToJSON(t *testing.T) {
	td := speedDesc()
	d := xtypes.NewDynamicData(&td)
	_ = d.Set("kmh", 99.5)
	_ = d.Set("unit", "kmh")

	b, err := d.ToJSON()
	if err != nil {
		t.Fatalf("ToJSON: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if m["kmh"] != 99.5 {
		t.Errorf("ToJSON kmh: got %v, want 99.5", m["kmh"])
	}
}

func TestDynamicData_FromJSON_RoundTrip(t *testing.T) {
	td := speedDesc()
	d := xtypes.NewDynamicData(&td)
	_ = d.Set("kmh", 80.0)
	_ = d.Set("unit", "kmh")

	b, err := d.ToJSON()
	if err != nil {
		t.Fatalf("ToJSON: %v", err)
	}

	d2 := xtypes.NewDynamicData(&td)
	if err := d2.FromJSON(b); err != nil {
		t.Fatalf("FromJSON: %v", err)
	}
	v, ok := d2.Get("kmh")
	if !ok {
		t.Fatal("FromJSON: kmh not found after round-trip")
	}
	// JSON numbers unmarshal as float64.
	f, ok2 := v.(float64)
	if !ok2 {
		t.Fatalf("FromJSON kmh: expected float64, got %T", v)
	}
	if f != 80.0 {
		t.Errorf("FromJSON kmh: got %v, want 80.0", f)
	}
}

func TestDynamicData_FromJSON_UnknownFieldsIgnored(t *testing.T) {
	td := speedDesc()
	d := xtypes.NewDynamicData(&td)
	// "future_field" is not in the descriptor — forward compat: ignore it.
	raw := `{"kmh":50,"unit":"kmh","future_field":true}`
	if err := d.FromJSON([]byte(raw)); err != nil {
		t.Fatalf("FromJSON: %v", err)
	}
	ignoredRet, ok := d.Get("future_field")
	_ = ignoredRet
	if ok {
		t.Error("unknown field should not be stored in DynamicData")
	}
}

func TestDynamicData_FromJSON_InvalidJSON(t *testing.T) {
	td := speedDesc()
	d := xtypes.NewDynamicData(&td)
	if err := d.FromJSON([]byte("{not-json}")); err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestDynamicData_TypeDescriptor(t *testing.T) {
	td := speedDesc()
	d := xtypes.NewDynamicData(&td)
	if d.TypeDescriptor() != &td {
		t.Error("TypeDescriptor() should return the backing descriptor pointer")
	}
}

// ── TypeRegistry ──────────────────────────────────────────────────────────────

func TestTypeRegistry_Register_And_Lookup(t *testing.T) {
	r := xtypes.NewTypeRegistry()
	td := speedDesc()
	to := xtypes.NewTypeObject(td)
	if err := r.Register(to); err != nil {
		t.Fatalf("Register: %v", err)
	}
	got, ok := r.Lookup("VehicleSpeed")
	if !ok {
		t.Fatal("Lookup: not found after Register")
	}
	if got.ID != to.ID {
		t.Errorf("Lookup: id mismatch")
	}
}

func TestTypeRegistry_Register_IdempotentSameHash(t *testing.T) {
	r := xtypes.NewTypeRegistry()
	td := speedDesc()
	to := xtypes.NewTypeObject(td)
	_ = r.Register(to)
	if err := r.Register(to); err != nil {
		t.Errorf("re-registering same object should be no-op, got %v", err)
	}
}

func TestTypeRegistry_Register_NameConflict_DifferentHash(t *testing.T) {
	r := xtypes.NewTypeRegistry()
	// v1
	td1 := xtypes.TypeDescriptor{Name: "MyType", Version: 1, Fields: []xtypes.FieldDescriptor{{Name: "x", Kind: xtypes.KindInt32}}}
	to1 := xtypes.NewTypeObject(td1)
	_ = r.Register(to1)

	// v2 — same name, different structure
	td2 := xtypes.TypeDescriptor{Name: "MyType", Version: 2, Fields: []xtypes.FieldDescriptor{{Name: "x", Kind: xtypes.KindString}}}
	to2 := xtypes.NewTypeObject(td2)
	if err := r.Register(to2); !errors.Is(err, xtypes.ErrTypeMismatch) {
		t.Errorf("expected ErrTypeMismatch for conflicting type, got %v", err)
	}
}

func TestTypeRegistry_Lookup_Missing(t *testing.T) {
	r := xtypes.NewTypeRegistry()
	ignoredRet, ok := r.Lookup("Nonexistent")
	_ = ignoredRet
	if ok {
		t.Error("Lookup of unregistered type should return false")
	}
}

func TestTypeRegistry_All_SortedByName(t *testing.T) {
	r := xtypes.NewTypeRegistry()
	for _, name := range []string{"Ztype", "Atype", "Mtype"} {
		td := xtypes.TypeDescriptor{Name: name}
		_ = r.Register(xtypes.NewTypeObject(td))
	}
	all := r.All()
	if len(all) != 3 {
		t.Fatalf("All: got %d, want 3", len(all))
	}
	if all[0].ID.Name != "Atype" || all[1].ID.Name != "Mtype" || all[2].ID.Name != "Ztype" {
		t.Errorf("All: not sorted: %v %v %v", all[0].ID.Name, all[1].ID.Name, all[2].ID.Name)
	}
}

// ── CheckCompatibility ────────────────────────────────────────────────────────

func TestCompatibility_SameType(t *testing.T) {
	td := speedDesc()
	res := xtypes.CheckCompatibility(&td, &td)
	if !res.Compatible {
		t.Errorf("same type should be compatible: %s", res.Reason)
	}
}

func TestCompatibility_AddedOptionalField(t *testing.T) {
	writer := xtypes.TypeDescriptor{
		Name:   "T",
		Fields: []xtypes.FieldDescriptor{{Name: "x", Kind: xtypes.KindInt32}},
	}
	reader := xtypes.TypeDescriptor{
		Name: "T",
		Fields: []xtypes.FieldDescriptor{
			{Name: "x", Kind: xtypes.KindInt32},
			{Name: "y", Kind: xtypes.KindInt32, Optional: true}, // added, optional
		},
	}
	res := xtypes.CheckCompatibility(&writer, &reader)
	if !res.Compatible {
		t.Errorf("added optional field should be compatible: %s", res.Reason)
	}
}

func TestCompatibility_AddedRequiredField_Incompatible(t *testing.T) {
	writer := xtypes.TypeDescriptor{
		Name:   "T",
		Fields: []xtypes.FieldDescriptor{{Name: "x", Kind: xtypes.KindInt32}},
	}
	reader := xtypes.TypeDescriptor{
		Name: "T",
		Fields: []xtypes.FieldDescriptor{
			{Name: "x", Kind: xtypes.KindInt32},
			{Name: "y", Kind: xtypes.KindInt32}, // required but absent in writer
		},
	}
	res := xtypes.CheckCompatibility(&writer, &reader)
	if res.Compatible {
		t.Error("added required field missing from writer should be incompatible")
	}
}

func TestCompatibility_FieldAbsentFromWriterOptionalInReader(t *testing.T) {
	// Writer lacks "unit"; reader declares it optional — compatible.
	writer := xtypes.TypeDescriptor{
		Name:   "T",
		Fields: []xtypes.FieldDescriptor{{Name: "kmh", Kind: xtypes.KindFloat64}},
	}
	reader := xtypes.TypeDescriptor{
		Name: "T",
		Fields: []xtypes.FieldDescriptor{
			{Name: "kmh", Kind: xtypes.KindFloat64},
			{Name: "unit", Kind: xtypes.KindString, Optional: true},
		},
	}
	res := xtypes.CheckCompatibility(&writer, &reader)
	if !res.Compatible {
		t.Errorf("optional field absent from writer should be compatible: %s", res.Reason)
	}
}

func TestCompatibility_TypeMismatch_Incompatible(t *testing.T) {
	writer := xtypes.TypeDescriptor{
		Name:   "T",
		Fields: []xtypes.FieldDescriptor{{Name: "speed", Kind: xtypes.KindInt32}},
	}
	reader := xtypes.TypeDescriptor{
		Name:   "T",
		Fields: []xtypes.FieldDescriptor{{Name: "speed", Kind: xtypes.KindFloat64}},
	}
	res := xtypes.CheckCompatibility(&writer, &reader)
	if res.Compatible {
		t.Error("field kind mismatch should be incompatible")
	}
}

func TestCompatibility_ExtraWriterField_OK(t *testing.T) {
	// Writer sends "extra" that reader doesn't know about — forward compat.
	writer := xtypes.TypeDescriptor{
		Name: "T",
		Fields: []xtypes.FieldDescriptor{
			{Name: "x", Kind: xtypes.KindInt32},
			{Name: "extra", Kind: xtypes.KindString},
		},
	}
	reader := xtypes.TypeDescriptor{
		Name:   "T",
		Fields: []xtypes.FieldDescriptor{{Name: "x", Kind: xtypes.KindInt32}},
	}
	res := xtypes.CheckCompatibility(&writer, &reader)
	if !res.Compatible {
		t.Errorf("extra writer field should be forward-compatible: %s", res.Reason)
	}
}
