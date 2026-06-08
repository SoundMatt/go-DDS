// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package safety_test

//fusa:test REQ-SAFETY-015
//fusa:test REQ-SAFETY-016
//fusa:test REQ-SAFETY-017

import (
	"testing"
	"time"

	dds "github.com/SoundMatt/go-DDS"
	"github.com/SoundMatt/go-DDS/mock"
	"github.com/SoundMatt/go-DDS/safety"
)

func TestE2ESubscriber_SafetyMetrics_CRCFailure(t *testing.T) {
	p, err := mock.New(dds.Domain(0))
	if err != nil {
		t.Fatalf("mock.New: %v", err)
	}
	defer p.Close()

	pub, err := p.NewPublisher("metrics/crc", dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
	defer pub.Close()

	sub, err := p.NewSubscriber("metrics/crc", dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewSubscriber: %v", err)
	}
	defer sub.Close()

	cfg := safety.E2EConfig{DataID: 1, SourceID: 1, Topic: "metrics/crc"}
	esub := safety.NewE2ESubscriber(sub, cfg)
	defer esub.Close()

	// Write a raw payload that is too short for the E2E header — triggers ErrHeaderTooShort.
	_ = pub.Write([]byte("short"))
	time.Sleep(50 * time.Millisecond)

	snap := esub.SafetyMetrics()
	if snap.HeaderTooShort != 1 {
		t.Errorf("HeaderTooShort = %d, want 1", snap.HeaderTooShort)
	}
	if snap.Topic != "metrics/crc" {
		t.Errorf("Topic = %q, want metrics/crc", snap.Topic)
	}
}

func TestE2ESubscriber_SafetyMetrics_ValidSamples(t *testing.T) {
	p, err := mock.New(dds.Domain(0))
	if err != nil {
		t.Fatalf("mock.New: %v", err)
	}
	defer p.Close()

	pub, err := p.NewPublisher("metrics/valid", dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
	defer pub.Close()

	sub, err := p.NewSubscriber("metrics/valid", dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewSubscriber: %v", err)
	}
	defer sub.Close()

	cfg := safety.E2EConfig{DataID: 5, SourceID: 5, Topic: "metrics/valid"}
	epub := safety.NewE2EPublisher(pub, cfg)
	defer epub.Close()

	esub := safety.NewE2ESubscriber(sub, cfg)
	defer esub.Close()

	const n = 3
	for i := 0; i < n; i++ {
		if err := epub.Write([]byte("hello")); err != nil {
			t.Fatalf("Write: %v", err)
		}
	}
	for i := 0; i < n; i++ {
		<-esub.C()
	}

	snap := esub.SafetyMetrics()
	if snap.ValidSamples != n {
		t.Errorf("ValidSamples = %d, want %d", snap.ValidSamples, n)
	}
	if snap.CRCFailures != 0 {
		t.Errorf("CRCFailures = %d, want 0", snap.CRCFailures)
	}
}

func TestE2ESubscriber_SafetyEvents_Channel(t *testing.T) {
	p, err := mock.New(dds.Domain(0))
	if err != nil {
		t.Fatalf("mock.New: %v", err)
	}
	defer p.Close()

	pub, err := p.NewPublisher("metrics/events", dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
	defer pub.Close()

	sub, err := p.NewSubscriber("metrics/events", dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewSubscriber: %v", err)
	}
	defer sub.Close()

	cfg := safety.E2EConfig{DataID: 9, SourceID: 9, Topic: "metrics/events"}
	esub := safety.NewE2ESubscriber(sub, cfg)
	defer esub.Close()

	// Write garbage to trigger a header-too-short event.
	_ = pub.Write([]byte("x"))

	select {
	case ev := <-esub.SafetyEvents():
		if ev.Kind != safety.SafetyEventHeaderTooShort {
			t.Errorf("event kind = %v, want SafetyEventHeaderTooShort", ev.Kind)
		}
		if ev.Topic != "metrics/events" {
			t.Errorf("event topic = %q, want metrics/events", ev.Topic)
		}
	case <-time.After(500 * time.Millisecond):
		t.Error("timeout waiting for safety event")
	}
}

func TestSafetyEventKind_String(t *testing.T) {
	cases := []struct {
		k    safety.SafetyEventKind
		want string
	}{
		{safety.SafetyEventCRCFailure, "crc_failure"},
		{safety.SafetyEventSequenceGap, "sequence_gap"},
		{safety.SafetyEventStaleSample, "stale_sample"},
		{safety.SafetyEventHeaderTooShort, "header_too_short"},
		{safety.SafetyEventSchemaViolation, "schema_violation"},
	}
	for _, tc := range cases {
		if got := tc.k.String(); got != tc.want {
			t.Errorf("%v.String() = %q, want %q", tc.k, got, tc.want)
		}
	}
}

func TestE2ESubscriber_ImplementsSafetyMetricsProvider(t *testing.T) {
	p, err := mock.New(dds.Domain(0))
	if err != nil {
		t.Fatalf("mock.New: %v", err)
	}
	defer p.Close()
	sub, err := p.NewSubscriber("t", dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewSubscriber: %v", err)
	}
	defer sub.Close()
	esub := safety.NewE2ESubscriber(sub, safety.E2EConfig{})
	defer esub.Close()
	var _ safety.SafetyMetricsProvider = esub
}
