// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Integration tests for rtps.New — uses the public dds.Participant interface.
// Wire-format unit tests live in wire_test.go (package rtps internal).

package rtps_test

import (
	"bytes"
	"context"
	"runtime"
	"testing"
	"time"

	dds "github.com/SoundMatt/go-DDS"
	"github.com/SoundMatt/go-DDS/rtps"
	"github.com/SoundMatt/go-DDS/security"
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
	if runtime.GOOS != "linux" {
		t.Skipf("UDP multicast loopback unreliable on %s; skipping cross-participant test", runtime.GOOS)
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

// ── WaitSet ───────────────────────────────────────────────────────────────────

func TestWaitSet_ReceiveFromFirst(t *testing.T) {
	p := newTestParticipant(t)

	subA, _ := p.NewSubscriber("waitset/a", dds.DefaultQoS)
	subB, _ := p.NewSubscriber("waitset/b", dds.DefaultQoS)
	defer subA.Close()
	defer subB.Close()

	pubA, _ := p.NewPublisher("waitset/a", dds.DefaultQoS)
	defer pubA.Close()

	ws := dds.NewWaitSet(subA, subB)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := pubA.Write([]byte("ping")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	s, sub, err := ws.Wait(ctx)
	if err != nil {
		t.Fatalf("Wait: %v", err)
	}
	if sub != subA {
		t.Error("expected sample from subA")
	}
	if string(s.Payload) != "ping" {
		t.Errorf("payload: got %q", s.Payload)
	}
}

func TestWaitSet_ReceiveFromSecond(t *testing.T) {
	p := newTestParticipant(t)

	subA, _ := p.NewSubscriber("waitset/only-b/a", dds.DefaultQoS)
	subB, _ := p.NewSubscriber("waitset/only-b/b", dds.DefaultQoS)
	defer subA.Close()
	defer subB.Close()

	pubB, _ := p.NewPublisher("waitset/only-b/b", dds.DefaultQoS)
	defer pubB.Close()

	ws := dds.NewWaitSet(subA, subB)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := pubB.Write([]byte("from-b")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	s, sub, err := ws.Wait(ctx)
	if err != nil {
		t.Fatalf("Wait: %v", err)
	}
	if sub != subB {
		t.Error("expected sample from subB")
	}
	if string(s.Payload) != "from-b" {
		t.Errorf("payload: got %q", s.Payload)
	}
}

func TestWaitSet_ContextCancellation(t *testing.T) {
	p := newTestParticipant(t)

	sub, _ := p.NewSubscriber("waitset/cancel", dds.DefaultQoS)
	defer sub.Close()

	ws := dds.NewWaitSet(sub)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, _, err := ws.Wait(ctx)
	if err == nil {
		t.Error("expected context error when no sample arrives")
	}
}

func TestWaitSet_ClosedSubscriber(t *testing.T) {
	p := newTestParticipant(t)

	sub, _ := p.NewSubscriber("waitset/closed", dds.DefaultQoS)
	ws := dds.NewWaitSet(sub)

	// Close the subscriber so its channel is closed.
	sub.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	// Wait should return (not spin) when all channels are closed.
	_, _, _ = ws.Wait(ctx)
	// No assertion on error — just must not hang.
}

// ── Reliable QoS (intra-process, happy path) ──────────────────────────────────

func TestRTPS_Reliable_IntraProcess(t *testing.T) {
	p := newTestParticipant(t)

	sub, err := p.NewSubscriber("reliable/intra", dds.ReliableQoS)
	if err != nil {
		t.Fatalf("NewSubscriber: %v", err)
	}
	defer sub.Close()

	pub, err := p.NewPublisher("reliable/intra", dds.ReliableQoS)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
	defer pub.Close()

	const n = 10
	for i := 0; i < n; i++ {
		if err := pub.Write([]byte{byte(i)}); err != nil {
			t.Fatalf("Write(%d): %v", i, err)
		}
	}
	for i := 0; i < n; i++ {
		select {
		case s := <-sub.C():
			if s.Payload[0] != byte(i) {
				t.Errorf("sample %d: got %d", i, s.Payload[0])
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("timeout waiting for sample %d", i)
		}
	}
}

// ── Security ──────────────────────────────────────────────────────────────────

func TestRTPS_Security_NullPlugin(t *testing.T) {
	p, err := rtps.New(testDomain, rtps.WithSecurity(security.NullPlugin{}))
	if err != nil {
		t.Skipf("rtps.New: %v", err)
	}
	t.Cleanup(func() { p.Close() })

	sub, _ := p.NewSubscriber("security/null", dds.DefaultQoS)
	pub, _ := p.NewPublisher("security/null", dds.DefaultQoS)
	defer sub.Close()
	defer pub.Close()

	want := []byte("secured-null")
	if err := pub.Write(want); err != nil {
		t.Fatalf("Write: %v", err)
	}
	select {
	case s := <-sub.C():
		if !bytes.Equal(s.Payload, want) {
			t.Errorf("payload: got %q, want %q", s.Payload, want)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout")
	}
}

func TestRTPS_Security_HMAC(t *testing.T) {
	key := security.NewRandomKey(32)
	plugin := security.NewHMACPlugin(key)
	p, err := rtps.New(testDomain, rtps.WithSecurity(plugin))
	if err != nil {
		t.Skipf("rtps.New: %v", err)
	}
	t.Cleanup(func() { p.Close() })

	sub, _ := p.NewSubscriber("security/hmac", dds.DefaultQoS)
	pub, _ := p.NewPublisher("security/hmac", dds.DefaultQoS)
	defer sub.Close()
	defer pub.Close()

	want := []byte("integrity-protected")
	if err := pub.Write(want); err != nil {
		t.Fatalf("Write: %v", err)
	}
	select {
	case s := <-sub.C():
		if !bytes.Equal(s.Payload, want) {
			t.Errorf("payload: got %q, want %q", s.Payload, want)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout")
	}
}

func TestRTPS_Security_AESGCM(t *testing.T) {
	key := security.NewRandomKey(32)
	plugin, err := security.NewAESGCMPlugin(key)
	if err != nil {
		t.Fatalf("NewAESGCMPlugin: %v", err)
	}
	p, err := rtps.New(testDomain, rtps.WithSecurity(plugin))
	if err != nil {
		t.Skipf("rtps.New: %v", err)
	}
	t.Cleanup(func() { p.Close() })

	sub, _ := p.NewSubscriber("security/aesgcm", dds.DefaultQoS)
	pub, _ := p.NewPublisher("security/aesgcm", dds.DefaultQoS)
	defer sub.Close()
	defer pub.Close()

	want := []byte(`{"encrypted":true,"value":42}`)
	if err := pub.Write(want); err != nil {
		t.Fatalf("Write: %v", err)
	}
	select {
	case s := <-sub.C():
		if !bytes.Equal(s.Payload, want) {
			t.Errorf("payload: got %q, want %q", s.Payload, want)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout")
	}
}
