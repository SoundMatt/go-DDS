// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Reliable QoS: HEARTBEAT / ACKNACK retransmission (RTPS 2.3 §8.4.9 – §8.4.12).
//
// Wire contract:
//   - A reliable writer sends a HEARTBEAT after every Write and periodically
//     (every heartbeatPeriod) advertising its send-history window.
//   - A reliable reader tracks received sequence numbers; when a gap is
//     detected it sends an ACKNACK requesting the missing range.
//   - On receipt of ACKNACK the writer retransmits each requested sample
//     from its history.

package rtps

import (
	"sync"
	"sync/atomic"
	"time"
)

const (
	heartbeatPeriod = 200 * time.Millisecond
	maxHistoryDepth = 256 // samples retained per writer for retransmission
)

// ── Sender-side reliability ───────────────────────────────────────────────────

// sendHistory keeps the last maxHistoryDepth RTPS wire messages keyed by
// sequence-number Low (we only use the Low 32 bits; sufficient for our
// history depth).
type sendHistory struct {
	mu      sync.Mutex
	msgs    map[uint32][]byte // seqLo → full RTPS message bytes
	hbCount atomic.Int32
}

func newSendHistory() *sendHistory {
	return &sendHistory{msgs: make(map[uint32][]byte)}
}

// store saves a copy of the full RTPS message for possible retransmission.
func (h *sendHistory) store(seqLo uint32, msg []byte) {
	cp := make([]byte, len(msg))
	copy(cp, msg)
	h.mu.Lock()
	h.msgs[seqLo] = cp
	// Evict oldest entries beyond depth.
	if len(h.msgs) > maxHistoryDepth {
		oldest := seqLo - uint32(maxHistoryDepth)
		delete(h.msgs, oldest)
	}
	h.mu.Unlock()
}

// get returns the stored message for seqLo, or nil if evicted.
func (h *sendHistory) get(seqLo uint32) []byte {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.msgs[seqLo]
}

// firstLast returns the lowest and highest stored sequence number lows.
func (h *sendHistory) firstLast() (first, last uint32, ok bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.msgs) == 0 {
		return 0, 0, false
	}
	first = ^uint32(0)
	for k := range h.msgs {
		if k < first {
			first = k
		}
		if k > last {
			last = k
		}
	}
	return first, last, true
}

// ── Receiver-side reliability ─────────────────────────────────────────────────

// recvTracker tracks the highest contiguous sequence number received from a
// single remote writer and produces ACKNACK bitmaps for any gaps.
type recvTracker struct {
	mu       sync.Mutex
	expected uint32 // next expected seqLo
	ackCount atomic.Int32
}

// receive records seqLo and returns (bitmap, needAck):
// - bitmap: bit N set means Base+N is missing (up to 32 bits ahead of Base)
// - needAck: true when the bitmap is non-zero (there are gaps to request)
func (rt *recvTracker) receive(seqLo uint32) (base uint32, bitmap uint32, needAck bool) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	if rt.expected == 0 {
		// First sample — initialise expected to seqLo+1.
		rt.expected = seqLo + 1
		return 0, 0, false
	}
	if seqLo == rt.expected {
		rt.expected++
		return 0, 0, false
	}
	if seqLo > rt.expected {
		// Gap detected. Build bitmap: each set bit requests a missing seqLo.
		base = rt.expected
		end := seqLo
		if end > base+31 {
			end = base + 31
		}
		for sn := base; sn < end; sn++ {
			bitmap |= 1 << (sn - base)
		}
		// Advance expected past what we just received (we may still be missing
		// the range [expected..seqLo-1], but we report them in the bitmap).
		rt.expected = seqLo + 1
		return base, bitmap, true
	}
	// Duplicate or out-of-order (seqLo < expected) — ignore.
	return 0, 0, false
}

// nextAckCount returns a monotonically increasing count for ACKNACK.
func (rt *recvTracker) nextAckCount() int32 {
	return rt.ackCount.Add(1)
}
