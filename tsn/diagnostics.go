// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package tsn

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

// ── StreamHealth ──────────────────────────────────────────────────────────────

// StreamHealth is a point-in-time timing health snapshot for a TSN stream.
// Healthy is true when fewer than 5 % of writes in the observation window
// were late (lateness > 20 µs past the nearest scheduled slot).
type StreamHealth struct {
	Topic       string
	Interval    time.Duration
	TxOffset    time.Duration
	WriteCount  uint64
	LateWrites  uint64
	MaxLateness time.Duration
	Healthy     bool
}

// ── HealthTracker ─────────────────────────────────────────────────────────────

// HealthTracker monitors actual write timestamps against a stream's scheduled
// interval and computes timing health statistics. It is safe for concurrent use.
type HealthTracker struct {
	stream     *Stream
	windowSize int
	mu         sync.Mutex
	entries    []healthEntry
}

type healthEntry struct {
	scheduled time.Time
	actual    time.Time
}

// NewHealthTracker creates a HealthTracker for stream. windowSize is the
// maximum number of past writes kept for health calculation; values ≤ 0 are
// clamped to 100.
func NewHealthTracker(s *Stream, windowSize int) *HealthTracker {
	if windowSize <= 0 {
		windowSize = 100
	}
	return &HealthTracker{stream: s, windowSize: windowSize}
}

// Record registers that a write occurred at actual. It computes the nearest
// scheduled interval boundary and stores the lateness for health tracking.
func (h *HealthTracker) Record(actual time.Time) {
	interval := h.stream.Interval()
	var scheduled time.Time
	if interval > 0 {
		ns := actual.UnixNano()
		iNS := int64(interval)
		// Snap down to the nearest boundary, then add the tx offset.
		boundary := (ns / iNS) * iNS
		scheduled = time.Unix(0, boundary).Add(h.stream.TxOffset())
		// If the offset pushes us past actual, step back one interval.
		if scheduled.After(actual) {
			scheduled = scheduled.Add(-interval)
		}
	} else {
		scheduled = actual
	}

	h.mu.Lock()
	h.entries = append(h.entries, healthEntry{scheduled: scheduled, actual: actual})
	if len(h.entries) > h.windowSize {
		h.entries = h.entries[1:]
	}
	h.mu.Unlock()
}

// Health returns a StreamHealth snapshot computed from the recorded writes.
func (h *HealthTracker) Health() StreamHealth {
	h.mu.Lock()
	entries := make([]healthEntry, len(h.entries))
	copy(entries, h.entries)
	h.mu.Unlock()

	const lateThreshold = 20 * time.Microsecond

	sh := StreamHealth{
		Topic:    h.stream.Topic,
		Interval: h.stream.Interval(),
		TxOffset: h.stream.TxOffset(),
	}
	for _, e := range entries {
		sh.WriteCount++
		lateness := e.actual.Sub(e.scheduled)
		if lateness < 0 {
			lateness = 0
		}
		if lateness > sh.MaxLateness {
			sh.MaxLateness = lateness
		}
		if lateness > lateThreshold {
			sh.LateWrites++
		}
	}
	if sh.WriteCount == 0 {
		sh.Healthy = true
	} else {
		sh.Healthy = sh.LateWrites*100/sh.WriteCount < 5
	}
	return sh
}

// Reset clears all recorded write history.
func (h *HealthTracker) Reset() {
	h.mu.Lock()
	h.entries = h.entries[:0]
	h.mu.Unlock()
}

// ── TAPRIOConfig ──────────────────────────────────────────────────────────────

// TAPRIOEntry is one gate control entry in an IEEE 802.1Qbv gate control list.
// GateMask is a bitmask of open traffic classes (bit N = TC N open).
type TAPRIOEntry struct {
	GateMask uint8
	Interval time.Duration
}

// TAPRIOConfig holds a TAPRIO gate control list derived from a StreamConfig.
// Use TAPRIOFromStreams to build one, then TCCommand to generate the
// corresponding tc(8) qdisc command.
type TAPRIOConfig struct {
	// CycleTime is the total schedule cycle (sum of all stream intervals).
	CycleTime time.Duration
	// Entries is the ordered list of gate control entries.
	Entries []TAPRIOEntry
}

// TAPRIOFromStreams derives a simple TAPRIO gate schedule from cfg.
// Each stream's PCP value determines its traffic class (TC = PCP).
// The gate opens each TC exclusively for its TransmitInterval; other TCs
// are closed during that slot. Returns an error if cfg has no streams or
// all streams have a zero interval.
func TAPRIOFromStreams(cfg *StreamConfig) (*TAPRIOConfig, error) {
	if cfg == nil || len(cfg.Streams) == 0 {
		return nil, fmt.Errorf("tsn: TAPRIOFromStreams: no streams configured")
	}

	var cycleNS int64
	for i := range cfg.Streams {
		s := &cfg.Streams[i]
		if s.IntervalUS > 0 {
			cycleNS += s.IntervalUS * 1000
		}
	}
	if cycleNS == 0 {
		return nil, fmt.Errorf("tsn: TAPRIOFromStreams: all streams have zero interval")
	}

	tc := &TAPRIOConfig{CycleTime: time.Duration(cycleNS)}
	for i := range cfg.Streams {
		s := &cfg.Streams[i]
		if s.IntervalUS <= 0 {
			continue
		}
		tc.Entries = append(tc.Entries, TAPRIOEntry{
			GateMask: uint8(1 << s.PCP),
			Interval: time.Duration(s.IntervalUS) * time.Microsecond,
		})
	}
	return tc, nil
}

// TCCommand returns a tc(8) command string that programs this TAPRIO schedule
// on iface. baseTimeNS is the TAI base time in nanoseconds (use 0 to start
// at the beginning of the epoch). The output is a starting-point template;
// operators may need to adjust the num_tc / map / queues arguments to match
// their specific NIC and traffic-class configuration.
func (t *TAPRIOConfig) TCCommand(iface string, baseTimeNS int64) string {
	var b strings.Builder
	fmt.Fprintf(&b, "tc qdisc replace dev %s parent root handle 100 taprio", iface)
	fmt.Fprintf(&b, " num_tc 8")
	fmt.Fprintf(&b, " map 0 1 2 3 4 5 6 7")
	fmt.Fprintf(&b, " queues 1@0 1@1 1@2 1@3 1@4 1@5 1@6 1@7")
	fmt.Fprintf(&b, " base-time %d", baseTimeNS)
	for _, e := range t.Entries {
		fmt.Fprintf(&b, " sched-entry S %02x %d", e.GateMask, int64(e.Interval))
	}
	fmt.Fprintf(&b, " flags 0x1")
	return b.String()
}
