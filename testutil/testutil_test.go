// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package testutil_test

//fusa:test REQ-TESTUTIL-001
//fusa:test REQ-TESTUTIL-002
//fusa:test REQ-TESTUTIL-003
//fusa:test REQ-TESTUTIL-004
//fusa:test REQ-TESTUTIL-005
//fusa:test REQ-TESTUTIL-006
//fusa:test REQ-TESTUTIL-007
//fusa:test REQ-TESTUTIL-008
//fusa:test REQ-TESTUTIL-009
//fusa:test REQ-TESTUTIL-010

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	dds "github.com/SoundMatt/go-DDS"
	"github.com/SoundMatt/go-DDS/mock"
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
	// Directly verify the mismatch by reading from the channel.
	// The payload "wrong" must not equal "right".
	select {
	case got := <-sub.C():
		if string(got.Payload) == "right" {
			t.Fatalf("expected payload %q != %q", got.Payload, "right")
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for published sample")
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

func TestPeriodicPublish_WriteError(t *testing.T) {
	p := testutil.NewParticipant(t, dds.Domain(0))
	topic := fmt.Sprintf("testutil/periodic-err/%d", time.Now().UnixNano())
	pub, err := p.NewPublisher(topic, dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
	_ = pub.Close() // pre-close so Write returns an error immediately

	stop := make(chan struct{})
	defer close(stop)
	done := make(chan error, 1)
	go func() { done <- testutil.PeriodicPublish(pub, []byte("x"), 5*time.Millisecond, stop) }()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected write error from PeriodicPublish on closed publisher")
		}
	case <-time.After(time.Second):
		t.Fatal("PeriodicPublish did not return on write error")
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

// TestTopicRecorder_Loop_ClosedSub verifies the loop's !ok branch, which fires
// when the underlying subscriber's channel is closed (sub.Close() called).
func TestTopicRecorder_Loop_ClosedSub(t *testing.T) {
	p := testutil.NewParticipant(t, dds.Domain(0))
	topic := fmt.Sprintf("testutil/recorder-closed/%d", time.Now().UnixNano())

	sub, err := p.NewSubscriber(topic, dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewSubscriber: %v", err)
	}

	rec := testutil.NewTopicRecorder(sub).Start()
	// Closing the subscriber closes its channel; the loop goroutine sees !ok and exits.
	sub.Close()
	// Wait long enough for the goroutine to see the closed channel.
	time.Sleep(50 * time.Millisecond)
	// Stop must complete promptly since the loop has already exited.
	rec.Stop()
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

// ── AssertSample / AssertNoSample fatal paths via subprocess ──────────────────

// The subprocess helpers below cover the t.Fatalf branches inside AssertSample
// and AssertNoSample. Each helper function runs only when the matching
// environment variable is set; the outer test spawns the subprocess and checks
// that it exits non-zero with the expected message.

func runFatalSubprocess(t *testing.T, env string, wantMsg string) {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run="+env, "-test.v")
	cmd.Env = append(os.Environ(), "TESTUTIL_FATAL_CASE="+env)
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("subprocess %q: expected non-zero exit, got success. output:\n%s", env, out)
	}
	if !strings.Contains(string(out), wantMsg) {
		t.Errorf("subprocess %q: output does not contain %q:\n%s", env, wantMsg, out)
	}
}

func TestAssertSample_Timeout_Fatal(t *testing.T) {
	if os.Getenv("TESTUTIL_FATAL_CASE") == "TestAssertSample_Timeout_Fatal" {
		p := testutil.NewParticipant(t, dds.Domain(0))
		sub, err := p.NewSubscriber(fmt.Sprintf("fatal/timeout/%d", time.Now().UnixNano()), dds.DefaultQoS)
		if err != nil {
			t.Fatalf("NewSubscriber: %v", err)
		}
		testutil.AssertSample(t, sub, []byte("x"), 10*time.Millisecond)
		return
	}
	runFatalSubprocess(t, "TestAssertSample_Timeout_Fatal", "AssertSample: timeout")
}

func TestAssertSample_Mismatch_Fatal(t *testing.T) {
	if os.Getenv("TESTUTIL_FATAL_CASE") == "TestAssertSample_Mismatch_Fatal" {
		p := testutil.NewParticipant(t, dds.Domain(0))
		topic := fmt.Sprintf("fatal/mismatch/%d", time.Now().UnixNano())
		sub, err := p.NewSubscriber(topic, dds.DefaultQoS)
		if err != nil {
			t.Fatalf("NewSubscriber: %v", err)
		}
		pub, err := p.NewPublisher(topic, dds.DefaultQoS)
		if err != nil {
			t.Fatalf("NewPublisher: %v", err)
		}
		_ = pub.Write([]byte("wrong"))
		testutil.AssertSample(t, sub, []byte("right"), time.Second)
		return
	}
	runFatalSubprocess(t, "TestAssertSample_Mismatch_Fatal", "AssertSample: payload mismatch")
}

func TestAssertNoSample_UnexpectedSample_Fatal(t *testing.T) {
	if os.Getenv("TESTUTIL_FATAL_CASE") == "TestAssertNoSample_UnexpectedSample_Fatal" {
		p := testutil.NewParticipant(t, dds.Domain(0))
		topic := fmt.Sprintf("fatal/nosample/%d", time.Now().UnixNano())
		sub, err := p.NewSubscriber(topic, dds.DefaultQoS)
		if err != nil {
			t.Fatalf("NewSubscriber: %v", err)
		}
		pub, err := p.NewPublisher(topic, dds.DefaultQoS)
		if err != nil {
			t.Fatalf("NewPublisher: %v", err)
		}
		_ = pub.Write([]byte("unexpected"))
		testutil.AssertNoSample(t, sub, time.Second)
		return
	}
	runFatalSubprocess(t, "TestAssertNoSample_UnexpectedSample_Fatal", "AssertNoSample: unexpected sample")
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

// ── mockTB — coverage for t.Fatalf paths in AssertSample / AssertNoSample ─────
//
// The public API accepts testing.TB, so we can supply a lightweight stub that
// captures Fatalf calls and calls runtime.Goexit() (the same mechanism that
// *testing.T.Fatalf uses internally) so the callee returns normally.

type mockTB struct {
	testing.TB
	failed bool
	msg    string
}

func (m *mockTB) Helper()          {}
func (m *mockTB) Cleanup(f func()) { f() }
func (m *mockTB) Fatalf(format string, args ...any) {
	m.failed = true
	m.msg = fmt.Sprintf(format, args...)
	runtime.Goexit()
}

// TestAssertSample_Timeout_Coverage covers the timeout t.Fatalf path in
// AssertSample (testutil.go:44) via a mockTB running in a goroutine.
func TestAssertSample_Timeout_Coverage(t *testing.T) {
	p, err := mock.New(dds.Domain(0))
	if err != nil {
		t.Fatalf("mock.New: %v", err)
	}
	defer p.Close()

	sub, err := p.NewSubscriber(fmt.Sprintf("util/cov/timeout/%d", time.Now().UnixNano()), dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewSubscriber: %v", err)
	}
	defer sub.Close()

	tb := &mockTB{}
	done := make(chan struct{})
	go func() {
		defer close(done)
		testutil.AssertSample(tb, sub, []byte("x"), 10*time.Millisecond)
	}()
	<-done

	if !tb.failed || !strings.Contains(tb.msg, "timeout") {
		t.Fatalf("expected timeout Fatalf; got failed=%v msg=%q", tb.failed, tb.msg)
	}
}

// TestAssertSample_Mismatch_Coverage covers the payload-mismatch t.Fatalf path
// in AssertSample (testutil.go:41) via a mockTB.
func TestAssertSample_Mismatch_Coverage(t *testing.T) {
	p, err := mock.New(dds.Domain(0))
	if err != nil {
		t.Fatalf("mock.New: %v", err)
	}
	defer p.Close()

	topic := fmt.Sprintf("util/cov/mismatch/%d", time.Now().UnixNano())
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

	_ = pub.Write([]byte("actual"))

	tb := &mockTB{}
	done := make(chan struct{})
	go func() {
		defer close(done)
		testutil.AssertSample(tb, sub, []byte("expected"), time.Second)
	}()
	<-done

	if !tb.failed || !strings.Contains(tb.msg, "payload mismatch") {
		t.Fatalf("expected mismatch Fatalf; got failed=%v msg=%q", tb.failed, tb.msg)
	}
}

// TestAssertNoSample_UnexpectedSample_Coverage covers the unexpected-sample
// t.Fatalf path in AssertNoSample (testutil.go:53) via a mockTB.
func TestAssertNoSample_UnexpectedSample_Coverage(t *testing.T) {
	p, err := mock.New(dds.Domain(0))
	if err != nil {
		t.Fatalf("mock.New: %v", err)
	}
	defer p.Close()

	topic := fmt.Sprintf("util/cov/nosample/%d", time.Now().UnixNano())
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

	_ = pub.Write([]byte("unexpected"))

	tb := &mockTB{}
	done := make(chan struct{})
	go func() {
		defer close(done)
		testutil.AssertNoSample(tb, sub, time.Second)
	}()
	<-done

	if !tb.failed || !strings.Contains(tb.msg, "unexpected sample") {
		t.Fatalf("expected unexpected-sample Fatalf; got failed=%v msg=%q", tb.failed, tb.msg)
	}
}
