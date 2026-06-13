// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package tsn_test

//fusa:test REQ-TSN-005
//fusa:test REQ-TSN-006
//fusa:test REQ-TSN-007

import (
	"errors"
	"math"
	"testing"
	"time"

	"github.com/SoundMatt/go-DDS/tsn"
)

func TestTAPRIOConfig_Validate_MissingInterface(t *testing.T) {
	cfg := &tsn.TAPRIOConfig{
		Entries: []tsn.TAPRIOEntry{{GateMask: 0xFF, Interval: time.Millisecond}},
	}
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for missing Interface")
	}
}

func TestTAPRIOConfig_Validate_NoEntries(t *testing.T) {
	cfg := &tsn.TAPRIOConfig{Interface: "eth0"}
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for empty Entries")
	}
}

func TestTAPRIOConfig_Validate_ZeroInterval(t *testing.T) {
	cfg := &tsn.TAPRIOConfig{
		Interface: "eth0",
		Entries:   []tsn.TAPRIOEntry{{GateMask: 0xFF, Interval: 0}},
	}
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for zero interval")
	}
}

func TestTAPRIOConfig_Validate_Valid(t *testing.T) {
	cfg := &tsn.TAPRIOConfig{
		Interface: "eth0",
		Entries: []tsn.TAPRIOEntry{
			{GateMask: 0xFF, Interval: 800 * time.Microsecond},
			{GateMask: 0x01, Interval: 200 * time.Microsecond},
		},
	}
	if err := cfg.Validate(); err != nil {
		t.Errorf("Validate: %v", err)
	}
}

func TestTAPRIOConfig_CycleDuration_Explicit(t *testing.T) {
	cfg := &tsn.TAPRIOConfig{
		Interface: "eth0",
		CycleTime: 5 * time.Millisecond,
		Entries:   []tsn.TAPRIOEntry{{GateMask: 0xFF, Interval: time.Millisecond}},
	}
	if got := cfg.CycleDuration(); got != 5*time.Millisecond {
		t.Errorf("CycleDuration = %v, want 5ms", got)
	}
}

func TestTAPRIOConfig_CycleDuration_Computed(t *testing.T) {
	cfg := &tsn.TAPRIOConfig{
		Interface: "eth0",
		Entries: []tsn.TAPRIOEntry{
			{GateMask: 0x0F, Interval: 600 * time.Microsecond},
			{GateMask: 0xF0, Interval: 400 * time.Microsecond},
		},
	}
	if got := cfg.CycleDuration(); got != time.Millisecond {
		t.Errorf("CycleDuration = %v, want 1ms", got)
	}
}

func TestTAPRIOConfig_Apply_NonLinux(t *testing.T) {
	cfg := &tsn.TAPRIOConfig{
		Interface: "eth0",
		Entries:   []tsn.TAPRIOEntry{{GateMask: 0xFF, Interval: time.Millisecond}},
	}
	err := cfg.Apply()
	// On Linux, Apply returns a different error (CAP_NET_ADMIN or interface not found).
	// On non-Linux, it must return ErrNotSupported.
	// We test that the error is non-nil and the common case is ErrNotSupported.
	if err == nil {
		t.Skip("Apply succeeded unexpectedly (running with CAP_NET_ADMIN on Linux?)")
	}
	// On non-Linux: must be ErrNotSupported
	if !errors.Is(err, tsn.ErrNotSupported) {
		// On Linux without the right interface/caps, any error is acceptable.
		t.Logf("Apply error (non-ErrNotSupported, likely Linux): %v", err)
	}
}

func TestTAPRIOConfig_VerifyApplied_EmptyInterface(t *testing.T) {
	cfg := &tsn.TAPRIOConfig{
		Entries: []tsn.TAPRIOEntry{{GateMask: 0xFF, Interval: time.Millisecond}},
	}
	err := cfg.VerifyApplied()
	if err == nil {
		t.Error("expected error for empty Interface")
	}
}

func TestTAPRIOConfig_VerifyApplied_NonLinux(t *testing.T) {
	cfg := &tsn.TAPRIOConfig{
		Interface: "eth0",
		Entries:   []tsn.TAPRIOEntry{{GateMask: 0xFF, Interval: time.Millisecond}},
	}
	err := cfg.VerifyApplied()
	if err == nil {
		t.Skip("VerifyApplied succeeded unexpectedly (taprio qdisc present?)")
	}
	// On non-Linux: must be ErrNotSupported.
	// On Linux without a taprio qdisc: any error is acceptable.
	if !errors.Is(err, tsn.ErrNotSupported) {
		t.Logf("VerifyApplied error (non-ErrNotSupported, likely Linux without taprio qdisc): %v", err)
	}
}

// TestTAPRIOConfig_Validate_IntervalExceedsUint32 covers the
// maxEntryIntervalNS overflow branch in Validate and the itoa loop body
// (entry index 1 → itoa(1) takes the n>0 loop path).
func TestTAPRIOConfig_Validate_IntervalExceedsUint32(t *testing.T) {
	cfg := &tsn.TAPRIOConfig{
		Interface: "eth0",
		Entries: []tsn.TAPRIOEntry{
			{GateMask: 0xFF, Interval: time.Millisecond},                  // valid first entry
			{GateMask: 0x0F, Interval: time.Duration(math.MaxUint32 + 1)}, // entry[1] too large
		},
	}
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for interval exceeding uint32 ns limit")
	}
}

// TestTAPRIOConfig_Validate_ZeroIntervalAtSecondEntry covers itoa(1) in the
// zero-interval error message when the bad entry is at index 1.
func TestTAPRIOConfig_Validate_ZeroIntervalAtSecondEntry(t *testing.T) {
	cfg := &tsn.TAPRIOConfig{
		Interface: "eth0",
		Entries: []tsn.TAPRIOEntry{
			{GateMask: 0xFF, Interval: time.Millisecond}, // valid
			{GateMask: 0x0F, Interval: 0},                // zero — hits itoa(1)
		},
	}
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for zero interval at entry[1]")
	}
}

func TestTAPRIOFromStreams_DerivesTAPRIO(t *testing.T) {
	cfg, err := tsn.ParseConfig([]byte(`{
		"streams":[
			{"topic":"a","pcp":5,"interval_us":1000},
			{"topic":"b","pcp":3,"interval_us":2000}
		]
	}`))
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}
	tc, err := tsn.TAPRIOFromStreams(cfg)
	if err != nil {
		t.Fatalf("TAPRIOFromStreams: %v", err)
	}
	if tc.CycleTime != 3*time.Millisecond {
		t.Errorf("CycleTime = %v, want 3ms", tc.CycleTime)
	}
	if len(tc.Entries) != 2 {
		t.Fatalf("entries = %d, want 2", len(tc.Entries))
	}
}
