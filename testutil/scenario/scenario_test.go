// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package scenario_test

//fusa:test REQ-LOAN-001
//fusa:test REQ-LOAN-002
//fusa:test REQ-LOAN-003

import (
	"context"
	"fmt"
	"testing"
	"time"

	dds "github.com/SoundMatt/go-DDS"
	"github.com/SoundMatt/go-DDS/mock"
	"github.com/SoundMatt/go-DDS/testutil/scenario"
)

func newParticipant(t *testing.T) dds.Participant {
	t.Helper()
	p, err := mock.New(dds.Domain(0))
	if err != nil {
		t.Fatalf("new participant: %v", err)
	}
	t.Cleanup(func() { _ = p.Close() })
	return p
}

func TestScenario_publish_expect(t *testing.T) {
	p := newParticipant(t)
	ctx := context.Background()

	// Use ReliableQoS (TransientLocal) so the mock retains the last sample for
	// the late-joining subscriber created by the Expect step.
	err := scenario.Run(ctx, p,
		scenario.Publish("sensor/temp", []byte("42"), dds.ReliableQoS),
		scenario.Expect("sensor/temp", dds.ReliableQoS, 100*time.Millisecond, func(s dds.Sample) bool {
			return string(s.Payload) == "42"
		}),
	)
	if err != nil {
		t.Fatal(err)
	}
}

func TestScenario_expect_none(t *testing.T) {
	p := newParticipant(t)
	ctx := context.Background()

	err := scenario.Run(ctx, p,
		scenario.ExpectNone("sensor/temp", dds.DefaultQoS, 20*time.Millisecond),
	)
	if err != nil {
		t.Fatal(err)
	}
}

func TestScenario_wait(t *testing.T) {
	p := newParticipant(t)
	ctx := context.Background()

	start := time.Now()
	err := scenario.Run(ctx, p,
		scenario.Wait(20*time.Millisecond),
	)
	if err != nil {
		t.Fatal(err)
	}
	if time.Since(start) < 20*time.Millisecond {
		t.Fatal("Wait step returned too early")
	}
}

func TestScenario_assert(t *testing.T) {
	p := newParticipant(t)
	ctx := context.Background()

	called := false
	err := scenario.Run(ctx, p,
		scenario.Assert("check domain", func(_ context.Context, part dds.Participant) error {
			called = true
			if part.Domain() != dds.Domain(0) {
				t.Errorf("unexpected domain %v", part.Domain())
			}
			return nil
		}),
	)
	if err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Fatal("assert step was not called")
	}
}

func TestScenario_context_cancel(t *testing.T) {
	p := newParticipant(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := scenario.Run(ctx, p,
		scenario.Wait(time.Second),
	)
	if err == nil {
		t.Fatal("expected error on cancelled context")
	}
}

func TestScenario_publish_error_propagates(t *testing.T) {
	p := newParticipant(t)
	ctx := context.Background()

	// Closed participant returns errors from NewPublisher.
	_ = p.Close()

	err := scenario.Run(ctx, p,
		scenario.Publish("x/y", []byte("data"), dds.DefaultQoS),
	)
	if err == nil {
		t.Fatal("expected error when publishing on closed participant")
	}
}

func TestScenario_expect_subscriber_closed(t *testing.T) {
	p := newParticipant(t)
	ctx := context.Background()

	// Close participant before Expect runs to trigger subscriber-closed path.
	_ = p.Close()

	err := scenario.Run(ctx, p,
		scenario.Expect("x/y", dds.DefaultQoS, 50*time.Millisecond, nil),
	)
	if err == nil {
		t.Fatal("expected error when expecting on closed participant")
	}
}

func TestScenario_expect_ctx_cancel(t *testing.T) {
	p := newParticipant(t)
	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()

	err := scenario.Run(ctx, p,
		scenario.Expect("never/fires", dds.DefaultQoS, 10*time.Second, nil),
	)
	if err == nil {
		t.Fatal("expected error on context cancel during Expect")
	}
}

func TestScenario_expect_none_unexpected_sample(t *testing.T) {
	p := newParticipant(t)
	ctx := context.Background()

	// Publish first so the subscriber created by ExpectNone receives it.
	err := scenario.Run(ctx, p,
		scenario.Publish("noisy/topic", []byte("boom"), dds.ReliableQoS),
		scenario.ExpectNone("noisy/topic", dds.ReliableQoS, 100*time.Millisecond),
	)
	if err == nil {
		t.Fatal("expected failure: ExpectNone should fail when a sample arrives")
	}
}

func TestScenario_assert_error(t *testing.T) {
	p := newParticipant(t)
	ctx := context.Background()

	err := scenario.Run(ctx, p,
		scenario.Assert("always fail", func(_ context.Context, _ dds.Participant) error {
			return fmt.Errorf("deliberate failure")
		}),
	)
	if err == nil {
		t.Fatal("expected assert error to propagate")
	}
}

func TestScenario_wait_ctx_cancel(t *testing.T) {
	p := newParticipant(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := scenario.Run(ctx, p,
		scenario.Wait(time.Second),
	)
	if err == nil {
		t.Fatal("expected context cancellation error from Wait")
	}
}

func TestScenario_multi_publish(t *testing.T) {
	p := newParticipant(t)
	ctx := context.Background()

	// TransientLocal retains the last sample so the Expect step (created after
	// the publish steps) still receives it.
	err := scenario.Run(ctx, p,
		scenario.Publish("a/b", []byte("first"), dds.ReliableQoS),
		scenario.Publish("a/b", []byte("second"), dds.ReliableQoS),
		scenario.Expect("a/b", dds.ReliableQoS, 100*time.Millisecond, func(s dds.Sample) bool {
			return string(s.Payload) == "first" || string(s.Payload) == "second"
		}),
	)
	if err != nil {
		t.Fatal(err)
	}
}

// ── helpers for error-path coverage ──────────────────────────────────────────

// closedChannelSub is a Subscriber whose channel is already closed, so the
// first receive from C() immediately returns (zero, false).
type closedChannelSub struct{ ch chan dds.Sample }

func (s *closedChannelSub) C() <-chan dds.Sample        { return s.ch }
func (s *closedChannelSub) TryRead() (dds.Sample, bool) { return dds.Sample{}, false }
func (s *closedChannelSub) Unsubscribe() error          { return nil }
func (s *closedChannelSub) Close() error                { return nil }

// preClosedParticipant wraps a Participant but returns a pre-closed subscriber
// from NewSubscriber, exercising the !ok branch in expectStep.
type preClosedParticipant struct{ dds.Participant }

func (p *preClosedParticipant) NewSubscriber(topic string, qos dds.QoS, opts ...dds.SubscriberOption) (dds.Subscriber, error) {
	ch := make(chan dds.Sample)
	close(ch)
	return &closedChannelSub{ch: ch}, nil
}

// ── error-path tests ──────────────────────────────────────────────────────────

func TestScenario_publish_write_error(t *testing.T) {
	p := newParticipant(t)
	ctx := context.Background()

	// MaxSampleSize:1 forces WriteCtx to fail for any payload larger than 1 byte.
	err := scenario.Run(ctx, p,
		scenario.Publish("x/y", []byte("hello"), dds.QoS{MaxSampleSize: 1}),
	)
	if err == nil {
		t.Fatal("expected write error from publishStep")
	}
}

func TestScenario_expect_timeout(t *testing.T) {
	p := newParticipant(t)
	ctx := context.Background()

	// 1ms timeout with no publisher — deadline fires before any sample arrives.
	err := scenario.Run(ctx, p,
		scenario.Expect("silent/topic", dds.DefaultQoS, 1*time.Millisecond, nil),
	)
	if err == nil {
		t.Fatal("expected timeout error from expectStep")
	}
}

func TestScenario_expect_sub_channel_closed(t *testing.T) {
	// Use a participant that returns a pre-closed subscriber channel, triggering
	// the !ok branch in expectStep.run.
	p := newParticipant(t)
	wrapped := &preClosedParticipant{Participant: p}
	ctx := context.Background()

	err := scenario.Run(ctx, wrapped,
		scenario.Expect("x/y", dds.DefaultQoS, time.Second, nil),
	)
	if err == nil {
		t.Fatal("expected subscriber-closed error from expectStep")
	}
}

func TestScenario_wait_ctx_cancel_during_wait(t *testing.T) {
	p := newParticipant(t)
	ctx, cancel := context.WithCancel(context.Background())

	// Cancel context while Wait is blocking, not before Run is entered.
	go func() {
		time.Sleep(10 * time.Millisecond)
		cancel()
	}()

	err := scenario.Run(ctx, p,
		scenario.Wait(time.Second),
	)
	if err == nil {
		t.Fatal("expected context-cancel error from waitStep")
	}
}

func TestScenario_expect_none_sub_error(t *testing.T) {
	p := newParticipant(t)
	ctx := context.Background()

	// Closed participant causes NewSubscriber to fail inside expectNoneStep.
	_ = p.Close()

	err := scenario.Run(ctx, p,
		scenario.ExpectNone("x/y", dds.DefaultQoS, 50*time.Millisecond),
	)
	if err == nil {
		t.Fatal("expected subscriber-creation error from expectNoneStep")
	}
}

func TestScenario_expect_none_ctx_cancel(t *testing.T) {
	p := newParticipant(t)
	ctx, cancel := context.WithCancel(context.Background())

	// Cancel context while ExpectNone is waiting, not before Run is entered.
	go func() {
		time.Sleep(10 * time.Millisecond)
		cancel()
	}()

	err := scenario.Run(ctx, p,
		scenario.ExpectNone("silent/topic", dds.DefaultQoS, time.Second),
	)
	if err == nil {
		t.Fatal("expected context-cancel error from expectNoneStep")
	}
}
