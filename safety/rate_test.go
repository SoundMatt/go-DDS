// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package safety_test

//fusa:test REQ-SAFETY-018
//fusa:test REQ-SAFETY-019

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/SoundMatt/go-DDS/safety"
)

// fixedProvider is a thread-safe SafetyMetricsProvider that returns a mutable snapshot.
type fixedProvider struct {
	mu   sync.Mutex
	snap safety.Snapshot
}

func (p *fixedProvider) SafetyMetrics() safety.Snapshot {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.snap
}

func (p *fixedProvider) set(s safety.Snapshot) {
	p.mu.Lock()
	p.snap = s
	p.mu.Unlock()
}

func TestRateMonitor_AlertOnExceedance(t *testing.T) {
	prov := &fixedProvider{snap: safety.Snapshot{Topic: "test/crc"}}
	var count atomic.Int32
	rm := safety.NewRateMonitor(prov, 10*time.Millisecond, safety.RateThresholds{
		CRCFailureRate: 5.0,
	}, func(a safety.RateAlert) {
		count.Add(1)
	})
	defer rm.Stop()

	// Advance by 100 in one 10 ms window → rate ≈ 10 000/s >> 5/s threshold.
	prov.set(safety.Snapshot{Topic: "test/crc", CRCFailures: 100})
	time.Sleep(60 * time.Millisecond)

	if count.Load() == 0 {
		t.Error("expected at least one alert for CRCFailure rate exceeding threshold")
	}
}

func TestRateMonitor_NoAlertBelowThreshold(t *testing.T) {
	prov := &fixedProvider{snap: safety.Snapshot{Topic: "test/seq"}}
	var count atomic.Int32
	rm := safety.NewRateMonitor(prov, 10*time.Millisecond, safety.RateThresholds{
		SequenceGapRate: 1_000_000.0, // extremely high — should never fire
	}, func(_ safety.RateAlert) {
		count.Add(1)
	})
	defer rm.Stop()

	prov.set(safety.Snapshot{Topic: "test/seq", SequenceGaps: 1})
	time.Sleep(60 * time.Millisecond)

	if count.Load() != 0 {
		t.Errorf("expected no alerts below threshold, got %d", count.Load())
	}
}

func TestRateMonitor_ZeroThreshold_Disabled(t *testing.T) {
	prov := &fixedProvider{snap: safety.Snapshot{Topic: "test/schema"}}
	var count atomic.Int32
	rm := safety.NewRateMonitor(prov, 10*time.Millisecond, safety.RateThresholds{
		SchemaViolationRate: 0, // disabled
	}, func(_ safety.RateAlert) {
		count.Add(1)
	})
	defer rm.Stop()

	prov.set(safety.Snapshot{Topic: "test/schema", SchemaViolations: 10000})
	time.Sleep(60 * time.Millisecond)

	if count.Load() != 0 {
		t.Errorf("zero threshold should disable check; got %d alerts", count.Load())
	}
}

func TestRateMonitor_DefaultInterval(t *testing.T) {
	// Interval ≤ 0 should not panic and should default to 5s.
	prov := &fixedProvider{}
	rm := safety.NewRateMonitor(prov, 0, safety.RateThresholds{}, func(_ safety.RateAlert) {})
	rm.Stop()
}

func TestRateMonitor_Stop_Idempotent(t *testing.T) {
	prov := &fixedProvider{}
	rm := safety.NewRateMonitor(prov, 10*time.Millisecond, safety.RateThresholds{}, func(_ safety.RateAlert) {})
	rm.Stop()
	rm.Stop() // second Stop must not panic
}

func TestRateMonitor_AlertKind(t *testing.T) {
	prov := &fixedProvider{snap: safety.Snapshot{Topic: "kind/test"}}
	var mu sync.Mutex
	var got safety.RateAlert
	rm := safety.NewRateMonitor(prov, 10*time.Millisecond, safety.RateThresholds{
		StaleSampleRate: 1.0,
	}, func(a safety.RateAlert) {
		mu.Lock()
		got = a
		mu.Unlock()
	})
	defer rm.Stop()

	prov.set(safety.Snapshot{Topic: "kind/test", StaleSamples: 500})
	time.Sleep(60 * time.Millisecond)

	mu.Lock()
	gotCopy := got
	mu.Unlock()

	if gotCopy.Kind != safety.SafetyEventStaleSample {
		t.Errorf("alert Kind = %v, want SafetyEventStaleSample", gotCopy.Kind)
	}
	if gotCopy.Topic != "kind/test" {
		t.Errorf("alert Topic = %q, want kind/test", gotCopy.Topic)
	}
	if gotCopy.Rate <= 0 {
		t.Errorf("alert Rate = %v, want > 0", gotCopy.Rate)
	}
}
