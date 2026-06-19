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
//fusa:req REQ-LLR-001
//fusa:req REQ-LLR-002

import (
	"sync"
	"sync/atomic"
	"time"
)

const (
	heartbeatPeriod = 200 * time.Millisecond
	maxHistoryDepth = 256 // samples retained per writer for retransmission
)

// snToU64 packs an RTPS SequenceNumber (High:Low) into a single uint64 so
// reliability bookkeeping never aliases after the low 32 bits wrap (spec §8.3.5).
func snToU64(sn SequenceNumber) uint64 {
	return uint64(uint32(sn.High))<<32 | uint64(sn.Low)
}

// u64ToSN splits a uint64 sequence number back into the wire High:Low form.
func u64ToSN(v uint64) SequenceNumber {
	return SequenceNumber{High: int32(v >> 32), Low: uint32(v)}
}

// ── Sender-side reliability ───────────────────────────────────────────────────

// sendHistory keeps the last maxHistoryDepth RTPS wire messages for
// retransmission, keyed by full 64-bit sequence number. Because a writer emits
// strictly increasing, contiguous sequence numbers, the store is a fixed-size
// ring indexed by seq%depth: O(1), provably bounded (never larger than depth),
// and free of the 2^32 aliasing the previous low-32-bit map suffered.
type sendHistory struct {
	mu      sync.Mutex
	depth   uint64
	msg     [][]byte // ring of message copies, len depth
	sn      []uint64 // sn[i] is the sequence number msg[i] holds
	filled  []bool   // whether slot i is occupied
	highest uint64   // highest sequence number stored
	lowest  uint64   // lowest sequence number still retained
	any     bool
	hbCount atomic.Int32
}

func newSendHistory() *sendHistory {
	d := uint64(maxHistoryDepth)
	return &sendHistory{
		depth:  d,
		msg:    make([][]byte, d),
		sn:     make([]uint64, d),
		filled: make([]bool, d),
	}
}

// store saves a copy of the full RTPS message for possible retransmission,
// evicting whatever sequence number previously occupied the ring slot.
func (h *sendHistory) store(seq uint64, msg []byte) {
	cp := make([]byte, len(msg))
	copy(cp, msg)
	h.mu.Lock()
	i := seq % h.depth
	h.msg[i] = cp
	h.sn[i] = seq
	h.filled[i] = true
	if !h.any {
		h.lowest, h.highest, h.any = seq, seq, true
	}
	if seq > h.highest {
		h.highest = seq
	}
	// The retained window is the last `depth` sequence numbers.
	if h.highest >= h.depth {
		if lb := h.highest - h.depth + 1; lb > h.lowest {
			h.lowest = lb
		}
	}
	h.mu.Unlock()
}

// get returns the stored message for seq, or nil if it was never stored or has
// been evicted from the retained window.
func (h *sendHistory) get(seq uint64) []byte {
	h.mu.Lock()
	defer h.mu.Unlock()
	if !h.any || seq < h.lowest || seq > h.highest {
		return nil
	}
	i := seq % h.depth
	if !h.filled[i] || h.sn[i] != seq {
		return nil
	}
	return h.msg[i]
}

// firstLast returns the lowest and highest retained sequence numbers.
func (h *sendHistory) firstLast() (first, last uint64, ok bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if !h.any {
		return 0, 0, false
	}
	return h.lowest, h.highest, true
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
	expected uint64          // lowest SN not yet received (cumulative-ACK base)
	ahead    map[uint64]bool // received SNs > expected (out-of-order/gap fillers)
	initDone bool
	ackCount atomic.Int32
}

// initExpected sets the cumulative-ACK base on first contact with a writer
// (typically from a HEARTBEAT's FirstSN) so the reader can request the writer's
// whole history. It is a no-op once the tracker has seen any sample.
func (rt *recvTracker) initExpected(firstSN uint64) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	if !rt.initDone {
		rt.expected = firstSN
		rt.initDone = true
	}
}

// record marks seq as received and advances the contiguous watermark over any
// buffered successors. It returns fresh=false when seq was already delivered
// (below the watermark) or already buffered, so callers can suppress duplicate
// delivery.
func (rt *recvTracker) record(seq uint64) (fresh bool) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	if !rt.initDone {
		rt.expected = seq
		rt.initDone = true
	}
	if seq < rt.expected {
		return false // already delivered
	}
	if rt.ahead == nil {
		rt.ahead = make(map[uint64]bool)
	}
	if seq == rt.expected {
		rt.expected++
		for rt.ahead[rt.expected] {
			delete(rt.ahead, rt.expected)
			rt.expected++
		}
		return true
	}
	// seq > expected: buffer it (bounded) unless already seen.
	if rt.ahead[seq] {
		return false
	}
	if seq-rt.expected <= maxReorderAhead {
		rt.ahead[seq] = true
	}
	return true
}

// missing returns the ACKNACK base and bitmap describing the sequence numbers
// the reader is still missing in [expected, lastSN], capped at one 32-bit
// window. base is the cumulative-ACK watermark; bit N set means base+N is
// missing. needAck is true when at least one SN in the window is missing.
func (rt *recvTracker) missing(lastSN uint64) (base uint64, bitmap uint32, needAck bool) {
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
			bitmap |= 1 << uint(sn-base)
			needAck = true
		}
	}
	return base, bitmap, needAck
}

// nextAckCount returns a monotonically increasing count for ACKNACK.
func (rt *recvTracker) nextAckCount() int32 {
	return rt.ackCount.Add(1)
}
