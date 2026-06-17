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

//fusa:req REQ-REL-001
//fusa:req REQ-REL-005
//fusa:req REQ-REL-006
//fusa:req REQ-REL-007
//fusa:req REQ-REL-008
//fusa:req REQ-REL-009

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

// maxReorderAhead bounds how far ahead of the cumulative-ack watermark a
// received sequence number is buffered. Samples further ahead are not retained
// in the out-of-order set (they are re-requested via HEARTBEAT/ACKNACK once the
// watermark advances), keeping memory bounded against a misbehaving writer.
const maxReorderAhead = 8192

// recvTracker tracks reliable-delivery state for a single remote writer.
//
// It maintains a sliding window: expected is the lowest sequence number not yet
// received (the cumulative-ACK base — everything below it has arrived), and
// ahead holds out-of-order SNs at or above expected that have been received.
// expected advances only over a contiguous run, so a missing SN is re-NACKed on
// every HEARTBEAT until it actually arrives (fixing one-shot NACK), and gaps
// larger than one 32-bit ACKNACK window are recovered window-by-window as the
// watermark advances (fixing the gaps-greater-than-31 data loss).
type recvTracker struct {
	mu       sync.Mutex
	expected uint32          // lowest seqLo not yet received (cumulative-ACK base)
	ahead    map[uint32]bool // received SNs > expected (out-of-order/gap fillers)
	initDone bool
	ackCount atomic.Int32
}

// initExpected sets the cumulative-ACK base on first contact with a writer
// (typically from a HEARTBEAT's FirstSN) so the reader can request the writer's
// whole history. It is a no-op once the tracker has seen any sample.
func (rt *recvTracker) initExpected(firstSN uint32) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	if !rt.initDone {
		rt.expected = firstSN
		rt.initDone = true
	}
}

// record marks seqLo as received and advances the contiguous watermark over any
// buffered successors. It returns fresh=false when seqLo was already delivered
// (below the watermark) or already buffered, so callers can suppress duplicate
// delivery.
func (rt *recvTracker) record(seqLo uint32) (fresh bool) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	if !rt.initDone {
		rt.expected = seqLo
		rt.initDone = true
	}
	if seqLo < rt.expected {
		return false // already delivered
	}
	if rt.ahead == nil {
		rt.ahead = make(map[uint32]bool)
	}
	if seqLo == rt.expected {
		rt.expected++
		for rt.ahead[rt.expected] {
			delete(rt.ahead, rt.expected)
			rt.expected++
		}
		return true
	}
	// seqLo > expected: buffer it (bounded) unless already seen.
	if rt.ahead[seqLo] {
		return false
	}
	if seqLo-rt.expected <= maxReorderAhead {
		rt.ahead[seqLo] = true
	}
	return true
}

// missing returns the ACKNACK base and bitmap describing the sequence numbers
// the reader is still missing in [expected, lastSN], capped at one 32-bit
// window. base is the cumulative-ACK watermark; bit N set means base+N is
// missing. needAck is true when at least one SN in the window is missing.
func (rt *recvTracker) missing(lastSN uint32) (base, bitmap uint32, needAck bool) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	if !rt.initDone {
		return 0, 0, false
	}
	base = rt.expected
	if lastSN < base {
		return base, 0, false // fully caught up with the writer
	}
	end := lastSN
	if end > base+31 {
		end = base + 31
	}
	for sn := base; sn <= end; sn++ {
		// expected is never present in ahead, so this also flags base itself.
		if !rt.ahead[sn] {
			bitmap |= 1 << (sn - base)
			needAck = true
		}
	}
	return base, bitmap, needAck
}

// nextAckCount returns a monotonically increasing count for ACKNACK.
func (rt *recvTracker) nextAckCount() int32 {
	return rt.ackCount.Add(1)
}
