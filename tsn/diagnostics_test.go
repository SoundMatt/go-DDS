// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package tsn_test

import (
	"strings"
	"testing"
	"time"

	"github.com/SoundMatt/go-DDS/tsn"
)

// ── helpers ───────────────────────────────────────────────────────────────────

func streamWith(topic string, intervalUS, txOffsetUS int64) *tsn.Stream {
	s := &tsn.Stream{
		Topic:      topic,
		PCP:        5,
		IntervalUS: intervalUS,
		TxOffsetUS: txOffsetUS,
	}
	return s
}

// ── HealthTracker ─────────────────────────────────────────────────────────────

func TestHealthTracker_NoWrites_IsHealthy(t *testing.T) {
	s := streamWith("vehicle/speed", 125, 0)
	ht := tsn.NewHealthTracker(s, 10)
	sh := ht.Health()
	if !sh.Healthy {
		t.Error("no writes: expected healthy")
	}
	if sh.WriteCount != 0 {
		t.Errorf("WriteCount: got %d, want 0", sh.WriteCount)
	}
}

func TestHealthTracker_OnTimeWrites_IsHealthy(t *testing.T) {
	s := streamWith("vehicle/speed", 1000, 0) // 1 ms interval
	ht := tsn.NewHealthTracker(s, 100)

	// Simulate 10 on-time writes (all within 5µs of their slot).
	base := time.Now().Truncate(time.Millisecond)
	for i := 0; i < 10; i++ {
		ht.Record(base.Add(time.Duration(i) * time.Millisecond))
	}
	sh := ht.Health()
	if !sh.Healthy {
		t.Errorf("on-time writes: expected healthy; LateWrites=%d WriteCount=%d", sh.LateWrites, sh.WriteCount)
	}
	if sh.WriteCount != 10 {
		t.Errorf("WriteCount: got %d, want 10", sh.WriteCount)
	}
}

func TestHealthTracker_ManyLateWrites_IsUnhealthy(t *testing.T) {
	s := streamWith("cam/feed", 1000, 0) // 1 ms interval
	ht := tsn.NewHealthTracker(s, 100)

	// Record writes that are 1 ms late (well past the 20µs threshold).
	base := time.Now().Truncate(time.Millisecond)
	for i := 0; i < 10; i++ {
		// Schedule is at i*1ms; we record at i*1ms + 500µs (late).
		ht.Record(base.Add(time.Duration(i)*time.Millisecond + 500*time.Microsecond))
	}
	sh := ht.Health()
	if sh.Healthy {
		t.Errorf("many late writes: expected unhealthy; LateWrites=%d WriteCount=%d", sh.LateWrites, sh.WriteCount)
	}
	if sh.LateWrites == 0 {
		t.Error("expected LateWrites > 0")
	}
}

func TestHealthTracker_Fields_Populated(t *testing.T) {
	s := streamWith("can/bus", 500, 50) // 500µs interval, 50µs offset
	ht := tsn.NewHealthTracker(s, 10)
	now := time.Now()
	ht.Record(now)

	sh := ht.Health()
	if sh.Topic != "can/bus" {
		t.Errorf("Topic: got %q, want %q", sh.Topic, "can/bus")
	}
	if sh.Interval != 500*time.Microsecond {
		t.Errorf("Interval: got %v, want 500µs", sh.Interval)
	}
	if sh.TxOffset != 50*time.Microsecond {
		t.Errorf("TxOffset: got %v, want 50µs", sh.TxOffset)
	}
}

func TestHealthTracker_Reset_ClearsHistory(t *testing.T) {
	s := streamWith("vehicle/speed", 1000, 0)
	ht := tsn.NewHealthTracker(s, 10)

	ht.Record(time.Now())
	ht.Record(time.Now())
	if ht.Health().WriteCount != 2 {
		t.Fatalf("before reset: WriteCount should be 2")
	}

	ht.Reset()
	if ht.Health().WriteCount != 0 {
		t.Errorf("after reset: WriteCount should be 0, got %d", ht.Health().WriteCount)
	}
}

func TestHealthTracker_WindowSize_LimitsHistory(t *testing.T) {
	s := streamWith("vehicle/speed", 1000, 0)
	ht := tsn.NewHealthTracker(s, 5) // window = 5

	base := time.Now().Truncate(time.Millisecond)
	for i := 0; i < 10; i++ {
		ht.Record(base.Add(time.Duration(i) * time.Millisecond))
	}
	sh := ht.Health()
	if sh.WriteCount > 5 {
		t.Errorf("window=5: WriteCount=%d, want ≤5", sh.WriteCount)
	}
}

func TestHealthTracker_DefaultWindowSize(t *testing.T) {
	s := streamWith("vehicle/speed", 1000, 0)
	ht := tsn.NewHealthTracker(s, 0) // 0 → default 100
	// Should not panic.
	ht.Record(time.Now())
	sh := ht.Health()
	if sh.WriteCount != 1 {
		t.Errorf("WriteCount: got %d, want 1", sh.WriteCount)
	}
}

func TestHealthTracker_ZeroInterval_NocrashNoLate(t *testing.T) {
	s := streamWith("vehicle/speed", 0, 0) // no interval configured
	ht := tsn.NewHealthTracker(s, 10)
	ht.Record(time.Now())
	sh := ht.Health()
	// With zero interval, lateness is always 0 (scheduled == actual).
	if sh.LateWrites != 0 {
		t.Errorf("zero interval: LateWrites=%d, want 0", sh.LateWrites)
	}
}

func TestHealthTracker_MaxLateness_Tracked(t *testing.T) {
	s := streamWith("vehicle/speed", 1000, 0) // 1 ms
	ht := tsn.NewHealthTracker(s, 100)

	base := time.Now().Truncate(time.Millisecond)
	// One very late write.
	ht.Record(base.Add(900 * time.Microsecond)) // 900µs late
	sh := ht.Health()
	if sh.MaxLateness < 800*time.Microsecond {
		t.Errorf("MaxLateness: got %v, expected ≥800µs", sh.MaxLateness)
	}
}

// ── TAPRIOFromStreams ─────────────────────────────────────────────────────────

func TestTAPRIOFromStreams_NilConfig(t *testing.T) {
	_, err := tsn.TAPRIOFromStreams(nil)
	if err == nil {
		t.Fatal("expected error for nil config")
	}
}

func TestTAPRIOFromStreams_EmptyStreams(t *testing.T) {
	cfg := &tsn.StreamConfig{}
	_, err := tsn.TAPRIOFromStreams(cfg)
	if err == nil {
		t.Fatal("expected error for empty streams")
	}
}

func TestTAPRIOFromStreams_AllZeroIntervals(t *testing.T) {
	cfg := &tsn.StreamConfig{
		Streams: []tsn.Stream{{Topic: "a", PCP: 1, IntervalUS: 0}},
	}
	_, err := tsn.TAPRIOFromStreams(cfg)
	if err == nil {
		t.Fatal("expected error when all intervals are zero")
	}
}

func TestTAPRIOFromStreams_SingleStream(t *testing.T) {
	cfg := &tsn.StreamConfig{
		Streams: []tsn.Stream{
			{Topic: "vehicle/speed", PCP: 5, IntervalUS: 125},
		},
	}
	tc, err := tsn.TAPRIOFromStreams(cfg)
	if err != nil {
		t.Fatalf("TAPRIOFromStreams: %v", err)
	}
	if tc.CycleTime != 125*time.Microsecond {
		t.Errorf("CycleTime: got %v, want 125µs", tc.CycleTime)
	}
	if len(tc.Entries) != 1 {
		t.Fatalf("Entries: got %d, want 1", len(tc.Entries))
	}
	if tc.Entries[0].GateMask != (1 << 5) {
		t.Errorf("GateMask: got 0x%02x, want 0x%02x", tc.Entries[0].GateMask, 1<<5)
	}
	if tc.Entries[0].Interval != 125*time.Microsecond {
		t.Errorf("Interval: got %v, want 125µs", tc.Entries[0].Interval)
	}
}

func TestTAPRIOFromStreams_MultiStream(t *testing.T) {
	cfg := &tsn.StreamConfig{
		Streams: []tsn.Stream{
			{Topic: "a", PCP: 3, IntervalUS: 100},
			{Topic: "b", PCP: 5, IntervalUS: 200},
		},
	}
	tc, err := tsn.TAPRIOFromStreams(cfg)
	if err != nil {
		t.Fatalf("TAPRIOFromStreams: %v", err)
	}
	want := 300 * time.Microsecond
	if tc.CycleTime != want {
		t.Errorf("CycleTime: got %v, want %v", tc.CycleTime, want)
	}
	if len(tc.Entries) != 2 {
		t.Errorf("Entries: got %d, want 2", len(tc.Entries))
	}
}

func TestTAPRIOFromStreams_SkipsZeroInterval(t *testing.T) {
	cfg := &tsn.StreamConfig{
		Streams: []tsn.Stream{
			{Topic: "a", PCP: 2, IntervalUS: 0},   // skipped
			{Topic: "b", PCP: 4, IntervalUS: 250}, // included
		},
	}
	tc, err := tsn.TAPRIOFromStreams(cfg)
	if err != nil {
		t.Fatalf("TAPRIOFromStreams: %v", err)
	}
	if len(tc.Entries) != 1 {
		t.Errorf("Entries: got %d, want 1 (zero-interval stream should be skipped)", len(tc.Entries))
	}
}

// ── TCCommand ────────────────────────────────────────────────────────────────

func TestTCCommand_ContainsIface(t *testing.T) {
	cfg := &tsn.StreamConfig{
		Streams: []tsn.Stream{{Topic: "x", PCP: 1, IntervalUS: 500}},
	}
	tc, err := tsn.TAPRIOFromStreams(cfg)
	if err != nil {
		t.Fatalf("TAPRIOFromStreams: %v", err)
	}
	cmd := tc.TCCommand("eth0", 0)
	if !strings.Contains(cmd, "eth0") {
		t.Errorf("TCCommand: iface missing from output: %s", cmd)
	}
}

func TestTCCommand_ContainsTaprio(t *testing.T) {
	cfg := &tsn.StreamConfig{
		Streams: []tsn.Stream{{Topic: "x", PCP: 3, IntervalUS: 125}},
	}
	tc, err := tsn.TAPRIOFromStreams(cfg)
	if err != nil {
		t.Fatalf("TAPRIOFromStreams: %v", err)
	}
	cmd := tc.TCCommand("eth0", 0)
	if !strings.Contains(cmd, "taprio") {
		t.Errorf("TCCommand: 'taprio' missing from output: %s", cmd)
	}
}

func TestTCCommand_ContainsSchedEntry(t *testing.T) {
	cfg := &tsn.StreamConfig{
		Streams: []tsn.Stream{{Topic: "x", PCP: 5, IntervalUS: 125}},
	}
	tc, err := tsn.TAPRIOFromStreams(cfg)
	if err != nil {
		t.Fatalf("TAPRIOFromStreams: %v", err)
	}
	cmd := tc.TCCommand("eth0", 1000000000)
	if !strings.Contains(cmd, "sched-entry") {
		t.Errorf("TCCommand: 'sched-entry' missing from output: %s", cmd)
	}
	if !strings.Contains(cmd, "base-time 1000000000") {
		t.Errorf("TCCommand: base-time missing or wrong: %s", cmd)
	}
}

func TestTCCommand_GateMaskEncoded(t *testing.T) {
	cfg := &tsn.StreamConfig{
		Streams: []tsn.Stream{{Topic: "x", PCP: 0, IntervalUS: 125}}, // TC0 → mask 0x01
	}
	tc, err := tsn.TAPRIOFromStreams(cfg)
	if err != nil {
		t.Fatalf("TAPRIOFromStreams: %v", err)
	}
	cmd := tc.TCCommand("eth0", 0)
	if !strings.Contains(cmd, "01") {
		t.Errorf("TCCommand: gate mask 0x01 missing from output: %s", cmd)
	}
}

func TestTCCommand_MultipleEntries(t *testing.T) {
	cfg := &tsn.StreamConfig{
		Streams: []tsn.Stream{
			{Topic: "a", PCP: 2, IntervalUS: 100},
			{Topic: "b", PCP: 6, IntervalUS: 200},
		},
	}
	tc, err := tsn.TAPRIOFromStreams(cfg)
	if err != nil {
		t.Fatalf("TAPRIOFromStreams: %v", err)
	}
	cmd := tc.TCCommand("enp3s0", 0)
	// Both entries must appear as sched-entry lines.
	count := strings.Count(cmd, "sched-entry")
	if count != 2 {
		t.Errorf("TCCommand: expected 2 sched-entry fields, got %d in: %s", count, cmd)
	}
}
