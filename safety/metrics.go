// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package safety

//fusa:req REQ-SAFETY-015
//fusa:req REQ-SAFETY-016
//fusa:req REQ-SAFETY-017

import "sync/atomic"

// SafetyEventKind categorises safety events emitted by E2ESubscriber.
type SafetyEventKind int

const (
	// SafetyEventCRCFailure is emitted when a frame fails its CRC check.
	SafetyEventCRCFailure SafetyEventKind = iota
	// SafetyEventSequenceGap is emitted when one or more sequence numbers are skipped.
	SafetyEventSequenceGap
	// SafetyEventStaleSample is emitted when a sample exceeds MaxAge.
	SafetyEventStaleSample
	// SafetyEventHeaderTooShort is emitted when a payload is shorter than the E2E header.
	SafetyEventHeaderTooShort
	// SafetyEventSchemaViolation is emitted when a sample's type does not match the registered schema.
	SafetyEventSchemaViolation
)

func (k SafetyEventKind) String() string {
	switch k {
	case SafetyEventCRCFailure:
		return "crc_failure"
	case SafetyEventSequenceGap:
		return "sequence_gap"
	case SafetyEventStaleSample:
		return "stale_sample"
	case SafetyEventHeaderTooShort:
		return "header_too_short"
	case SafetyEventSchemaViolation:
		return "schema_violation"
	default:
		return "unknown"
	}
}

// SafetyEvent represents a single E2E safety violation detected by an E2ESubscriber.
// It is broadcast to registered safety event listeners (e.g., monitor.Monitor).
type SafetyEvent struct {
	// Kind classifies the violation.
	Kind SafetyEventKind
	// Topic is the DDS topic on which the violation was detected.
	Topic string
	// Counter is the sequence counter from the frame header (0 if unavailable).
	Counter uint32
	// Message is a human-readable description of the violation.
	Message string
}

// Snapshot is a point-in-time copy of violation counters for one subscriber.
type Snapshot struct {
	Topic            string `json:"topic"`
	CRCFailures      uint64 `json:"crc_failures"`
	SequenceGaps     uint64 `json:"sequence_gaps"`
	StaleSamples     uint64 `json:"stale_samples"`
	HeaderTooShort   uint64 `json:"header_too_short"`
	SchemaViolations uint64 `json:"schema_violations"`
	ValidSamples     uint64 `json:"valid_samples"`
}

// metrics tracks cumulative violation counters for one E2ESubscriber.
// All fields are updated atomically; Snapshot provides a consistent read.
type metrics struct {
	topic            string
	crcFailures      atomic.Uint64
	sequenceGaps     atomic.Uint64
	staleSamples     atomic.Uint64
	headerTooShort   atomic.Uint64
	schemaViolations atomic.Uint64
	validSamples     atomic.Uint64
}

func newMetrics(topic string) *metrics {
	return &metrics{topic: topic}
}

// Snapshot returns a point-in-time copy of all counters.
func (m *metrics) Snapshot() Snapshot {
	return Snapshot{
		Topic:            m.topic,
		CRCFailures:      m.crcFailures.Load(),
		SequenceGaps:     m.sequenceGaps.Load(),
		StaleSamples:     m.staleSamples.Load(),
		HeaderTooShort:   m.headerTooShort.Load(),
		SchemaViolations: m.schemaViolations.Load(),
		ValidSamples:     m.validSamples.Load(),
	}
}

// SafetyMetricsProvider is implemented by components that expose E2E violation counters.
type SafetyMetricsProvider interface {
	// SafetyMetrics returns a point-in-time snapshot of safety violation counters.
	SafetyMetrics() Snapshot
}
