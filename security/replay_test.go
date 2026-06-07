// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package security_test

import (
	"errors"
	"testing"
	"time"

	"github.com/SoundMatt/go-DDS/security"
)

func TestReplayGuard_FirstSeenAllowed(t *testing.T) {
	g := security.NewReplayGuard(30 * time.Second)
	now := time.Now()
	if err := g.Check(1, now); err != nil {
		t.Errorf("first Check should succeed: %v", err)
	}
}

func TestReplayGuard_ReplayDetected(t *testing.T) {
	g := security.NewReplayGuard(30 * time.Second)
	now := time.Now()
	_ = g.Check(42, now)
	if err := g.Check(42, now); !errors.Is(err, security.ErrReplay) {
		t.Errorf("second Check of same seq should return ErrReplay, got %v", err)
	}
}

func TestReplayGuard_DifferentSeqAllowed(t *testing.T) {
	g := security.NewReplayGuard(30 * time.Second)
	now := time.Now()
	_ = g.Check(1, now)
	if err := g.Check(2, now); err != nil {
		t.Errorf("different seq should be allowed: %v", err)
	}
}

func TestReplayGuard_Purge_RemovesExpiredEntries(t *testing.T) {
	g := security.NewReplayGuard(50 * time.Millisecond)
	past := time.Now().Add(-100 * time.Millisecond) // outside window
	_ = g.Check(99, past)

	// After purging against now, the past entry should be removed.
	g.Purge()
	if g.Len() != 0 {
		t.Errorf("expected 0 entries after purge, got %d", g.Len())
	}
}

func TestReplayGuard_ExpiredSeq_AllowedAfterWindow(t *testing.T) {
	g := security.NewReplayGuard(50 * time.Millisecond)
	past := time.Now().Add(-100 * time.Millisecond)
	_ = g.Check(7, past)

	// The entry should have expired; checking seq 7 with a future ts should succeed.
	future := time.Now()
	if err := g.Check(7, future); err != nil {
		t.Errorf("expired seq should be allowed after window: %v", err)
	}
}

func TestReplayGuard_DefaultWindow(t *testing.T) {
	// window <= 0 is replaced with 30s — just verify it constructs without panic.
	g := security.NewReplayGuard(0)
	if err := g.Check(1, time.Now()); err != nil {
		t.Errorf("zero-window guard should still allow first check: %v", err)
	}
}

func TestReplayGuard_Len_TracksCount(t *testing.T) {
	g := security.NewReplayGuard(time.Minute)
	now := time.Now()
	if g.Len() != 0 {
		t.Errorf("initial Len: got %d, want 0", g.Len())
	}
	_ = g.Check(1, now)
	_ = g.Check(2, now)
	if g.Len() != 2 {
		t.Errorf("after 2 checks Len: got %d, want 2", g.Len())
	}
}

func TestReplayGuard_Concurrent(t *testing.T) {
	g := security.NewReplayGuard(time.Second)
	now := time.Now()
	done := make(chan struct{})
	for i := uint64(0); i < 20; i++ {
		seq := i
		go func() {
			_ = g.Check(seq, now)
			done <- struct{}{}
		}()
	}
	for i := 0; i < 20; i++ {
		<-done
	}
}
