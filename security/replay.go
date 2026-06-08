// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package security

//fusa:req REQ-SEC-003
//fusa:req REQ-SEC-011
//fusa:req REQ-SEC-012
//fusa:req REQ-SEC-023

import (
	"errors"
	"sync"
	"time"
)

// ErrReplay is returned by ReplayGuard.Check when a sequence number has
// already been accepted within the replay window.
var ErrReplay = errors.New("security: replayed sequence number detected")

// ReplayGuard protects against replay attacks by tracking recently-seen
// sequence numbers within a sliding time window.
//
// Each sequence number is associated with the timestamp of the message that
// carried it. A sequence number is considered a replay if it has been seen
// within window duration of the current call.
//
// ReplayGuard is safe for concurrent use from multiple goroutines.
type ReplayGuard struct {
	mu     sync.Mutex
	window time.Duration
	seen   map[uint64]time.Time
}

// NewReplayGuard creates a ReplayGuard with the given sliding window.
// A window of 0 or negative is replaced with 30 seconds.
func NewReplayGuard(window time.Duration) *ReplayGuard {
	if window <= 0 {
		window = 30 * time.Second
	}
	return &ReplayGuard{window: window, seen: make(map[uint64]time.Time)}
}

// Check reports whether seq is a replay. If seq has not been seen within
// window of ts, it is recorded and nil is returned. If seq has already been
// seen within the window, ErrReplay is returned.
//
// ts is the claimed send timestamp of the message. Entries whose recorded
// timestamp is more than window before ts are pruned on each Check call.
func (g *ReplayGuard) Check(seq uint64, ts time.Time) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.purge(ts)
	if _, ok := g.seen[seq]; ok {
		return ErrReplay
	}
	g.seen[seq] = ts
	return nil
}

// Purge removes all entries older than window before time.Now().
// Check calls Purge automatically; call this method explicitly only when
// driving the clock externally (e.g. in tests).
func (g *ReplayGuard) Purge() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.purge(time.Now())
}

// Len returns the number of sequence numbers currently tracked.
func (g *ReplayGuard) Len() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return len(g.seen)
}

func (g *ReplayGuard) purge(now time.Time) {
	cutoff := now.Add(-g.window)
	for seq, t := range g.seen {
		if t.Before(cutoff) {
			delete(g.seen, seq)
		}
	}
}
