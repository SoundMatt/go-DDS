// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package mock_test

import (
	"context"
	"errors"
	"testing"
	"time"

	dds "github.com/SoundMatt/go-DDS"
	"github.com/SoundMatt/go-DDS/mock"
)

// TestWithContext_CancelsParticipant verifies that cancelling the context
// propagated via WithContext closes the participant. Subsequent NewPublisher
// calls must return ErrClosed.
func TestWithContext_CancelsParticipant(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	p, err := mock.New(0, mock.IsolatedBroker(), mock.WithContext(ctx))
	if err != nil {
		t.Fatalf("mock.New: %v", err)
	}

	// Participant is open before cancel.
	ignoredVal, err := p.NewPublisher("ctx/before", dds.DefaultQoS)
	_ = ignoredVal
	if err != nil {
		t.Fatalf("NewPublisher before cancel: %v", err)
	}

	cancel()

	// Poll until the background goroutine calls Close.
	deadline := time.After(2 * time.Second)
	for {
		ignoredRet, err := p.NewPublisher("ctx/after", dds.DefaultQoS)
		_ = ignoredRet
		if errors.Is(err, dds.ErrClosed) {
			return // correct
		}
		select {
		case <-deadline:
			t.Fatal("participant did not close after context cancellation")
		case <-time.After(5 * time.Millisecond):
		}
	}
}

// TestWithContext_AlreadyCancelledContext verifies that a participant created
// with an already-cancelled context closes promptly.
func TestWithContext_AlreadyCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already cancelled

	p, err := mock.New(0, mock.IsolatedBroker(), mock.WithContext(ctx))
	if err != nil {
		t.Fatalf("mock.New: %v", err)
	}

	deadline := time.After(2 * time.Second)
	for {
		ignoredRet, err := p.NewPublisher("ctx/precancelled", dds.DefaultQoS)
		_ = ignoredRet
		if errors.Is(err, dds.ErrClosed) {
			return // correct
		}
		select {
		case <-deadline:
			t.Fatal("participant did not close after pre-cancelled context")
		case <-time.After(5 * time.Millisecond):
		}
	}
}

// TestWithContext_NilContextNoOp verifies that not supplying WithContext leaves
// the participant unaffected (backward-compatibility: no option = no cancel goroutine).
func TestWithContext_NilContextNoOp(t *testing.T) {
	p, err := mock.New(0, mock.IsolatedBroker())
	if err != nil {
		t.Fatalf("mock.New: %v", err)
	}
	defer p.Close()

	pub, err := p.NewPublisher("ctx/nilctx", dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
	defer pub.Close()

	if err := pub.Write([]byte("ok")); err != nil {
		t.Fatalf("Write on no-ctx participant: %v", err)
	}
}
