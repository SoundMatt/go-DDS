// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Package dds defines the Go interface for Data Distribution Service (DDS)
// publish/subscribe operations.
//
// The interface is intentionally narrow: it covers the pub/sub primitives
// needed for vehicle-signal transport and nothing more.
//
// Choose an implementation by importing one of the sub-packages and calling
// its New function:
//
//	import "github.com/SoundMatt/go-DDS/mock"    // in-process, no CGo
//	import "github.com/SoundMatt/go-DDS/cyclone" // CycloneDDS via CGo
//	import "github.com/SoundMatt/go-DDS/rtps"    // pure-Go RTPS/UDP
//
// All packages expose a New(Domain) (Participant, error) constructor that
// satisfies this package's Participant interface.
package dds

import (
	"context"
	"errors"
	"reflect"
	"time"
)

// ── Sentinel errors ───────────────────────────────────────────────────────────

// ErrClosed is returned when an operation is attempted on a closed entity.
var ErrClosed = errors.New("dds: entity is closed")

// ErrTopicEmpty is returned when an empty topic string is passed.
var ErrTopicEmpty = errors.New("dds: topic name must not be empty")

// ── Domain ────────────────────────────────────────────────────────────────────

// Domain is a DDS domain identifier (0–232 inclusive per the DDS spec).
// Participants on the same domain and network segment discover each other
// automatically without a broker.
type Domain int

// ── QoS ──────────────────────────────────────────────────────────────────────

// ReliabilityKind controls delivery guarantees for a topic endpoint.
type ReliabilityKind int

const (
	// BestEffort delivers samples without retransmission. Suitable for
	// high-frequency sensor data where occasional loss is acceptable.
	BestEffort ReliabilityKind = iota
	// Reliable retransmits lost samples until acknowledged. Required for
	// command/control and actuator writes.
	Reliable
)

// DurabilityKind controls whether late-joining subscribers receive
// historical samples that were published before they joined.
type DurabilityKind int

const (
	// Volatile discards samples as soon as they are delivered.
	Volatile DurabilityKind = iota
	// TransientLocal retains the last N samples so that late joiners
	// receive current state on subscription.
	TransientLocal
)

// QoS bundles the policies that govern a single publisher or subscriber
// endpoint.
type QoS struct {
	Reliability  ReliabilityKind
	Durability   DurabilityKind
	HistoryDepth int           // 0 means implementation default (typically 1)
	Deadline     time.Duration // 0 = disabled; publisher fires DeadlineCallback if no Write within this period
}

// DefaultQoS is BestEffort + Volatile with implementation-default history.
var DefaultQoS = QoS{
	Reliability:  BestEffort,
	Durability:   Volatile,
	HistoryDepth: 1,
}

// ReliableQoS is Reliable + TransientLocal. Use for actuator commands and
// any topic where a late-joining subscriber must receive the current value.
var ReliableQoS = QoS{
	Reliability:  Reliable,
	Durability:   TransientLocal,
	HistoryDepth: 1,
}

// ── Sample ────────────────────────────────────────────────────────────────────

// Sample is a single data sample delivered to a Subscriber.
type Sample struct {
	Topic   string
	Payload []byte
}

// ── SubscriberOption ──────────────────────────────────────────────────────────

// SubscriberConfig holds per-subscriber options applied at construction time.
// It is exported so that implementation packages (mock, rtps, cyclone) can
// read the resolved configuration without duplicating the option-merge logic.
type SubscriberConfig struct {
	Filter func(Sample) bool
}

// SubscriberOption configures a subscriber at creation time.
type SubscriberOption func(*SubscriberConfig)

// WithFilter returns a SubscriberOption that applies fn as a content filter.
// Only samples for which fn returns true are delivered to the subscriber's
// channel; non-matching samples are discarded silently.
func WithFilter(fn func(Sample) bool) SubscriberOption {
	return func(c *SubscriberConfig) { c.Filter = fn }
}

// ApplySubscriberOpts merges a slice of SubscriberOption into a SubscriberConfig.
func ApplySubscriberOpts(opts []SubscriberOption) SubscriberConfig {
	var c SubscriberConfig
	for _, o := range opts {
		o(&c)
	}
	return c
}

// ── Metrics ───────────────────────────────────────────────────────────────────

// Metrics holds cumulative statistics for a participant.
type Metrics struct {
	WriteCount     uint64
	DeliverCount   uint64
	DropCount      uint64
	BytesWritten   uint64
	BytesDelivered uint64
}

// MetricsProvider is implemented by participants that expose runtime statistics.
type MetricsProvider interface {
	Metrics() Metrics
}

// ── Interfaces ────────────────────────────────────────────────────────────────

// Participant is the DDS domain participant — the root factory for all DDS
// entities. Create one per process per domain. A Participant is safe for
// concurrent use from multiple goroutines.
type Participant interface {
	// NewPublisher creates a writer for the named topic using the given QoS.
	NewPublisher(topic string, qos QoS) (Publisher, error)

	// NewSubscriber creates a reader for the named topic using the given QoS.
	// Optional SubscriberOption values configure content filtering and other
	// per-subscriber policies.
	NewSubscriber(topic string, qos QoS, opts ...SubscriberOption) (Subscriber, error)

	// Close releases all DDS resources held by this participant.
	Close() error
}

// Publisher writes samples to a single DDS topic.
// A Publisher is safe for concurrent use from multiple goroutines.
type Publisher interface {
	Write(payload []byte) error
	Close() error
}

// Subscriber reads samples from a single DDS topic as a Go channel.
// A Subscriber is safe for concurrent use from multiple goroutines.
type Subscriber interface {
	C() <-chan Sample
	Close() error
}

// ── WaitSet ───────────────────────────────────────────────────────────────────

// WaitSet multiplexes over a set of subscribers, blocking until any one of
// them delivers a sample.
type WaitSet struct {
	subs []Subscriber
}

// NewWaitSet creates a WaitSet that monitors the given subscribers.
func NewWaitSet(subs ...Subscriber) *WaitSet {
	s := make([]Subscriber, len(subs))
	copy(s, subs)
	return &WaitSet{subs: s}
}

// Wait blocks until a sample is available on any attached subscriber, or until
// ctx is cancelled.
func (ws *WaitSet) Wait(ctx context.Context) (Sample, Subscriber, error) {
	cases := make([]reflect.SelectCase, 1+len(ws.subs))
	cases[0] = reflect.SelectCase{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(ctx.Done())}
	for i, sub := range ws.subs {
		cases[1+i] = reflect.SelectCase{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(sub.C())}
	}
	for {
		chosen, recv, ok := reflect.Select(cases)
		if chosen == 0 {
			return Sample{}, nil, ctx.Err()
		}
		if !ok {
			cases[chosen] = reflect.SelectCase{Dir: reflect.SelectDefault}
			all := true
			for _, c := range cases[1:] {
				if c.Dir != reflect.SelectDefault {
					all = false
					break
				}
			}
			if all {
				return Sample{}, nil, ctx.Err()
			}
			continue
		}
		s, ok2 := recv.Interface().(Sample)
		if !ok2 {
			continue
		}
		return s, ws.subs[chosen-1], nil
	}
}
