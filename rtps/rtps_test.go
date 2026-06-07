// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Integration tests for rtps.New — uses the public dds.Participant interface.
// Wire-format unit tests live in wire_test.go (package rtps internal).

package rtps_test

import (
	"bytes"
	"testing"
	"time"

	dds "github.com/SoundMatt/go-DDS"
	"github.com/SoundMatt/go-DDS/rtps"
)

// domain 99 avoids port conflicts with any real DDS deployment.
// Ports: meta-multicast=32150, meta-unicast=32160, data-unicast=32161
const testDomain = dds.Domain(99)

// newTestParticipant creates a participant and registers cleanup.
// Skips the test if UDP multicast is unavailable (CI environments).
func newTestParticipant(t *testing.T) dds.Participant {
	t.Helper()
	p, err := rtps.New(testDomain)
	if err != nil {
		t.Skipf("rtps.New: %v — UDP multicast unavailable", err)
	}
	t.Cleanup(func() { p.Close() })
	return p
}

// ── Intra-process pub/sub ─────────────────────────────────────────────────────

func TestRTPS_IntraProcess_PubSub(t *testing.T) {
	p := newTestParticipant(t)

	sub, err := p.NewSubscriber("test/intra/simple", dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewSubscriber: %v", err)
	}
	defer sub.Close()

	pub, err := p.NewPublisher("test/intra/simple", dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
	defer pub.Close()

	want := []byte(`{"sensor":"speed","value":120.5}`)
	if err := pub.Write(want); err != nil {
		t.Fatalf("Write: %v", err)
	}

	select {
	case s := <-sub.C():
		if !bytes.Equal(s.Payload, want) {
			t.Errorf("payload:\n  got  %q\n  want %q", s.Payload, want)
		}
		if s.Topic != "test/intra/simple" {
			t.Errorf("topic: got %q", s.Topic)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for intra-process sample")
	}
}

func TestRTPS_IntraProcess_MultipleSubscribers(t *testing.T) {
	p := newTestParticipant(t)
	const n = 4
	subs := make([]dds.Subscriber, n)
	for i := range subs {
		var err error
		subs[i], err = p.NewSubscriber("test/intra/fanout", dds.DefaultQoS)
		if err != nil {
			t.Fatalf("NewSubscriber[%d]: %v", i, err)
		}
	}
	defer func() {
		for _, s := range subs {
			s.Close()
		}
	}()

	pub, err := p.NewPublisher("test/intra/fanout", dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
	defer pub.Close()

	want := []byte("broadcast")
	if err := pub.Write(want); err != nil {
		t.Fatalf("Write: %v", err)
	}
	for i, sub := range subs {
		select {
		case s := <-sub.C():
			if !bytes.Equal(s.Payload, want) {
				t.Errorf("sub[%d] payload mismatch", i)
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("sub[%d] timeout", i)
		}
	}
}

func TestRTPS_IntraProcess_TopicIsolation(t *testing.T) {
	p := newTestParticipant(t)

	subA, err := p.NewSubscriber("test/intra/topicA", dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewSubscriber(A): %v", err)
	}
	defer subA.Close()

	subB, err := p.NewSubscriber("test/intra/topicB", dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewSubscriber(B): %v", err)
	}
	defer subB.Close()

	pubA, err := p.NewPublisher("test/intra/topicA", dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewPublisher(A): %v", err)
	}
	defer pubA.Close()

	if err := pubA.Write([]byte("for-A-only")); err != nil {
		t.Fatalf("Write: %v", err)
	}

	select {
	case s := <-subA.C():
		if string(s.Payload) != "for-A-only" {
			t.Errorf("subA got %q", s.Payload)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("subA timeout")
	}

	select {
	case s := <-subB.C():
		t.Errorf("subB received unexpected sample: %q", s.Payload)
	default: // correct: no cross-topic delivery
	}
}

func TestRTPS_PayloadIsolation(t *testing.T) {
	p := newTestParticipant(t)

	sub, err := p.NewSubscriber("test/intra/isolation", dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewSubscriber: %v", err)
	}
	defer sub.Close()

	pub, err := p.NewPublisher("test/intra/isolation", dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
	defer pub.Close()

	original := []byte("mutable-payload")
	want := make([]byte, len(original))
	copy(want, original)

	if err := pub.Write(original); err != nil {
		t.Fatalf("Write: %v", err)
	}
	// Mutate after Write — delivered copy must not be affected.
	for i := range original {
		original[i] ^= 0xFF
	}

	select {
	case s := <-sub.C():
		if !bytes.Equal(s.Payload, want) {
			t.Errorf("mutation leaked into delivered sample: got %q, want %q", s.Payload, want)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout")
	}
}

// ── Lifecycle ─────────────────────────────────────────────────────────────────

func TestRTPS_ParticipantClose_BlocksNewEndpoints(t *testing.T) {
	p, err := rtps.New(testDomain)
	if err != nil {
		t.Skipf("rtps.New: %v", err)
	}
	p.Close()

	if _, err := p.NewPublisher("t", dds.DefaultQoS); err == nil {
		t.Error("expected error from closed participant (NewPublisher)")
	}
	if _, err := p.NewSubscriber("t", dds.DefaultQoS); err == nil {
		t.Error("expected error from closed participant (NewSubscriber)")
	}
}

func TestRTPS_WriterClose_ReturnsError(t *testing.T) {
	p := newTestParticipant(t)

	pub, err := p.NewPublisher("test/close/pub", dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
	pub.Close()
	if err := pub.Write([]byte("x")); err == nil {
		t.Error("Write on closed publisher should return error")
	}
}

func TestRTPS_SubscriberClose_ClosesChannel(t *testing.T) {
	p := newTestParticipant(t)

	sub, err := p.NewSubscriber("test/close/sub", dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewSubscriber: %v", err)
	}
	sub.Close()

	select {
	case _, ok := <-sub.C():
		if ok {
			t.Error("channel should be closed after sub.Close()")
		}
	case <-time.After(200 * time.Millisecond):
		t.Error("channel not closed after sub.Close()")
	}
}

func TestRTPS_SubscriberClose_Idempotent(t *testing.T) {
	p := newTestParticipant(t)
	sub, err := p.NewSubscriber("test/idempotent", dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewSubscriber: %v", err)
	}
	sub.Close()
	sub.Close() // must not panic
}

// ── Two-participant loopback ──────────────────────────────────────────────────

// TestRTPS_TwoParticipants_SameHost creates two participants and verifies
// cross-participant delivery via loopback UDP. The test waits for SPDP
// discovery before writing.
func TestRTPS_TwoParticipants_SameHost(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping two-participant test in -short mode (requires network)")
	}

	p1, err := rtps.New(testDomain)
	if err != nil {
		t.Skipf("rtps.New(p1): %v", err)
	}
	defer p1.Close()

	p2, err := rtps.New(testDomain)
	if err != nil {
		t.Skipf("rtps.New(p2): %v", err)
	}
	defer p2.Close()

	sub, err := p2.NewSubscriber("test/cross/basic", dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewSubscriber(p2): %v", err)
	}
	defer sub.Close()

	pub, err := p1.NewPublisher("test/cross/basic", dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewPublisher(p1): %v", err)
	}
	defer pub.Close()

	// Allow SPDP + SEDP to complete (within the 2 s announce period).
	time.Sleep(2200 * time.Millisecond)

	want := []byte(`{"rtps":"cross-participant","ok":true}`)
	if err := pub.Write(want); err != nil {
		t.Fatalf("Write: %v", err)
	}

	select {
	case s := <-sub.C():
		if !bytes.Equal(s.Payload, want) {
			t.Errorf("payload: got %q, want %q", s.Payload, want)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timeout: cross-participant sample not received")
	}
}
