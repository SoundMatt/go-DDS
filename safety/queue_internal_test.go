// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Internal tests for DeterministicQueue drain paths that require access to
// unexported fields.

package safety

//fusa:test REQ-SAFETY-002

import (
	"context"
	"testing"
	"time"
)

// stubPub is a minimal dds.Publisher stub for internal queue tests.
type stubPub struct{}

func (stubPub) Write(_ []byte) error                      { return nil }
func (stubPub) WriteCtx(_ context.Context, _ []byte) error { return nil }
func (stubPub) Close() error                               { return nil }

// TestDrain_ChClosed covers the `case payload, ok := <-q.ch: if !ok { return }`
// branch where ok is false. This fires when q.ch is closed externally.
// Normally q.ch is never closed (q.done is closed by Stop), but the drain
// goroutine handles both shutdown paths defensively.
func TestDrain_ChClosed(t *testing.T) {
	q := &DeterministicQueue{
		pub:    stubPub{},
		ch:     make(chan []byte, 4),
		done:   make(chan struct{}),
		errors: make(chan error, 4),
	}
	q.wg.Add(1)
	go q.drain()

	// Close q.ch directly — the drain goroutine exits via the !ok branch.
	close(q.ch)
	time.Sleep(20 * time.Millisecond)

	// Close q.done so wg.Wait() returns (drain already exited, but being safe).
	q.once.Do(func() { close(q.done) })
}
