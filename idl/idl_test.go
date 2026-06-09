// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package idl_test

//fusa:test REQ-IDL-001
//fusa:test REQ-IDL-002
//fusa:test REQ-IDL-003
//fusa:test REQ-IDL-004

import (
	"strings"
	"testing"

	"github.com/SoundMatt/go-DDS/idl"
)

const vehicleIDL = `
// Vehicle telemetry IDL
module VehicleData {
    struct Speed {
        string vehicle_id;
        double kph;
        long long timestamp_ns;
        boolean valid;
    };

    struct EngineStatus {
        boolean running;
        unsigned long rpm;
        float temperature;
        unsigned short gear;
        short error_code;
    };
};
`

func TestParseString_BasicModule(t *testing.T) {
	m, err := idl.ParseString(vehicleIDL)
	if err != nil {
		t.Fatalf("ParseString: %v", err)
	}
	if len(m.Modules) != 1 {
		t.Fatalf("got %d modules, want 1", len(m.Modules))
	}
	sub := m.Modules[0]
	if sub.Name != "VehicleData" {
		t.Errorf("module name = %q, want VehicleData", sub.Name)
	}
	if len(sub.Structs) != 2 {
		t.Fatalf("got %d structs, want 2", len(sub.Structs))
	}
}

func TestParseString_StructFields(t *testing.T) {
	m, err := idl.ParseString(vehicleIDL)
	if err != nil {
		t.Fatalf("ParseString: %v", err)
	}
	speed := m.Modules[0].Structs[0]
	if speed.Name != "Speed" {
		t.Fatalf("struct name = %q, want Speed", speed.Name)
	}
	if len(speed.Fields) != 4 {
		t.Fatalf("field count = %d, want 4", len(speed.Fields))
	}
	if speed.Fields[0].Name != "vehicle_id" {
		t.Errorf("field[0].Name = %q, want vehicle_id", speed.Fields[0].Name)
	}
	if speed.Fields[0].Type.Kind != idl.KindString {
		t.Errorf("field[0].Type = %v, want KindString", speed.Fields[0].Type.Kind)
	}
	if speed.Fields[1].Type.Kind != idl.KindDouble {
		t.Errorf("field[1].Type = %v, want KindDouble", speed.Fields[1].Type.Kind)
	}
	if speed.Fields[2].Type.Kind != idl.KindLongLong {
		t.Errorf("field[2].Type = %v, want KindLongLong", speed.Fields[2].Type.Kind)
	}
	if speed.Fields[3].Type.Kind != idl.KindBoolean {
		t.Errorf("field[3].Type = %v, want KindBoolean", speed.Fields[3].Type.Kind)
	}
}

func TestParseString_UnsignedTypes(t *testing.T) {
	m, err := idl.ParseString(vehicleIDL)
	if err != nil {
		t.Fatalf("ParseString: %v", err)
	}
	eng := m.Modules[0].Structs[1]
	if eng.Fields[1].Type.Kind != idl.KindULong {
		t.Errorf("rpm: got %v, want KindULong", eng.Fields[1].Type.Kind)
	}
	if eng.Fields[3].Type.Kind != idl.KindUShort {
		t.Errorf("gear: got %v, want KindUShort", eng.Fields[3].Type.Kind)
	}
	if eng.Fields[4].Type.Kind != idl.KindShort {
		t.Errorf("error_code: got %v, want KindShort", eng.Fields[4].Type.Kind)
	}
}

func TestParseString_Sequence(t *testing.T) {
	src := `struct Batch { sequence<double> samples; };`
	m, err := idl.ParseString(src)
	if err != nil {
		t.Fatalf("ParseString: %v", err)
	}
	if len(m.Structs) != 1 {
		t.Fatalf("expected 1 struct, got %d", len(m.Structs))
	}
	f := m.Structs[0].Fields[0]
	if f.Type.Kind != idl.KindSequence {
		t.Errorf("type kind = %v, want KindSequence", f.Type.Kind)
	}
	if f.Type.ElemType == nil || f.Type.ElemType.Kind != idl.KindDouble {
		t.Errorf("elem type: got %v, want KindDouble", f.Type.ElemType)
	}
}

func TestParseString_OctetSequence(t *testing.T) {
	src := `struct Blob { sequence<octet> data; };`
	m, err := idl.ParseString(src)
	if err != nil {
		t.Fatalf("ParseString: %v", err)
	}
	f := m.Structs[0].Fields[0]
	if f.Type.Kind != idl.KindSequence {
		t.Errorf("type kind = %v, want KindSequence", f.Type.Kind)
	}
	if f.Type.ElemType.Kind != idl.KindOctet {
		t.Errorf("elem = %v, want KindOctet", f.Type.ElemType.Kind)
	}
}

func TestGenerate_ContainsStruct(t *testing.T) {
	m, err := idl.ParseString(vehicleIDL)
	if err != nil {
		t.Fatalf("ParseString: %v", err)
	}
	src, err := idl.Generate(m)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if !strings.Contains(src, "type Speed struct") {
		t.Error("generated source missing 'type Speed struct'")
	}
	if !strings.Contains(src, "type EngineStatus struct") {
		t.Error("generated source missing 'type EngineStatus struct'")
	}
}

func TestGenerate_ContainsCodec(t *testing.T) {
	m, err := idl.ParseString(vehicleIDL)
	if err != nil {
		t.Fatalf("ParseString: %v", err)
	}
	src, err := idl.Generate(m)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if !strings.Contains(src, "type SpeedCodec struct") {
		t.Error("generated source missing SpeedCodec")
	}
	if !strings.Contains(src, "func (SpeedCodec) Marshal") {
		t.Error("generated source missing SpeedCodec.Marshal")
	}
	if !strings.Contains(src, "func (SpeedCodec) Unmarshal") {
		t.Error("generated source missing SpeedCodec.Unmarshal")
	}
}

func TestGenerate_PackageName(t *testing.T) {
	m, err := idl.ParseString(vehicleIDL)
	if err != nil {
		t.Fatalf("ParseString: %v", err)
	}
	src, err := idl.Generate(m)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	// Top-level module is VehicleData, so package is the outer (empty) wrapper;
	// structs are emitted from the sub-module, package is "vehicledata".
	if !strings.Contains(src, "package vehicledata") && !strings.Contains(src, "package idlgen") {
		t.Errorf("unexpected package name in:\n%s", src[:min(200, len(src))])
	}
}

func TestGenerate_FieldNames(t *testing.T) {
	m, err := idl.ParseString(vehicleIDL)
	if err != nil {
		t.Fatalf("ParseString: %v", err)
	}
	src, err := idl.Generate(m)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if !strings.Contains(src, "VehicleId") {
		t.Error("expected exported field VehicleId")
	}
	if !strings.Contains(src, `json:"vehicle_id"`) {
		t.Error("expected json tag vehicle_id")
	}
}

const nestedIDL = `
struct Header {
    unsigned long seq;
    string source;
};
struct Frame {
    Header header;
    double value;
    boolean valid;
};
`

func TestGenerate_NestedStruct_NoTODO(t *testing.T) {
	m, err := idl.ParseString(nestedIDL)
	if err != nil {
		t.Fatalf("ParseString: %v", err)
	}
	src, err := idl.Generate(m)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if strings.Contains(src, "// TODO:") {
		t.Errorf("generated code has TODO stub(s):\n%s", src)
	}
}

func TestGenerate_NestedStruct_InlinesFields(t *testing.T) {
	m, err := idl.ParseString(nestedIDL)
	if err != nil {
		t.Fatalf("ParseString: %v", err)
	}
	src, err := idl.Generate(m)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	// Encoder must reference nested struct fields directly.
	if !strings.Contains(src, "v.Header.Seq") {
		t.Errorf("expected v.Header.Seq in encoder, got:\n%s", src)
	}
	if !strings.Contains(src, "v.Header.Source") {
		t.Errorf("expected v.Header.Source in encoder, got:\n%s", src)
	}
	// Decoder must assign to nested struct fields.
	if !strings.Contains(src, "v.Header.Seq") {
		t.Errorf("expected v.Header.Seq assignment in decoder, got:\n%s", src)
	}
}

func TestGenerate_NestedStruct_DeepNesting(t *testing.T) {
	src := `
struct A { long x; };
struct B { A a; long y; };
struct C { B b; A a2; };
`
	m, err := idl.ParseString(src)
	if err != nil {
		t.Fatalf("ParseString: %v", err)
	}
	out, err := idl.Generate(m)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if strings.Contains(out, "// TODO:") {
		t.Errorf("deep nesting left TODO stubs:\n%s", out)
	}
	// C's codec must reference B's field A's x transitively.
	if !strings.Contains(out, "v.B.A.X") {
		t.Errorf("expected v.B.A.X for deep nesting, got:\n%s", out)
	}
}

func TestGenerate_ContainsFactories(t *testing.T) {
	m, err := idl.ParseString(vehicleIDL)
	if err != nil {
		t.Fatalf("ParseString: %v", err)
	}
	src, err := idl.Generate(m)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	for _, name := range []string{"NewSpeedPublisher", "NewSpeedSubscriber", "NewEngineStatusPublisher", "NewEngineStatusSubscriber"} {
		if !strings.Contains(src, name) {
			t.Errorf("generated source missing factory %q", name)
		}
	}
	if !strings.Contains(src, "dds.Participant") {
		t.Error("generated source missing dds.Participant parameter")
	}
	if !strings.Contains(src, "dds.TypedPublisher[Speed]") {
		t.Error("generated source missing TypedPublisher return type")
	}
	if !strings.Contains(src, "dds.SubscriberOption") {
		t.Error("generated source missing dds.SubscriberOption variadic")
	}
}

func TestGenerate_EmptyInput(t *testing.T) {
	m, err := idl.ParseString("")
	if err != nil {
		t.Fatalf("ParseString empty: %v", err)
	}
	src, err := idl.Generate(m)
	if err != nil {
		t.Fatalf("Generate empty: %v", err)
	}
	if !strings.Contains(src, "package") {
		t.Error("generated source missing package declaration")
	}
}

func TestParseString_Comments(t *testing.T) {
	src := `
// This is a comment
module Foo {
    /* block comment */
    struct Bar {
        long x; // inline comment
    };
};`
	m, err := idl.ParseString(src)
	if err != nil {
		t.Fatalf("ParseString with comments: %v", err)
	}
	if len(m.Modules) != 1 || len(m.Modules[0].Structs) != 1 {
		t.Errorf("unexpected structure: %+v", m)
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
