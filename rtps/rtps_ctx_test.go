// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package rtps_test

//fusa:test REQ-REL-004
//fusa:test REQ-PUB-003
//fusa:test REQ-RT-001

import (
	"context"
	"errors"
	"testing"
	"time"

	dds "github.com/SoundMatt/go-DDS"
	"github.com/SoundMatt/go-DDS/rtps"
)

// TestRTPSWithContext_CancelsParticipant verifies that cancelling the context
// passed via rtps.WithContext closes the participant. Subsequent NewPublisher
// calls must return ErrClosed.
func TestRTPSWithContext_CancelsParticipant(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	p, err := rtps.New(testDomain, rtps.WithContext(ctx))
	if err != nil {
		t.Skipf("rtps.New: %v — UDP multicast unavailable", err)
	}

	// Participant is open before cancel.
	ignoredVal, err := p.NewPublisher("rtps/ctx/before", dds.DefaultQoS)
	_ = ignoredVal
	if err != nil {
		t.Fatalf("NewPublisher before cancel: %v", err)
	}

	cancel()

	// Poll until the background goroutine calls Close.
	deadline := time.After(3 * time.Second)
	for {
		ignoredRet, err := p.NewPublisher("rtps/ctx/after", dds.DefaultQoS)
		_ = ignoredRet
		if errors.Is(err, dds.ErrClosed) {
			return // correct
		}
		select {
		case <-deadline:
			t.Fatal("rtps participant did not close after context cancellation")
		case <-time.After(10 * time.Millisecond):
		}
	}
}

// TestRTPSWithContext_AlreadyCancelledContext verifies that a participant
// created with an already-cancelled context closes promptly.
func TestRTPSWithContext_AlreadyCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	p, err := rtps.New(testDomain, rtps.WithContext(ctx))
	if err != nil {
		t.Skipf("rtps.New: %v — UDP multicast unavailable", err)
	}

	deadline := time.After(3 * time.Second)
	for {
		ignoredRet, err := p.NewPublisher("rtps/ctx/precancelled", dds.DefaultQoS)
		_ = ignoredRet
		if errors.Is(err, dds.ErrClosed) {
			return
		}
		select {
		case <-deadline:
			t.Fatal("rtps participant did not close after pre-cancelled context")
		case <-time.After(10 * time.Millisecond):
		}
	}
}
