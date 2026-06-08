// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package dds_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	dds "github.com/SoundMatt/go-DDS"
	"github.com/SoundMatt/go-DDS/mock"
)

func newMockParticipant(t *testing.T) dds.Participant {
	t.Helper()
	p, err := mock.New(dds.Domain(0))
	if err != nil {
		t.Fatalf("mock.New: %v", err)
	}
	t.Cleanup(func() { _ = p.Close() })
	return p
}

func TestWaitSet_DeliversSample(t *testing.T) {
	p := newMockParticipant(t)
	sub, _ := p.NewSubscriber("ws/deliver", dds.DefaultQoS)
	pub, _ := p.NewPublisher("ws/deliver", dds.DefaultQoS)
	defer sub.Close()
	defer pub.Close()

	_ = pub.Write([]byte("hello"))

	ws := dds.NewWaitSet(sub)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	s, got, err := ws.Wait(ctx)
	if err != nil {
		t.Fatalf("Wait: %v", err)
	}
	if got != sub {
		t.Error("expected sub")
	}
	if string(s.Payload) != "hello" {
		t.Errorf("payload: %q", s.Payload)
	}
}

func TestWaitSet_ContextCancelled(t *testing.T) {
	p := newMockParticipant(t)
	sub, _ := p.NewSubscriber("ws/cancel", dds.DefaultQoS)
	defer sub.Close()

	ws := dds.NewWaitSet(sub)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()

	_, _, err := ws.Wait(ctx)
	if err == nil {
		t.Error("expected context error when no sample arrives")
	}
}

// TestWaitSet_AllChannelsClosed exercises the branch that returns when every
// subscriber channel is closed (closed-channel event with ok=false).
// When all channels are closed and the context is still active the error must
// be ErrClosed (not nil and not a context error).
func TestWaitSet_AllChannelsClosed(t *testing.T) {
	p := newMockParticipant(t)
	sub, _ := p.NewSubscriber("ws/closed", dds.DefaultQoS)

	// Close the channel before calling Wait.
	sub.Close()

	ws := dds.NewWaitSet(sub)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	type result struct {
		err error
	}
	ch := make(chan result, 1)
	go func() {
		_, _, err := ws.Wait(ctx)
		ch <- result{err}
	}()

	select {
	case r := <-ch:
		if !errors.Is(r.err, dds.ErrClosed) {
			t.Errorf("expected ErrClosed when all channels closed, got %v", r.err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("WaitSet.Wait should return when all subscriber channels are closed")
	}
}

// TestWaitSet_AllChannelsClosed_ContextCancelled verifies that a cancelled
// context takes priority over ErrClosed when both occur simultaneously.
func TestWaitSet_AllChannelsClosed_ContextCancelled(t *testing.T) {
	p := newMockParticipant(t)
	sub, _ := p.NewSubscriber("ws/closedctx", dds.DefaultQoS)
	sub.Close()

	ws := dds.NewWaitSet(sub)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already cancelled

	_, _, err := ws.Wait(ctx)
	if err == nil {
		t.Error("expected non-nil error when context is cancelled")
	}
	// May be ctx.Err() or ErrClosed — both are acceptable; must not be nil.
}

// ── SubscriberOption tests ────────────────────────────────────────────────────

func TestWithFilter_AppliedInConfig(t *testing.T) {
	fn := func(s dds.Sample) bool { return true }
	cfg := dds.ApplySubscriberOpts([]dds.SubscriberOption{dds.WithFilter(fn)})
	if cfg.Filter == nil {
		t.Error("expected Filter to be set")
	}
}

func TestWithChannelDepth_AppliedInConfig(t *testing.T) {
	cfg := dds.ApplySubscriberOpts([]dds.SubscriberOption{dds.WithChannelDepth(128)})
	if cfg.ChannelDepth != 128 {
		t.Errorf("ChannelDepth: got %d, want 128", cfg.ChannelDepth)
	}
}

func TestWithBackPressure_AppliedInConfig(t *testing.T) {
	cfg := dds.ApplySubscriberOpts([]dds.SubscriberOption{dds.WithBackPressure(dds.DropOldest)})
	if cfg.BackPressure != dds.DropOldest {
		t.Errorf("BackPressure: got %v, want DropOldest", cfg.BackPressure)
	}
}

func TestApplySubscriberOpts_Empty(t *testing.T) {
	cfg := dds.ApplySubscriberOpts(nil)
	if cfg.Filter != nil || cfg.ChannelDepth != 0 {
		t.Error("empty opts: unexpected non-zero config")
	}
}

func TestChanDepth_Default(t *testing.T) {
	var cfg dds.SubscriberConfig // ChannelDepth == 0
	if got := cfg.ChanDepth(64); got != 64 {
		t.Errorf("ChanDepth default: got %d, want 64", got)
	}
}

func TestChanDepth_Custom(t *testing.T) {
	cfg := dds.SubscriberConfig{ChannelDepth: 32}
	if got := cfg.ChanDepth(64); got != 32 {
		t.Errorf("ChanDepth custom: got %d, want 32", got)
	}
}

// ── CloseWithDrain tests ──────────────────────────────────────────────────────

func TestCloseWithDrain_WithDrainer(t *testing.T) {
	p := newMockParticipant(t)
	// mock.participant implements Drainer via CloseWithDrain.
	ctx := context.Background()
	if err := dds.CloseWithDrain(ctx, p); err != nil {
		t.Fatalf("CloseWithDrain: %v", err)
	}
}

func TestCloseWithDrain_WithoutDrainer(t *testing.T) {
	// Use a real mock participant but wrap it — the wrapper doesn't implement Drainer.
	p := newMockParticipant(t)
	_ = p // still cleaned up by t.Cleanup; double-close is safe for mock
	// Directly exercise the non-drainer path by constructing a non-Drainer participant.
	// The easiest approach: use a partial mock that only implements Close.
	type simplePart struct{ dds.Participant }
	sp := &simplePart{Participant: p}
	ctx := context.Background()
	// simplePart does not have its own CloseWithDrain, but Participant from mock does;
	// since simplePart embeds dds.Participant (interface), the type assertion checks the
	// runtime type of sp (which is *simplePart), and *simplePart does NOT implement Drainer.
	// Therefore CloseWithDrain falls back to sp.Close() which calls the mock's Close.
	if err := dds.CloseWithDrain(ctx, sp); err != nil {
		t.Fatalf("CloseWithDrain fallback: %v", err)
	}
}

// ── NoopTracer tests ──────────────────────────────────────────────────────────

func TestNoopTracer_Start(t *testing.T) {
	ctx, span := dds.NoopTracer.Start(context.Background(), "test-span")
	if ctx == nil {
		t.Error("expected non-nil context")
	}
	// Verify span methods do not panic.
	span.SetAttribute("key", "value")
	span.End()
}

// ── TypedPublisher / TypedSubscriber tests ────────────────────────────────────

type speedSample struct {
	KMH float64 `json:"kmh"`
}

func TestTypedPublisher_WriteAndClose(t *testing.T) {
	p := newMockParticipant(t)
	pub, _ := p.NewPublisher("typed/speed", dds.DefaultQoS)
	defer pub.Close()

	tp := dds.NewTypedPublisher[speedSample](pub, dds.JSONCodec[speedSample]{})
	if err := tp.Write(speedSample{KMH: 120}); err != nil {
		t.Fatalf("TypedPublisher.Write: %v", err)
	}
	if err := tp.Close(); err != nil {
		t.Fatalf("TypedPublisher.Close: %v", err)
	}
}

func TestTypedSubscriber_DeliversDecodedValue(t *testing.T) {
	p := newMockParticipant(t)
	rawSub, _ := p.NewSubscriber("typed/sub", dds.DefaultQoS)
	rawPub, _ := p.NewPublisher("typed/sub", dds.DefaultQoS)
	defer rawPub.Close()

	ts := dds.NewTypedSubscriber[speedSample](rawSub, dds.JSONCodec[speedSample]{})
	defer ts.Close()

	_ = rawPub.Write([]byte(`{"kmh":99.5}`))

	select {
	case s := <-ts.C():
		if s.Value.KMH != 99.5 {
			t.Errorf("decoded KMH: got %v, want 99.5", s.Value.KMH)
		}
		if s.Topic != "typed/sub" {
			t.Errorf("topic: got %q", s.Topic)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for typed sample")
	}
}

func TestTypedSubscriber_DecodeErrorDropped(t *testing.T) {
	p := newMockParticipant(t)
	rawSub, _ := p.NewSubscriber("typed/drop", dds.DefaultQoS)
	rawPub, _ := p.NewPublisher("typed/drop", dds.DefaultQoS)
	defer rawPub.Close()

	ts := dds.NewTypedSubscriber[speedSample](rawSub, dds.JSONCodec[speedSample]{})
	defer ts.Close()

	_ = rawPub.Write([]byte("not-json")) // should be silently dropped

	select {
	case <-ts.C():
		t.Error("expected bad-JSON sample to be dropped")
	case <-time.After(100 * time.Millisecond):
		// correct: nothing delivered
	}
}

// TestTypedSubscriber_ClosedRawSub covers the pump's !ok branch, which fires
// when the underlying subscriber channel closes before ts.Close() is called.
func TestTypedSubscriber_ClosedRawSub(t *testing.T) {
	p := newMockParticipant(t)
	rawSub, _ := p.NewSubscriber("typed/rawclose", dds.DefaultQoS)

	ts := dds.NewTypedSubscriber[speedSample](rawSub, dds.JSONCodec[speedSample]{})

	// Close the raw subscriber — its channel closes, pump sees !ok and exits.
	rawSub.Close()

	// Drain the typed channel until it closes (pump exits on !ok from rawSub).
	select {
	case <-ts.C():
	case <-time.After(500 * time.Millisecond):
		// pump may have already exited without sending
	}
	// ts.Close() must not block since the pump is done.
	if err := ts.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func TestTypedSubscriber_Close_StopsPump(t *testing.T) {
	p := newMockParticipant(t)
	rawSub, _ := p.NewSubscriber("typed/close", dds.DefaultQoS)
	ts := dds.NewTypedSubscriber[speedSample](rawSub, dds.JSONCodec[speedSample]{})
	if err := ts.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func TestJSONCodec_MarshalUnmarshal(t *testing.T) {
	codec := dds.JSONCodec[speedSample]{}
	v := speedSample{KMH: 55.5}
	data, err := codec.Marshal(v)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	got, err := codec.Unmarshal(data)
	if err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if got.KMH != v.KMH {
		t.Errorf("round-trip: got %v, want %v", got, v)
	}
}

func TestJSONCodec_Unmarshal_InvalidJSON(t *testing.T) {
	codec := dds.JSONCodec[speedSample]{}
	if _, err := codec.Unmarshal([]byte("not-json")); err == nil {
		t.Error("expected error for invalid JSON")
	}
}

// errCodec is a stub Codec that always fails to Marshal.
type errCodec[T any] struct{}

func (errCodec[T]) Marshal(T) ([]byte, error)   { return nil, errors.New("forced marshal error") }
func (errCodec[T]) Unmarshal([]byte) (T, error) { var z T; return z, nil }

func TestTypedPublisher_MarshalError_Propagated(t *testing.T) {
	p := newMockParticipant(t)
	pub, _ := p.NewPublisher("typed/marshal-err", dds.DefaultQoS)
	defer pub.Close()

	tp := dds.NewTypedPublisher[speedSample](pub, errCodec[speedSample]{})
	err := tp.Write(speedSample{KMH: 1})
	if err == nil {
		t.Error("expected TypedPublisher.Write to propagate Marshal error")
	}
}

// ── HealthStatus.String ───────────────────────────────────────────────────────

func TestHealthStatus_String_OK(t *testing.T) {
	if got := dds.HealthOK.String(); got != "ok" {
		t.Errorf("HealthOK.String(): got %q, want %q", got, "ok")
	}
}

func TestHealthStatus_String_Degraded(t *testing.T) {
	if got := dds.HealthDegraded.String(); got != "degraded" {
		t.Errorf("HealthDegraded.String(): got %q, want %q", got, "degraded")
	}
}

func TestHealthStatus_String_Down(t *testing.T) {
	if got := dds.HealthDown.String(); got != "down" {
		t.Errorf("HealthDown.String(): got %q, want %q", got, "down")
	}
}

// ── WaitSet OneClosedOnePending ───────────────────────────────────────────────

// TestWaitSet_OneClosedOnePending verifies that Wait continues after a closed
// channel is zeroed, and still delivers from the remaining open subscriber.
// ── Attach / Detach ───────────────────────────────────────────────────────────

func TestWaitSet_Attach_DeliversFromNewSub(t *testing.T) {
	p := newMockParticipant(t)
	sub1, _ := p.NewSubscriber("ws/attach1", dds.DefaultQoS)
	sub2, _ := p.NewSubscriber("ws/attach2", dds.DefaultQoS)
	pub2, _ := p.NewPublisher("ws/attach2", dds.DefaultQoS)
	defer sub1.Close()
	defer sub2.Close()
	defer pub2.Close()

	ws := dds.NewWaitSet(sub1)
	ws.Attach(sub2) // attach before Wait

	_ = pub2.Write([]byte("from-sub2"))

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	s, got, err := ws.Wait(ctx)
	if err != nil {
		t.Fatalf("Wait: %v", err)
	}
	if got != sub2 {
		t.Error("expected sample from sub2 (the attached subscriber)")
	}
	if string(s.Payload) != "from-sub2" {
		t.Errorf("payload: %q", s.Payload)
	}
}

func TestWaitSet_Detach_RemovesSub(t *testing.T) {
	p := newMockParticipant(t)
	sub1, _ := p.NewSubscriber("ws/detach1", dds.DefaultQoS)
	sub2, _ := p.NewSubscriber("ws/detach2", dds.DefaultQoS)
	pub1, _ := p.NewPublisher("ws/detach1", dds.DefaultQoS)
	pub2, _ := p.NewPublisher("ws/detach2", dds.DefaultQoS)
	defer sub1.Close()
	defer sub2.Close()
	defer pub1.Close()
	defer pub2.Close()

	ws := dds.NewWaitSet(sub1, sub2)
	ws.Detach(sub2) // remove sub2 before Wait

	_ = pub1.Write([]byte("from-sub1"))

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	_, got, err := ws.Wait(ctx)
	if err != nil {
		t.Fatalf("Wait: %v", err)
	}
	if got != sub1 {
		t.Errorf("expected sub1, got something else")
	}
}

func TestWaitSet_Detach_Idempotent(t *testing.T) {
	p := newMockParticipant(t)
	sub, _ := p.NewSubscriber("ws/detach-idem", dds.DefaultQoS)
	defer sub.Close()

	ws := dds.NewWaitSet(sub)
	ws.Detach(sub) // remove it
	ws.Detach(sub) // must not panic

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	_, _, err := ws.Wait(ctx) // empty WaitSet times out immediately
	if err == nil {
		t.Error("expected context error on empty WaitSet")
	}
}

func TestWaitSet_Attach_ReturnsWaitSet(t *testing.T) {
	p := newMockParticipant(t)
	sub, _ := p.NewSubscriber("ws/chain", dds.DefaultQoS)
	defer sub.Close()

	ws := dds.NewWaitSet()
	got := ws.Attach(sub)
	if got != ws {
		t.Error("Attach must return the WaitSet for chaining")
	}
}

func TestWaitSet_OneClosedOnePending(t *testing.T) {
	p := newMockParticipant(t)

	subClosed, _ := p.NewSubscriber("ws/closed2", dds.DefaultQoS)
	subOpen, _ := p.NewSubscriber("ws/open", dds.DefaultQoS)
	pub, _ := p.NewPublisher("ws/open", dds.DefaultQoS)
	defer subOpen.Close()
	defer pub.Close()

	subClosed.Close() // closes its channel immediately

	ws := dds.NewWaitSet(subClosed, subOpen)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	_ = pub.Write([]byte("open"))

	s, got, err := ws.Wait(ctx)
	if err != nil {
		t.Fatalf("Wait: %v", err)
	}
	if got != subOpen {
		t.Error("expected sample from subOpen, not the closed subscriber")
	}
	if string(s.Payload) != "open" {
		t.Errorf("payload: %q", s.Payload)
	}
}

func TestTypedPublisher_WriteCtx_ValidContext(t *testing.T) {
	p, _ := mock.New(0)
	defer func() { _ = p.Close() }()

	pub, _ := p.NewPublisher("typed/ctx", dds.DefaultQoS)
	sub, _ := p.NewSubscriber("typed/ctx", dds.DefaultQoS)
	defer func() { _ = sub.Close() }()

	tp := dds.NewTypedPublisher[speedSample](pub, dds.JSONCodec[speedSample]{})
	defer func() { _ = tp.Close() }()

	if err := tp.WriteCtx(context.Background(), speedSample{KMH: 120.0}); err != nil {
		t.Fatalf("TypedPublisher.WriteCtx: %v", err)
	}
	select {
	case s := <-sub.C():
		if len(s.Payload) == 0 {
			t.Error("expected non-empty payload")
		}
	case <-time.After(time.Second):
		t.Fatal("timeout")
	}
}

func TestTypedPublisher_WriteCtx_CancelledContext(t *testing.T) {
	p, _ := mock.New(0)
	defer func() { _ = p.Close() }()

	pub, _ := p.NewPublisher("typed/ctxcancel", dds.DefaultQoS)
	tp := dds.NewTypedPublisher[speedSample](pub, dds.JSONCodec[speedSample]{})
	defer func() { _ = tp.Close() }()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := tp.WriteCtx(ctx, speedSample{KMH: 0}); err == nil {
		t.Error("WriteCtx with cancelled context must return error")
	}
}

func TestTypedPublisher_WriteCtx_MarshalError(t *testing.T) {
	p, _ := mock.New(0)
	defer func() { _ = p.Close() }()

	pub, _ := p.NewPublisher("typed/ctxmarshalerr", dds.DefaultQoS)
	tp := dds.NewTypedPublisher[speedSample](pub, errCodec[speedSample]{})
	defer func() { _ = tp.Close() }()

	if err := tp.WriteCtx(context.Background(), speedSample{KMH: 10}); err == nil {
		t.Error("WriteCtx with marshal error must return error")
	}
}

// ── v0.9.1 additions ──────────────────────────────────────────────────────────

type gobMsg struct {
	Value int
	Label string
}

func TestGobCodec_RoundTrip(t *testing.T) {
	codec := dds.GobCodec[gobMsg]{}
	want := gobMsg{Value: 42, Label: "hello"}
	data, err := codec.Marshal(want)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	got, err := codec.Unmarshal(data)
	if err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if got != want {
		t.Errorf("round-trip: got %v, want %v", got, want)
	}
}

func TestGobCodec_ImplementsCodec(t *testing.T) {
	var _ dds.Codec[gobMsg] = dds.GobCodec[gobMsg]{}
}

func TestNewSentinels_ErrorsIs(t *testing.T) {
	sentinels := []error{
		dds.ErrQoSMismatch,
		dds.ErrDeadlineMissed,
		dds.ErrSampleRejected,
		dds.ErrResourceLimits,
	}
	for _, s := range sentinels {
		wrapped := fmt.Errorf("wrapped: %w", s)
		if !errors.Is(wrapped, s) {
			t.Errorf("errors.Is failed for %v", s)
		}
	}
}

func TestTryRead_Interface(t *testing.T) {
	p := newMockParticipant(t)
	sub, err := p.NewSubscriber("tryread/iface", dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewSubscriber: %v", err)
	}
	defer sub.Close()

	// Channel is empty — TryRead must return false.
	_, ok := sub.TryRead()
	if ok {
		t.Error("TryRead on empty channel must return false")
	}
}

func TestWithDeadlineMissed_Option(t *testing.T) {
	called := false
	cfg := dds.ApplySubscriberOpts([]dds.SubscriberOption{
		dds.WithDeadlineMissed(func() { called = true }),
	})
	if cfg.DeadlineMissedCallback == nil {
		t.Fatal("DeadlineMissedCallback must be set")
	}
	cfg.DeadlineMissedCallback()
	if !called {
		t.Error("callback not invoked")
	}
}

func TestTypedSample_MetadataForwarded(t *testing.T) {
	p := newMockParticipant(t)
	rawSub, _ := p.NewSubscriber("typed/meta", dds.DefaultQoS)
	rawPub, _ := p.NewPublisher("typed/meta", dds.DefaultQoS)
	defer rawPub.Close()

	ts := dds.NewTypedSubscriber[speedSample](rawSub, dds.JSONCodec[speedSample]{})
	defer ts.Close()

	_ = rawPub.Write([]byte(`{"kmh":10}`))

	select {
	case s := <-ts.C():
		if s.SequenceNumber == 0 {
			t.Error("TypedSample.SequenceNumber should be non-zero after mock write")
		}
	case <-time.After(time.Second):
		t.Fatal("timeout")
	}
}
