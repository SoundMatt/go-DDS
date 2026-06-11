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
