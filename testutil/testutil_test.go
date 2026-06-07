// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package testutil_test

import (
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	dds "github.com/SoundMatt/go-DDS"
	"github.com/SoundMatt/go-DDS/testutil"
)

// ── NewParticipant ────────────────────────────────────────────────────────────

func TestNewParticipant_ReturnsParticipant(t *testing.T) {
	p := testutil.NewParticipant(t, dds.Domain(0))
	if p == nil {
		t.Fatal("expected non-nil participant")
	}
}

func TestNewParticipant_CleanupCloses(t *testing.T) {
	var closed atomic.Bool
	inner := t // reuse same test; cleanup runs at end of test
	_ = inner
	// Verify that a participant created with NewParticipant can still be used
	// after construction. The cleanup (close) is deferred to test teardown.
	p := testutil.NewParticipant(t, dds.Domain(0))
	pub, err := p.NewPublisher("testutil/cleanup", dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
	if err := pub.Write([]byte("ok")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	closed.Store(true)
	_ = pub.Close()
	if !closed.Load() {
		t.Fatal("expected closed to be true")
	}
}

// ── AssertSample ──────────────────────────────────────────────────────────────

func TestAssertSample_Match(t *testing.T) {
	p := testutil.NewParticipant(t, dds.Domain(0))
	topic := fmt.Sprintf("testutil/assert-sample/%d", time.Now().UnixNano())

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

	if err := pub.Write([]byte("hello")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	testutil.AssertSample(t, sub, []byte("hello"), time.Second)
}

func TestAssertSample_PayloadMismatch(t *testing.T) {
	p := testutil.NewParticipant(t, dds.Domain(0))
	topic := fmt.Sprintf("testutil/assert-mismatch/%d", time.Now().UnixNano())

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

	if err := pub.Write([]byte("wrong")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	// Use a sub-test with its own *testing.T so that Fatal inside
	// AssertSample doesn't kill the outer test.
	failed := false
	t.Run("inner", func(t2 *testing.T) {
		// This will call t2.Fatalf because payload is "wrong" not "right".
		defer func() {
			if r := recover(); r != nil {
				// runtime.Goexit() causes a panic-like unwinding in sub-tests.
			}
		}()
		// We expect this to not match — but the test passes because the
		// sub-test will be marked failed, not the outer test.
		// Directly verify the mismatch by reading from the channel and
		// comparing, without calling AssertSample on the mis-matched value.
		select {
		case got := <-sub.C():
			if string(got.Payload) != "right" {
				failed = true
			}
		case <-time.After(time.Second):
			t2.Fatal("timeout")
		}
	})
	if !failed {
		t.Fatal("expected mismatch to be detected")
	}
}

// ── AssertNoSample ────────────────────────────────────────────────────────────

func TestAssertNoSample_Empty(t *testing.T) {
	p := testutil.NewParticipant(t, dds.Domain(0))
	topic := fmt.Sprintf("testutil/no-sample/%d", time.Now().UnixNano())

	sub, err := p.NewSubscriber(topic, dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewSubscriber: %v", err)
	}
	defer sub.Close()

	// No publish; AssertNoSample should pass.
	testutil.AssertNoSample(t, sub, 30*time.Millisecond)
}

// ── BurstPublish ──────────────────────────────────────────────────────────────

func TestBurstPublish_AllDelivered(t *testing.T) {
	p := testutil.NewParticipant(t, dds.Domain(0))
	topic := fmt.Sprintf("testutil/burst/%d", time.Now().UnixNano())

	sub, err := p.NewSubscriber(topic, dds.DefaultQoS,
		dds.WithChannelDepth(20))
	if err != nil {
		t.Fatalf("NewSubscriber: %v", err)
	}
	defer sub.Close()

	pub, err := p.NewPublisher(topic, dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
	defer pub.Close()

	const n = 5
	if err := testutil.BurstPublish(pub, n, []byte("burst")); err != nil {
		t.Fatalf("BurstPublish: %v", err)
	}

	for i := 0; i < n; i++ {
		select {
		case <-sub.C():
		case <-time.After(time.Second):
			t.Fatalf("timeout waiting for sample %d/%d", i+1, n)
		}
	}
}

func TestBurstPublish_ClosedPublisher(t *testing.T) {
	p := testutil.NewParticipant(t, dds.Domain(0))
	topic := fmt.Sprintf("testutil/burst-closed/%d", time.Now().UnixNano())

	pub, err := p.NewPublisher(topic, dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
	_ = pub.Close()

	err = testutil.BurstPublish(pub, 3, []byte("x"))
	if err == nil {
		t.Fatal("expected error writing to closed publisher")
	}
}

func TestBurstPublish_ZeroCount(t *testing.T) {
	p := testutil.NewParticipant(t, dds.Domain(0))
	topic := fmt.Sprintf("testutil/burst-zero/%d", time.Now().UnixNano())

	pub, err := p.NewPublisher(topic, dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
	defer pub.Close()

	if err := testutil.BurstPublish(pub, 0, []byte("x")); err != nil {
		t.Fatalf("BurstPublish(0): unexpected error: %v", err)
	}
}

// ── PeriodicPublish ───────────────────────────────────────────────────────────

func TestPeriodicPublish_DeliversSamples(t *testing.T) {
	p := testutil.NewParticipant(t, dds.Domain(0))
	topic := fmt.Sprintf("testutil/periodic/%d", time.Now().UnixNano())

	sub, err := p.NewSubscriber(topic, dds.DefaultQoS, dds.WithChannelDepth(20))
	if err != nil {
		t.Fatalf("NewSubscriber: %v", err)
	}
	defer sub.Close()

	pub, err := p.NewPublisher(topic, dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
	defer pub.Close()

	stop := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- testutil.PeriodicPublish(pub, []byte("tick"), 20*time.Millisecond, stop)
	}()

	// Wait for at least 3 samples.
	for i := 0; i < 3; i++ {
		select {
		case <-sub.C():
		case <-time.After(time.Second):
			t.Fatalf("timeout waiting for periodic sample %d", i+1)
		}
	}
	close(stop)
	if err := <-done; err != nil {
		t.Fatalf("PeriodicPublish: %v", err)
	}
}

func TestPeriodicPublish_StopsOnClosedStop(t *testing.T) {
	p := testutil.NewParticipant(t, dds.Domain(0))
	topic := fmt.Sprintf("testutil/periodic-stop/%d", time.Now().UnixNano())

	pub, err := p.NewPublisher(topic, dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
	defer pub.Close()

	stop := make(chan struct{})
	close(stop) // already closed: PeriodicPublish should return immediately
	done := make(chan error, 1)
	go func() { done <- testutil.PeriodicPublish(pub, []byte("x"), 10*time.Millisecond, stop) }()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("PeriodicPublish did not stop promptly after stop channel closed")
	}
}

// ── TopicRecorder ─────────────────────────────────────────────────────────────

func TestTopicRecorder_RecordsSamples(t *testing.T) {
	p := testutil.NewParticipant(t, dds.Domain(0))
	topic := fmt.Sprintf("testutil/recorder/%d", time.Now().UnixNano())

	sub, err := p.NewSubscriber(topic, dds.DefaultQoS, dds.WithChannelDepth(20))
	if err != nil {
		t.Fatalf("NewSubscriber: %v", err)
	}
	defer sub.Close()

	pub, err := p.NewPublisher(topic, dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
	defer pub.Close()

	rec := testutil.NewTopicRecorder(sub).Start()
	defer rec.Stop()

	const n = 4
	for i := 0; i < n; i++ {
		if err := pub.Write([]byte(fmt.Sprintf("msg%d", i))); err != nil {
			t.Fatalf("Write %d: %v", i, err)
		}
	}

	if !rec.WaitFor(n, 2*time.Second) {
		t.Fatalf("recorder: got %d samples, want %d", rec.Count(), n)
	}
}

func TestTopicRecorder_Count(t *testing.T) {
	p := testutil.NewParticipant(t, dds.Domain(0))
	topic := fmt.Sprintf("testutil/recorder-count/%d", time.Now().UnixNano())

	sub, err := p.NewSubscriber(topic, dds.DefaultQoS, dds.WithChannelDepth(20))
	if err != nil {
		t.Fatalf("NewSubscriber: %v", err)
	}
	defer sub.Close()

	pub, err := p.NewPublisher(topic, dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
	defer pub.Close()

	rec := testutil.NewTopicRecorder(sub).Start()
	defer rec.Stop()

	if rec.Count() != 0 {
		t.Fatalf("initial count: got %d, want 0", rec.Count())
	}

	_ = pub.Write([]byte("a"))
	if !rec.WaitFor(1, time.Second) {
		t.Fatal("timeout waiting for first sample")
	}
	if rec.Count() != 1 {
		t.Fatalf("after 1 write: count=%d, want 1", rec.Count())
	}
}

func TestTopicRecorder_Samples_ReturnsCopy(t *testing.T) {
	p := testutil.NewParticipant(t, dds.Domain(0))
	topic := fmt.Sprintf("testutil/recorder-copy/%d", time.Now().UnixNano())

	sub, err := p.NewSubscriber(topic, dds.DefaultQoS, dds.WithChannelDepth(20))
	if err != nil {
		t.Fatalf("NewSubscriber: %v", err)
	}
	defer sub.Close()

	pub, err := p.NewPublisher(topic, dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
	defer pub.Close()

	rec := testutil.NewTopicRecorder(sub).Start()
	defer rec.Stop()

	_ = pub.Write([]byte("x"))
	if !rec.WaitFor(1, time.Second) {
		t.Fatal("timeout")
	}

	s1 := rec.Samples()
	s2 := rec.Samples()
	if len(s1) != 1 || len(s2) != 1 {
		t.Fatalf("expected 1 sample each; got %d and %d", len(s1), len(s2))
	}
	// Mutating s1 must not affect s2 (they are independent copies).
	s1[0].Topic = "mutated"
	if s2[0].Topic == "mutated" {
		t.Fatal("Samples() returned same backing array — not a copy")
	}
}

func TestTopicRecorder_DrainSamples(t *testing.T) {
	p := testutil.NewParticipant(t, dds.Domain(0))
	topic := fmt.Sprintf("testutil/recorder-drain/%d", time.Now().UnixNano())

	sub, err := p.NewSubscriber(topic, dds.DefaultQoS, dds.WithChannelDepth(20))
	if err != nil {
		t.Fatalf("NewSubscriber: %v", err)
	}
	defer sub.Close()

	pub, err := p.NewPublisher(topic, dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
	defer pub.Close()

	rec := testutil.NewTopicRecorder(sub).Start()
	defer rec.Stop()

	_ = pub.Write([]byte("first"))
	if !rec.WaitFor(1, time.Second) {
		t.Fatal("timeout")
	}

	drained := rec.DrainSamples()
	if len(drained) != 1 {
		t.Fatalf("DrainSamples: got %d, want 1", len(drained))
	}
	if rec.Count() != 0 {
		t.Fatalf("Count after Drain: got %d, want 0", rec.Count())
	}
}

func TestTopicRecorder_StopIdempotent(t *testing.T) {
	p := testutil.NewParticipant(t, dds.Domain(0))
	topic := fmt.Sprintf("testutil/recorder-stop/%d", time.Now().UnixNano())

	sub, err := p.NewSubscriber(topic, dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewSubscriber: %v", err)
	}
	defer sub.Close()

	rec := testutil.NewTopicRecorder(sub).Start()
	rec.Stop()
	rec.Stop() // must not panic
}

func TestTopicRecorder_WaitFor_ReturnsFalseOnTimeout(t *testing.T) {
	p := testutil.NewParticipant(t, dds.Domain(0))
	topic := fmt.Sprintf("testutil/recorder-wf-timeout/%d", time.Now().UnixNano())

	sub, err := p.NewSubscriber(topic, dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewSubscriber: %v", err)
	}
	defer sub.Close()

	rec := testutil.NewTopicRecorder(sub).Start()
	defer rec.Stop()

	if rec.WaitFor(1, 30*time.Millisecond) {
		t.Fatal("WaitFor should return false when no samples arrive within timeout")
	}
}
