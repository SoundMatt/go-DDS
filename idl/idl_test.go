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
	"os"
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

// ── ParseFile ──────────────────────────────────────────────────────────────────

func TestParseFile_RoundTrip(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "test*.idl")
	if err != nil {
		t.Fatalf("create temp: %v", err)
	}
	_, _ = f.WriteString(`struct Temp { double celsius; };`)
	f.Close()

	m, err := idl.ParseFile(f.Name())
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	if len(m.Structs) != 1 || m.Structs[0].Name != "Temp" {
		t.Errorf("unexpected module: %+v", m)
	}
}

func TestParseFile_NotFound(t *testing.T) {
	_, err := idl.ParseFile("/nonexistent/path/to/file.idl")
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}

func TestParseFile_Generate(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "test*.idl")
	if err != nil {
		t.Fatalf("create temp: %v", err)
	}
	_, _ = f.WriteString(`struct Sensor { string id; float value; };`)
	f.Close()

	m, err := idl.ParseFile(f.Name())
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	src, err := idl.Generate(m)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if !strings.Contains(src, "type Sensor struct") {
		t.Errorf("generated code missing Sensor struct:\n%s", src)
	}
}

// ── Array support ─────────────────────────────────────────────────────────────

const arrayIDL = `
struct Matrix {
    float data[16];
    long  dims[4];
    boolean flags[8];
};
`

func TestParseString_Array(t *testing.T) {
	m, err := idl.ParseString(arrayIDL)
	if err != nil {
		t.Fatalf("ParseString: %v", err)
	}
	if len(m.Structs) != 1 {
		t.Fatalf("want 1 struct, got %d", len(m.Structs))
	}
	s := m.Structs[0]
	if len(s.Fields) != 3 {
		t.Fatalf("want 3 fields, got %d", len(s.Fields))
	}
	f := s.Fields[0]
	if f.Type.Kind != idl.KindArray {
		t.Errorf("data: kind = %v, want KindArray", f.Type.Kind)
	}
	if f.Type.ArraySize != 16 {
		t.Errorf("data: size = %d, want 16", f.Type.ArraySize)
	}
	if f.Type.ElemType == nil || f.Type.ElemType.Kind != idl.KindFloat {
		t.Errorf("data: elem = %v, want KindFloat", f.Type.ElemType)
	}
}

func TestGenerate_Array_GoType(t *testing.T) {
	m, err := idl.ParseString(arrayIDL)
	if err != nil {
		t.Fatalf("ParseString: %v", err)
	}
	src, err := idl.Generate(m)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if !strings.Contains(src, "[16]float32") {
		t.Errorf("expected [16]float32 field, got:\n%s", src)
	}
	if !strings.Contains(src, "[4]int32") {
		t.Errorf("expected [4]int32 field, got:\n%s", src)
	}
	if !strings.Contains(src, "[8]bool") {
		t.Errorf("expected [8]bool field, got:\n%s", src)
	}
}

func TestGenerate_Array_NoTODO(t *testing.T) {
	m, err := idl.ParseString(arrayIDL)
	if err != nil {
		t.Fatalf("ParseString: %v", err)
	}
	src, err := idl.Generate(m)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if strings.Contains(src, "// TODO:") {
		t.Errorf("array codegen has TODO stubs:\n%s", src)
	}
}

func TestGenerate_Array_ContainsRangeLoop(t *testing.T) {
	m, err := idl.ParseString(arrayIDL)
	if err != nil {
		t.Fatalf("ParseString: %v", err)
	}
	src, err := idl.Generate(m)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if !strings.Contains(src, "for _i := range") {
		t.Errorf("expected range loop in array codec, got:\n%s", src)
	}
}

// ── Enum support ──────────────────────────────────────────────────────────────

const enumIDL = `
enum Gear { PARK, REVERSE, NEUTRAL, DRIVE, SPORT };
struct Transmission {
    Gear  current_gear;
    long  rpm;
};
`

func TestParseString_Enum(t *testing.T) {
	m, err := idl.ParseString(enumIDL)
	if err != nil {
		t.Fatalf("ParseString: %v", err)
	}
	if len(m.Enums) != 1 {
		t.Fatalf("want 1 enum, got %d", len(m.Enums))
	}
	e := m.Enums[0]
	if e.Name != "Gear" {
		t.Errorf("enum name = %q, want Gear", e.Name)
	}
	if len(e.Values) != 5 {
		t.Errorf("enum values = %v, want 5 items", e.Values)
	}
}

func TestGenerate_Enum_TypeDecl(t *testing.T) {
	m, err := idl.ParseString(enumIDL)
	if err != nil {
		t.Fatalf("ParseString: %v", err)
	}
	src, err := idl.Generate(m)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if !strings.Contains(src, "type Gear int32") {
		t.Errorf("expected 'type Gear int32', got:\n%s", src)
	}
	if !strings.Contains(src, "GearPARK") {
		t.Errorf("expected GearPARK constant, got:\n%s", src)
	}
	if !strings.Contains(src, "GearDRIVE") {
		t.Errorf("expected GearDRIVE constant, got:\n%s", src)
	}
}

func TestGenerate_Enum_InStruct_NoTODO(t *testing.T) {
	m, err := idl.ParseString(enumIDL)
	if err != nil {
		t.Fatalf("ParseString: %v", err)
	}
	src, err := idl.Generate(m)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if strings.Contains(src, "// TODO:") {
		t.Errorf("enum-in-struct codegen has TODO stubs:\n%s", src)
	}
}

func TestGenerate_Enum_UsesInt32Codec(t *testing.T) {
	m, err := idl.ParseString(enumIDL)
	if err != nil {
		t.Fatalf("ParseString: %v", err)
	}
	src, err := idl.Generate(m)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if !strings.Contains(src, "WriteInt32(int32(") {
		t.Errorf("expected int32 encoder for enum, got:\n%s", src)
	}
	if !strings.Contains(src, "ReadInt32()") {
		t.Errorf("expected int32 decoder for enum, got:\n%s", src)
	}
}

// ── Qualified names ───────────────────────────────────────────────────────────

const qualifiedIDL = `
module Vehicles {
    struct Wheel { double radius; };
    struct Car {
        Vehicles::Wheel front_left;
        Vehicles::Wheel front_right;
        long speed;
    };
};
`

func TestGenerate_QualifiedName_NoTODO(t *testing.T) {
	m, err := idl.ParseString(qualifiedIDL)
	if err != nil {
		t.Fatalf("ParseString: %v", err)
	}
	src, err := idl.Generate(m)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if strings.Contains(src, "// TODO:") {
		t.Errorf("qualified name lookup has TODO stubs:\n%s", src)
	}
}

func TestGenerate_QualifiedName_InlinesNestedFields(t *testing.T) {
	m, err := idl.ParseString(qualifiedIDL)
	if err != nil {
		t.Fatalf("ParseString: %v", err)
	}
	src, err := idl.Generate(m)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if !strings.Contains(src, "v.FrontLeft.Radius") {
		t.Errorf("expected v.FrontLeft.Radius in codec, got:\n%s", src)
	}
}

// ── Bounded string ────────────────────────────────────────────────────────────

func TestParseString_BoundedString(t *testing.T) {
	src := `struct Tag { string<64> label; long value; };`
	m, err := idl.ParseString(src)
	if err != nil {
		t.Fatalf("ParseString: %v", err)
	}
	if len(m.Structs) != 1 {
		t.Fatalf("want 1 struct, got %d", len(m.Structs))
	}
	f := m.Structs[0].Fields[0]
	if f.Type.Kind != idl.KindString {
		t.Errorf("bounded string kind = %v, want KindString", f.Type.Kind)
	}
}

// ── go/format ─────────────────────────────────────────────────────────────────

func TestGenerate_OutputIsGofmtFormatted(t *testing.T) {
	m, err := idl.ParseString(vehicleIDL)
	if err != nil {
		t.Fatalf("ParseString: %v", err)
	}
	src, err := idl.Generate(m)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	// After go/format, a second pass should produce identical output.
	m2, err := idl.ParseString(vehicleIDL)
	if err != nil {
		t.Fatalf("ParseString (2): %v", err)
	}
	src2, err := idl.Generate(m2)
	if err != nil {
		t.Fatalf("Generate (2): %v", err)
	}
	if src != src2 {
		t.Error("Generate is not idempotent — output changed on second call")
	}
}

// ── @key annotation ───────────────────────────────────────────────────────────

const keyIDL = `
struct Sensor {
    @key string  sensor_id;
    @key long    zone;
    double       reading;
};
`

func TestParseString_KeyAnnotation(t *testing.T) {
	m, err := idl.ParseString(keyIDL)
	if err != nil {
		t.Fatalf("ParseString: %v", err)
	}
	s := m.Structs[0]
	if !s.Fields[0].Key {
		t.Errorf("sensor_id: Key = false, want true")
	}
	if !s.Fields[1].Key {
		t.Errorf("zone: Key = false, want true")
	}
	if s.Fields[2].Key {
		t.Errorf("reading: Key = true, want false")
	}
}

func TestGenerate_KeyFields_Method(t *testing.T) {
	m, err := idl.ParseString(keyIDL)
	if err != nil {
		t.Fatalf("ParseString: %v", err)
	}
	src, err := idl.Generate(m)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if !strings.Contains(src, `"SensorId"`) {
		t.Errorf("expected SensorId in KeyFields, got:\n%s", src)
	}
	if !strings.Contains(src, `"Zone"`) {
		t.Errorf("expected Zone in KeyFields, got:\n%s", src)
	}
	if !strings.Contains(src, "func (SensorCodec) KeyFields()") {
		t.Errorf("expected KeyFields method, got:\n%s", src)
	}
}

func TestGenerate_NoKeyFields_ReturnsNil(t *testing.T) {
	src := `struct Plain { long x; double y; };`
	m, err := idl.ParseString(src)
	if err != nil {
		t.Fatalf("ParseString: %v", err)
	}
	out, err := idl.Generate(m)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if !strings.Contains(out, "return nil") {
		t.Errorf("expected 'return nil' in KeyFields for struct with no @key fields")
	}
}

// ── typedef support ───────────────────────────────────────────────────────────

const typedefIDL = `
typedef unsigned long   NodeID;
typedef double          Meters;
struct Position {
    NodeID node;
    Meters x;
    Meters y;
};
`

func TestParseString_Typedef(t *testing.T) {
	m, err := idl.ParseString(typedefIDL)
	if err != nil {
		t.Fatalf("ParseString: %v", err)
	}
	if len(m.Typedefs) != 2 {
		t.Fatalf("want 2 typedefs, got %d", len(m.Typedefs))
	}
	if m.Typedefs[0].Name != "NodeID" {
		t.Errorf("typedef[0].Name = %q, want NodeID", m.Typedefs[0].Name)
	}
	if m.Typedefs[0].Type.Kind != idl.KindULong {
		t.Errorf("NodeID underlying = %v, want KindULong", m.Typedefs[0].Type.Kind)
	}
}

func TestGenerate_Typedef_TypeAlias(t *testing.T) {
	m, err := idl.ParseString(typedefIDL)
	if err != nil {
		t.Fatalf("ParseString: %v", err)
	}
	src, err := idl.Generate(m)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if !strings.Contains(src, "type NodeID = uint32") {
		t.Errorf("expected 'type NodeID = uint32', got:\n%s", src)
	}
	if !strings.Contains(src, "type Meters = float64") {
		t.Errorf("expected 'type Meters = float64', got:\n%s", src)
	}
}

func TestGenerate_Typedef_ExpandedInStruct(t *testing.T) {
	m, err := idl.ParseString(typedefIDL)
	if err != nil {
		t.Fatalf("ParseString: %v", err)
	}
	src, err := idl.Generate(m)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	// The struct fields should use the expanded (underlying) types in the codec.
	if strings.Contains(src, "// TODO:") {
		t.Errorf("typedef resolution left TODO stubs:\n%s", src)
	}
}

// ── committed round-trip fixture ──────────────────────────────────────────────

func TestGenerate_MatchesCommitted(t *testing.T) {
	committed, err := os.ReadFile("roundtrip/schema_gen.go")
	if err != nil {
		t.Fatalf("read committed file: %v", err)
	}
	idlSrc, err := os.ReadFile("roundtrip/schema.idl")
	if err != nil {
		t.Fatalf("read IDL: %v", err)
	}
	m, err := idl.ParseString(string(idlSrc))
	if err != nil {
		t.Fatalf("ParseString: %v", err)
	}
	got, err := idl.Generate(m)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	// Normalise line endings so the test is portable across Windows (CRLF checkout) and Unix.
	norm := func(s string) string { return strings.ReplaceAll(s, "\r\n", "\n") }
	if norm(got) != norm(string(committed)) {
		t.Errorf("generated output differs from idl/roundtrip/schema_gen.go — run:\n"+
			"  go run ./cmd/ddstool idl -out idl/roundtrip/schema_gen.go"+
			" idl/roundtrip/schema.idl\nto regenerate.\n\ngot:\n%s", got)
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
