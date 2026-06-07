// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package dds_test

import (
	"context"
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
func TestWaitSet_AllChannelsClosed(t *testing.T) {
	p := newMockParticipant(t)
	sub, _ := p.NewSubscriber("ws/closed", dds.DefaultQoS)

	// Close the channel before calling Wait.
	sub.Close()

	ws := dds.NewWaitSet(sub)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _, _ = ws.Wait(ctx) // should return promptly, not block until deadline
	}()

	select {
	case <-done:
		// correct
	case <-time.After(500 * time.Millisecond):
		t.Fatal("WaitSet.Wait should return when all subscriber channels are closed")
	}
}

// TestWaitSet_OneClosedOnePending verifies that Wait continues after a closed
// channel is zeroed, and still delivers from the remaining open subscriber.
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
