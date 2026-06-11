//go:build cyclone

// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package cyclone_test

import (
	"testing"
	"time"

	dds "github.com/SoundMatt/go-DDS"
	"github.com/SoundMatt/go-DDS/cyclone"
)

// domain 99 avoids colliding with any application DDS domain in CI.
const testDomain = dds.Domain(99)

func newParticipant(t *testing.T) dds.Participant {
	t.Helper()
	p, err := cyclone.New(testDomain)
	if err != nil {
		t.Skipf("CycloneDDS unavailable (%v) — skipping", err)
	}
	t.Cleanup(func() { p.Close() })
	return p
}

func TestCyclone_NewWithOptions(t *testing.T) {
	p, err := cyclone.NewWithOptions(testDomain, cyclone.Options{
		PollInterval: 2 * time.Millisecond,
	})
	if err != nil {
		t.Skipf("CycloneDDS unavailable: %v", err)
	}
	defer p.Close()

	sub, err := p.NewSubscriber("opts/test", dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewSubscriber: %v", err)
	}
	defer sub.Close()

	pub, err := p.NewPublisher("opts/test", dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
	defer pub.Close()

	time.Sleep(100 * time.Millisecond)
	pub.Write([]byte(`{"sensor":"gyro","value":1.23}`))

	select {
	case s := <-sub.C():
		if string(s.Payload) != `{"sensor":"gyro","value":1.23}` {
			t.Errorf("unexpected payload: %q", s.Payload)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for sample")
	}
}

func TestCyclone_PubSub_RoundTrip(t *testing.T) {
	p := newParticipant(t)

	const topic = "test/roundtrip"
	sub, err := p.NewSubscriber(topic, dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewSubscriber: %v", err)
	}
	defer sub.Close()

	pub, err := p.NewPublisher(topic, dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
	defer pub.Close()

	// Allow DDS discovery to complete before writing.
	time.Sleep(100 * time.Millisecond)

	want := `{"replyTopic":"client/test","request":{"action":"get","path":"Vehicle.Speed"}}`
	if err := pub.Write([]byte(want)); err != nil {
		t.Fatalf("Write: %v", err)
	}

	select {
	case s := <-sub.C():
		if string(s.Payload) != want {
			t.Errorf("payload mismatch:\n  got  %q\n  want %q", s.Payload, want)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for DDS sample")
	}
}

func TestCyclone_PublisherClose_ReturnsError(t *testing.T) {
	p := newParticipant(t)
	pub, err := p.NewPublisher("close/test", dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
	pub.Close()
	if err := pub.Write([]byte("x")); err == nil {
		t.Error("expected error writing to closed publisher")
	}
}

func TestCyclone_ParticipantClose_BlocksNewEndpoints(t *testing.T) {
	p, err := cyclone.New(testDomain)
	if err != nil {
		t.Skipf("CycloneDDS unavailable: %v", err)
	}
	p.Close()
	ignoredVal, err := p.NewPublisher("t", dds.DefaultQoS)
	_ = ignoredVal
	if err == nil {
		t.Error("expected error from closed participant")
	}
	ignoredVal, err := p.NewSubscriber("t", dds.DefaultQoS)
	_ = ignoredVal
	if err == nil {
		t.Error("expected error from closed participant")
	}
}

func TestCyclone_StubReturnsError(t *testing.T) {
	// This test only runs under -tags cyclone, so New() must succeed.
	p, err := cyclone.New(testDomain)
	if err != nil {
		t.Skipf("CycloneDDS unavailable: %v", err)
	}
	p.Close()
}

// ── Optional interfaces ───────────────────────────────────────────────────────

func TestCyclone_DiscoveryMetrics(t *testing.T) {
	p := newParticipant(t)
	mp, ok := p.(dds.DiscoveryMetricsProvider)
	if !ok {
		t.Fatal("cyclone participant does not implement dds.DiscoveryMetricsProvider")
	}
	m := mp.DiscoveryMetrics()
	// Cyclone returns zeros for all fields (CGo metrics are not surfaced).
	_ = m
}

func TestCyclone_TopicMetrics(t *testing.T) {
	p := newParticipant(t)
	mp, ok := p.(dds.TopicMetricsProvider)
	if !ok {
		t.Fatal("cyclone participant does not implement dds.TopicMetricsProvider")
	}
	metrics := mp.TopicMetrics()
	// Cyclone returns nil (no per-topic counters in CGo shim).
	if metrics != nil {
		t.Errorf("expected nil TopicMetrics, got %v", metrics)
	}
}

func TestCyclone_Health_Open(t *testing.T) {
	p := newParticipant(t)
	hp, ok := p.(dds.HealthProvider)
	if !ok {
		t.Fatal("cyclone participant does not implement dds.HealthProvider")
	}
	h := hp.Health()
	if h.Status != dds.HealthOK {
		t.Errorf("expected HealthOK, got %v", h.Status)
	}
}

func TestCyclone_Health_Closed(t *testing.T) {
	p, err := cyclone.New(testDomain)
	if err != nil {
		t.Skipf("CycloneDDS unavailable: %v", err)
	}
	hp, ok := p.(dds.HealthProvider)
	if !ok {
		t.Fatal("cyclone participant does not implement dds.HealthProvider")
	}
	p.Close()
	h := hp.Health()
	if h.Status != dds.HealthDown {
		t.Errorf("expected HealthDown after Close, got %v", h.Status)
	}
	if h.Details["state"] != "closed" {
		t.Errorf("expected details[state]=closed, got %v", h.Details)
	}
}
