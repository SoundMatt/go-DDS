// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package safety

//fusa:req REQ-SAFETY-018
//fusa:req REQ-SAFETY-019

import (
	"sync"
	"time"
)

// RateThresholds defines the maximum tolerated violation rate (events per second)
// for each safety event kind. A threshold of zero disables that check.
type RateThresholds struct {
	CRCFailureRate      float64
	SequenceGapRate     float64
	StaleSampleRate     float64
	SchemaViolationRate float64
}

// RateAlert is emitted by RateMonitor when a violation rate exceeds its threshold.
type RateAlert struct {
	// Topic is the DDS topic for which the threshold was exceeded.
	Topic string
	// Kind is the safety event category that triggered the alert.
	Kind SafetyEventKind
	// Rate is the observed violation rate (events per second) over the last window.
	Rate float64
	// Threshold is the configured maximum rate that was exceeded.
	Threshold float64
}

// RateMonitor polls a SafetyMetricsProvider at a fixed interval and invokes
// an alert callback whenever any violation rate exceeds its configured threshold.
// It is safe for concurrent use.
type RateMonitor struct {
	p          SafetyMetricsProvider
	thresholds RateThresholds
	interval   time.Duration
	onAlert    func(RateAlert)

	mu       sync.Mutex
	prev     Snapshot
	stopOnce sync.Once
	done     chan struct{}
}

// NewRateMonitor starts a RateMonitor that polls p every interval. When a
// violation rate exceeds a threshold, onAlert is called synchronously from
// the background goroutine. Call Stop to release resources. Intervals ≤ 0
// default to 5 seconds.
func NewRateMonitor(p SafetyMetricsProvider, interval time.Duration, thresholds RateThresholds, onAlert func(RateAlert)) *RateMonitor {
	if interval <= 0 {
		interval = 5 * time.Second
	}
	rm := &RateMonitor{
		p:          p,
		thresholds: thresholds,
		interval:   interval,
		onAlert:    onAlert,
		done:       make(chan struct{}),
		prev:       p.SafetyMetrics(),
	}
	go rm.loop()
	return rm
}

// Stop shuts down the background polling goroutine. It is safe to call once.
func (rm *RateMonitor) Stop() {
	rm.stopOnce.Do(func() { close(rm.done) })
}

func (rm *RateMonitor) loop() {
	tick := time.NewTicker(rm.interval)
	defer tick.Stop()
	for {
		select {
		case <-tick.C:
			rm.check()
		case <-rm.done:
			return
		}
	}
}

func (rm *RateMonitor) check() {
	rm.mu.Lock()
	prev := rm.prev
	rm.mu.Unlock()

	cur := rm.p.SafetyMetrics()
	secs := rm.interval.Seconds()

	type entry struct {
		kind            SafetyEventKind
		curVal, prevVal uint64
		threshold       float64
	}
	entries := [4]entry{
		{SafetyEventCRCFailure, cur.CRCFailures, prev.CRCFailures, rm.thresholds.CRCFailureRate},
		{SafetyEventSequenceGap, cur.SequenceGaps, prev.SequenceGaps, rm.thresholds.SequenceGapRate},
		{SafetyEventStaleSample, cur.StaleSamples, prev.StaleSamples, rm.thresholds.StaleSampleRate},
		{SafetyEventSchemaViolation, cur.SchemaViolations, prev.SchemaViolations, rm.thresholds.SchemaViolationRate},
	}
	for _, e := range entries {
		if e.threshold <= 0 || e.curVal <= e.prevVal {
			continue
		}
		rate := float64(e.curVal-e.prevVal) / secs
		if rate > e.threshold {
			rm.onAlert(RateAlert{Topic: cur.Topic, Kind: e.kind, Rate: rate, Threshold: e.threshold})
		}
	}

	rm.mu.Lock()
	rm.prev = cur
	rm.mu.Unlock()
}
