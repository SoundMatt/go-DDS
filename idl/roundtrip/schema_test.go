// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package idlgen

import (
	"reflect"
	"testing"
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
