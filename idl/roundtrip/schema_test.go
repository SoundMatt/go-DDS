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
