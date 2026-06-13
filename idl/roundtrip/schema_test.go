// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package idlgen

import (
	"reflect"
	"testing"
	"time"

	dds "github.com/SoundMatt/go-DDS"
	"github.com/SoundMatt/go-DDS/mock"
)

func TestHeader_RoundTrip(t *testing.T) {
	v := Header{
		TopicId:     "sensors/engine",
		TimestampNs: 1_718_000_000_000,
		Priority:    PriorityHIGH,
	}
	data, err := HeaderCodec{}.Marshal(v)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	got, err := HeaderCodec{}.Unmarshal(data)
	if err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if got != v {
		t.Errorf("round-trip mismatch:\n got  %+v\n want %+v", got, v)
	}
}

func TestHeader_KeyFields(t *testing.T) {
	keys := HeaderCodec{}.KeyFields()
	if len(keys) != 1 || keys[0] != "TopicId" {
		t.Errorf("KeyFields = %v, want [TopicId]", keys)
	}
}

func TestTelemetry_RoundTrip(t *testing.T) {
	v := Telemetry{
		Header: Header{
			TopicId:     "sensors/temp",
			TimestampNs: 9_999_999_999,
			Priority:    PriorityCRITICAL,
		},
		Values:      [4]float64{1.1, 2.2, 3.3, 4.4},
		Temperature: 98.6,
		Valid:       true,
		Extras:      []float64{10.0, 20.0, 30.0},
	}
	data, err := TelemetryCodec{}.Marshal(v)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	got, err := TelemetryCodec{}.Unmarshal(data)
	if err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if !reflect.DeepEqual(got, v) {
		t.Errorf("round-trip mismatch:\n got  %+v\n want %+v", got, v)
	}
}

func TestTelemetry_EmptyExtras_RoundTrip(t *testing.T) {
	v := Telemetry{
		Header: Header{TopicId: "x", Priority: PriorityLOW},
		Valid:  false,
		Extras: nil,
	}
	data, err := TelemetryCodec{}.Marshal(v)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	got, err := TelemetryCodec{}.Unmarshal(data)
	if err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if got.Header.TopicId != v.Header.TopicId || got.Valid != v.Valid || len(got.Extras) != 0 {
		t.Errorf("round-trip mismatch: got %+v, want %+v", got, v)
	}
}

func TestPriority_EnumValues(t *testing.T) {
	if PriorityLOW != 0 || PriorityMEDIUM != 1 || PriorityHIGH != 2 || PriorityCRITICAL != 3 {
		t.Error("enum values out of order")
	}
}

func TestTopicID_IsUint32Alias(t *testing.T) {
	var id TopicID = 42
	if id != 42 {
		t.Errorf("TopicID alias broken: %v", id)
	}
}

func TestTelemetry_KeyFields_Empty(t *testing.T) {
	keys := TelemetryCodec{}.KeyFields()
	if keys != nil {
		t.Errorf("TelemetryCodec.KeyFields = %v, want nil", keys)
	}
}

// ── Factory function tests ────────────────────────────────────────────────────

func newMockParticipant(t *testing.T) dds.Participant {
	t.Helper()
	p, err := mock.New(dds.Domain(0))
	if err != nil {
		t.Fatalf("mock.New: %v", err)
	}
	t.Cleanup(func() { _ = p.Close() })
	return p
}

func TestNewHeaderPublisher_and_Subscriber(t *testing.T) {
	p := newMockParticipant(t)

	sub, err := NewHeaderSubscriber(p, "idl/header", dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewHeaderSubscriber: %v", err)
	}
	defer sub.Close()

	pub, err := NewHeaderPublisher(p, "idl/header", dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewHeaderPublisher: %v", err)
	}
	defer pub.Close()

	want := Header{TopicId: "sensors/test", TimestampNs: 12345, Priority: PriorityHIGH}
	if err := pub.Write(want); err != nil {
		t.Fatalf("Write: %v", err)
	}

	select {
	case ts := <-sub.C():
		if ts.Value != want {
			t.Errorf("roundtrip mismatch: got %+v want %+v", ts.Value, want)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for sample")
	}
}

func TestNewTelemetryPublisher_and_Subscriber(t *testing.T) {
	p := newMockParticipant(t)

	sub, err := NewTelemetrySubscriber(p, "idl/telemetry", dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewTelemetrySubscriber: %v", err)
	}
	defer sub.Close()

	pub, err := NewTelemetryPublisher(p, "idl/telemetry", dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewTelemetryPublisher: %v", err)
	}
	defer pub.Close()

	want := Telemetry{
		Header:      Header{TopicId: "t", Priority: PriorityCRITICAL},
		Values:      [4]float64{1, 2, 3, 4},
		Temperature: 36.6,
		Valid:       true,
		Extras:      []float64{0.1},
	}
	if err := pub.Write(want); err != nil {
		t.Fatalf("Write: %v", err)
	}

	select {
	case ts := <-sub.C():
		if !reflect.DeepEqual(ts.Value, want) {
			t.Errorf("roundtrip mismatch: got %+v want %+v", ts.Value, want)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for sample")
	}
}

func TestNewHeaderPublisher_error_on_closed_participant(t *testing.T) {
	p := newMockParticipant(t)
	_ = p.Close()

	_, err := NewHeaderPublisher(p, "idl/header/err", dds.DefaultQoS)
	if err == nil {
		t.Fatal("expected error from closed participant")
	}
}

func TestNewHeaderSubscriber_error_on_closed_participant(t *testing.T) {
	p := newMockParticipant(t)
	_ = p.Close()

	_, err := NewHeaderSubscriber(p, "idl/header/err", dds.DefaultQoS)
	if err == nil {
		t.Fatal("expected error from closed participant")
	}
}

// ── Unmarshal error paths ─────────────────────────────────────────────────────

// TestHeaderCodec_Unmarshal_BadData covers the NewDecoder error path.
func TestHeaderCodec_Unmarshal_BadData(t *testing.T) {
	_, err := HeaderCodec{}.Unmarshal([]byte{0x00}) // too short for CDR header
	if err == nil {
		t.Fatal("expected error for too-short CDR buffer")
	}
}

// TestTelemetryCodec_Unmarshal_BadData covers the NewDecoder error path.
func TestTelemetryCodec_Unmarshal_BadData(t *testing.T) {
	_, err := TelemetryCodec{}.Unmarshal([]byte{0x00}) // too short for CDR header
	if err == nil {
		t.Fatal("expected error for too-short CDR buffer")
	}
}

// TestNewTelemetryPublisher_error covers the publisher creation error path.
func TestNewTelemetryPublisher_error(t *testing.T) {
	p := newMockParticipant(t)
	_ = p.Close()

	_, err := NewTelemetryPublisher(p, "idl/telemetry/err", dds.DefaultQoS)
	if err == nil {
		t.Fatal("expected error from closed participant")
	}
}

// TestNewTelemetrySubscriber_error covers the subscriber creation error path.
func TestNewTelemetrySubscriber_error(t *testing.T) {
	p := newMockParticipant(t)
	_ = p.Close()

	_, err := NewTelemetrySubscriber(p, "idl/telemetry/err", dds.DefaultQoS)
	if err == nil {
		t.Fatal("expected error from closed participant")
	}
}

// headerBytes returns a canonical marshalled Header for truncation tests.
func headerBytes(t *testing.T) []byte {
	t.Helper()
	data, err := HeaderCodec{}.Marshal(Header{TopicId: "x", TimestampNs: 1, Priority: PriorityLOW})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	return data
}

// telemetryBytes returns a canonical marshalled Telemetry for truncation tests.
func telemetryBytes(t *testing.T) []byte {
	t.Helper()
	data, err := TelemetryCodec{}.Marshal(Telemetry{
		Header:      Header{TopicId: "x", TimestampNs: 1, Priority: PriorityLOW},
		Values:      [4]float64{1, 2, 3, 4},
		Temperature: 1.0,
		Valid:       true,
		Extras:      []float64{5.0},
	})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	return data
}

// TestHeaderCodec_Unmarshal_TruncatedString triggers the ReadString error (string
// length prefix unreachable — only 4-byte CDR header present).
func TestHeaderCodec_Unmarshal_TruncatedString(t *testing.T) {
	data := headerBytes(t)
	_, err := HeaderCodec{}.Unmarshal(data[:4]) // header only, no string length
	if err == nil {
		t.Fatal("expected error for truncated string field")
	}
}

// TestHeaderCodec_Unmarshal_TruncatedInt64 triggers the ReadInt64 error.
// The string "x" occupies bytes 8–9; int64 aligns to offset 16 which is past byte 10.
func TestHeaderCodec_Unmarshal_TruncatedInt64(t *testing.T) {
	data := headerBytes(t)
	_, err := HeaderCodec{}.Unmarshal(data[:10]) // string ok, int64 alignment fails
	if err == nil {
		t.Fatal("expected error for truncated int64 field")
	}
}

// TestHeaderCodec_Unmarshal_TruncatedInt32 triggers the ReadInt32 error.
// int64 occupies bytes 16–23; int32 needs bytes 24–27.
func TestHeaderCodec_Unmarshal_TruncatedInt32(t *testing.T) {
	data := headerBytes(t)
	_, err := HeaderCodec{}.Unmarshal(data[:24]) // int64 ok, int32 truncated
	if err == nil {
		t.Fatal("expected error for truncated int32 field")
	}
}

// TestTelemetryCodec_Unmarshal_TruncatedFloat64Array triggers the ReadFloat64 error
// inside the Values array. float64 Values[0] aligns to offset 32; data[:28] is too short.
func TestTelemetryCodec_Unmarshal_TruncatedFloat64Array(t *testing.T) {
	data := telemetryBytes(t)
	_, err := TelemetryCodec{}.Unmarshal(data[:28]) // Priority ok, float64 align fails
	if err == nil {
		t.Fatal("expected error for truncated float64 array element")
	}
}

// TestTelemetryCodec_Unmarshal_TruncatedFloat32 triggers the ReadFloat32 error.
// All 4 float64 Values occupy bytes 32–63; float32 Temperature needs bytes 64–67.
func TestTelemetryCodec_Unmarshal_TruncatedFloat32(t *testing.T) {
	data := telemetryBytes(t)
	_, err := TelemetryCodec{}.Unmarshal(data[:64]) // Values ok, Temperature truncated
	if err == nil {
		t.Fatal("expected error for truncated float32 field")
	}
}

// TestTelemetryCodec_Unmarshal_TruncatedBool triggers the ReadBool error.
// float32 occupies bytes 64–67; bool Valid is at byte 68.
func TestTelemetryCodec_Unmarshal_TruncatedBool(t *testing.T) {
	data := telemetryBytes(t)
	_, err := TelemetryCodec{}.Unmarshal(data[:68]) // Temperature ok, bool truncated
	if err == nil {
		t.Fatal("expected error for truncated bool field")
	}
}

// TestTelemetryCodec_Unmarshal_TruncatedSequenceLen triggers the ReadUint32 error
// for the Extras sequence length. uint32 aligns to offset 72; data[:69] is too short.
func TestTelemetryCodec_Unmarshal_TruncatedSequenceLen(t *testing.T) {
	data := telemetryBytes(t)
	_, err := TelemetryCodec{}.Unmarshal(data[:69]) // bool ok, sequence length truncated
	if err == nil {
		t.Fatal("expected error for truncated sequence length")
	}
}

// TestTelemetryCodec_Unmarshal_TruncatedSequenceElem triggers the ReadFloat64 error
// for the Extras sequence element. float64 aligns to offset 80; data[:76] is too short.
func TestTelemetryCodec_Unmarshal_TruncatedSequenceElem(t *testing.T) {
	data := telemetryBytes(t)
	_, err := TelemetryCodec{}.Unmarshal(data[:76]) // sequence length=1 ok, element truncated
	if err == nil {
		t.Fatal("expected error for truncated sequence element")
	}
}
