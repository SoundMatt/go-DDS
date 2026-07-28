// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Tests for Milestone 14 "QoS Enforcement — Active Policy" in the mock
// backend: Liveliness, Ownership, Partition, and the Time-Based Filter.

package mock_test

import (
	"sync"
	"testing"
	"time"

	dds "github.com/SoundMatt/go-DDS"
	"github.com/SoundMatt/go-DDS/mock"
)

func drainOneQoS(t *testing.T, ch <-chan dds.Sample, timeout time.Duration) (dds.Sample, bool) {
	t.Helper()
	select {
	case s, ok := <-ch:
		return s, ok
	case <-time.After(timeout):
		return dds.Sample{}, false
	}
}

func expectNoneQoS(t *testing.T, ch <-chan dds.Sample, wait time.Duration) {
	t.Helper()
	select {
	case s, ok := <-ch:
		if ok {
			t.Fatalf("expected no delivery, got sample %q", s.Payload)
		}
	case <-time.After(wait):
	}
}

// ── Partition ─────────────────────────────────────────────────────────────────

func TestMockPartition_Gating(t *testing.T) {
	p, err := mock.New(0, mock.IsolatedBroker())
	if err != nil {
		t.Fatalf("mock.New: %v", err)
	}
	defer p.Close()

	subA, err := p.NewSubscriber("part/topic", dds.QoS{Partition: []string{"A"}})
	if err != nil {
		t.Fatalf("NewSubscriber A: %v", err)
	}
	defer subA.Close()
	subB, err := p.NewSubscriber("part/topic", dds.QoS{Partition: []string{"B"}})
	if err != nil {
		t.Fatalf("NewSubscriber B: %v", err)
	}
	defer subB.Close()
	subDefault, err := p.NewSubscriber("part/topic", dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewSubscriber default: %v", err)
	}
	defer subDefault.Close()

	pubA, err := p.NewPublisher("part/topic", dds.QoS{Partition: []string{"A"}})
	if err != nil {
		t.Fatalf("NewPublisher A: %v", err)
	}
	defer pubA.Close()

	if err := pubA.Write([]byte("hello")); err != nil {
		t.Fatalf("Write: %v", err)
	}

	if s, ok := drainOneQoS(t, subA.C(), time.Second); !ok || string(s.Payload) != "hello" {
		t.Fatalf("subA: expected delivery, got %v ok=%v", s, ok)
	}
	expectNoneQoS(t, subB.C(), 100*time.Millisecond)
	expectNoneQoS(t, subDefault.C(), 100*time.Millisecond)
}

// ── Ownership ─────────────────────────────────────────────────────────────────

func TestMockOwnership_HighestStrengthWinsAndFailsOver(t *testing.T) {
	p, err := mock.New(0, mock.IsolatedBroker())
	if err != nil {
		t.Fatalf("mock.New: %v", err)
	}
	defer p.Close()

	sub, err := p.NewSubscriber("own/topic", dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewSubscriber: %v", err)
	}
	defer sub.Close()

	low, err := p.NewPublisher("own/topic", dds.QoS{Ownership: dds.ExclusiveOwnership, OwnershipStrength: 1})
	if err != nil {
		t.Fatalf("NewPublisher low: %v", err)
	}
	defer low.Close()
	high, err := p.NewPublisher("own/topic", dds.QoS{Ownership: dds.ExclusiveOwnership, OwnershipStrength: 10})
	if err != nil {
		t.Fatalf("NewPublisher high: %v", err)
	}

	if err := low.Write([]byte("from-low")); err != nil {
		t.Fatalf("Write low: %v", err)
	}
	expectNoneQoS(t, sub.C(), 100*time.Millisecond)

	if err := high.Write([]byte("from-high")); err != nil {
		t.Fatalf("Write high: %v", err)
	}
	if s, ok := drainOneQoS(t, sub.C(), time.Second); !ok || string(s.Payload) != "from-high" {
		t.Fatalf("expected delivery from high-strength writer, got %v ok=%v", s, ok)
	}

	if err := high.Close(); err != nil {
		t.Fatalf("Close high: %v", err)
	}
	if err := low.Write([]byte("failover")); err != nil {
		t.Fatalf("Write low after failover: %v", err)
	}
	if s, ok := drainOneQoS(t, sub.C(), time.Second); !ok || string(s.Payload) != "failover" {
		t.Fatalf("expected delivery from low-strength writer after failover, got %v ok=%v", s, ok)
	}
}

func TestMockOwnership_SharedIsUnaffected(t *testing.T) {
	p, err := mock.New(0, mock.IsolatedBroker())
	if err != nil {
		t.Fatalf("mock.New: %v", err)
	}
	defer p.Close()

	sub, err := p.NewSubscriber("own/shared", dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewSubscriber: %v", err)
	}
	defer sub.Close()

	pubA, err := p.NewPublisher("own/shared", dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewPublisher A: %v", err)
	}
	defer pubA.Close()
	pubB, err := p.NewPublisher("own/shared", dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewPublisher B: %v", err)
	}
	defer pubB.Close()

	if err := pubA.Write([]byte("a")); err != nil {
		t.Fatalf("Write A: %v", err)
	}
	if err := pubB.Write([]byte("b")); err != nil {
		t.Fatalf("Write B: %v", err)
	}
	got := map[string]bool{}
	for i := 0; i < 2; i++ {
		s, ok := drainOneQoS(t, sub.C(), time.Second)
		if !ok {
			t.Fatalf("expected 2 deliveries under SharedOwnership, got %d", i)
		}
		got[string(s.Payload)] = true
	}
	if !got["a"] || !got["b"] {
		t.Fatalf("expected both writers' samples delivered, got %v", got)
	}
}

// ── Liveliness ────────────────────────────────────────────────────────────────

func TestMockLiveliness_AutomaticAssertionKeepsWriterAlive(t *testing.T) {
	p, err := mock.New(0, mock.IsolatedBroker())
	if err != nil {
		t.Fatalf("mock.New: %v", err)
	}
	defer p.Close()

	var mu sync.Mutex
	var lost []dds.GUID
	sub, err := p.NewSubscriber("live/auto", dds.DefaultQoS, dds.WithLivelinessLost(func(g dds.GUID) {
		mu.Lock()
		lost = append(lost, g)
		mu.Unlock()
	}))
	if err != nil {
		t.Fatalf("NewSubscriber: %v", err)
	}
	defer sub.Close()

	pub, err := p.NewPublisher("live/auto", dds.QoS{
		Liveliness:              dds.AutomaticLiveliness,
		LivelinessLeaseDuration: 40 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
	defer pub.Close()

	time.Sleep(250 * time.Millisecond)

	mu.Lock()
	n := len(lost)
	mu.Unlock()
	if n != 0 {
		t.Fatalf("expected 0 LivelinessLost callbacks for an actively-asserting writer, got %d", n)
	}
}

func TestMockLiveliness_ManualByTopicFiresWhenSilent(t *testing.T) {
	p, err := mock.New(0, mock.IsolatedBroker())
	if err != nil {
		t.Fatalf("mock.New: %v", err)
	}
	defer p.Close()

	var mu sync.Mutex
	var lost []dds.GUID
	sub, err := p.NewSubscriber("live/manual", dds.DefaultQoS, dds.WithLivelinessLost(func(g dds.GUID) {
		mu.Lock()
		lost = append(lost, g)
		mu.Unlock()
	}))
	if err != nil {
		t.Fatalf("NewSubscriber: %v", err)
	}
	defer sub.Close()

	pub, err := p.NewPublisher("live/manual", dds.QoS{
		Liveliness:              dds.ManualByTopicLiveliness,
		LivelinessLeaseDuration: 40 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
	defer pub.Close()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		n := len(lost)
		mu.Unlock()
		if n > 0 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("expected LivelinessLost to fire for a silent ManualByTopicLiveliness writer")
}

// ── Time-Based Filter ─────────────────────────────────────────────────────────

func TestMockTimeBasedFilter_DropsFastSamples(t *testing.T) {
	p, err := mock.New(0, mock.IsolatedBroker())
	if err != nil {
		t.Fatalf("mock.New: %v", err)
	}
	defer p.Close()

	sub, err := p.NewSubscriber("tbf/topic", dds.QoS{MinSeparation: 200 * time.Millisecond})
	if err != nil {
		t.Fatalf("NewSubscriber: %v", err)
	}
	defer sub.Close()

	pub, err := p.NewPublisher("tbf/topic", dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
	defer pub.Close()

	if err := pub.Write([]byte("first")); err != nil {
		t.Fatalf("Write first: %v", err)
	}
	if s, ok := drainOneQoS(t, sub.C(), time.Second); !ok || string(s.Payload) != "first" {
		t.Fatalf("expected first sample delivered, got %v ok=%v", s, ok)
	}

	if err := pub.Write([]byte("second")); err != nil {
		t.Fatalf("Write second: %v", err)
	}
	expectNoneQoS(t, sub.C(), 100*time.Millisecond)

	time.Sleep(200 * time.Millisecond)
	if err := pub.Write([]byte("third")); err != nil {
		t.Fatalf("Write third: %v", err)
	}
	if s, ok := drainOneQoS(t, sub.C(), time.Second); !ok || string(s.Payload) != "third" {
		t.Fatalf("expected third sample delivered after MinSeparation elapsed, got %v ok=%v", s, ok)
	}
}

func TestMockTimeBasedFilter_Disabled(t *testing.T) {
	p, err := mock.New(0, mock.IsolatedBroker())
	if err != nil {
		t.Fatalf("mock.New: %v", err)
	}
	defer p.Close()

	sub, err := p.NewSubscriber("tbf/off", dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewSubscriber: %v", err)
	}
	defer sub.Close()
	pub, err := p.NewPublisher("tbf/off", dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
	defer pub.Close()

	for i := 0; i < 3; i++ {
		if err := pub.Write([]byte("x")); err != nil {
			t.Fatalf("Write: %v", err)
		}
	}
	for i := 0; i < 3; i++ {
		if _, ok := drainOneQoS(t, sub.C(), time.Second); !ok {
			t.Fatalf("expected all 3 samples delivered with MinSeparation disabled, got %d", i)
		}
	}
}
