// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Package dds defines the Go interface for Data Distribution Service (DDS)
// publish/subscribe operations.
//
// The interface is intentionally narrow: it covers the pub/sub primitives
// needed for vehicle-signal transport (e.g. VISS/VISSR) and nothing more.
// Additional QoS policies, DDS-Security, and discovery configuration are
// deferred to later phases.
//
// Choose an implementation by importing one of the sub-packages and calling
// its New function:
//
//	import "github.com/SoundMatt/go-DDS/mock"    // in-process, no CGo
//	import "github.com/SoundMatt/go-DDS/cyclone" // CycloneDDS via CGo
//
// Both packages expose a New(Domain) (Participant, error) constructor that
// satisfies this package's Participant interface.
package dds

// Domain is a DDS domain identifier (0–232 inclusive per the DDS spec).
// Participants on the same domain and network segment discover each other
// automatically without a broker.
type Domain int

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
	// Volatile discards samples as soon as they are delivered. Appropriate
	// for live telemetry that has no meaning outside its observation window.
	Volatile DurabilityKind = iota
	// TransientLocal retains the last N samples (per history depth) so that
	// late joiners receive current state on subscription.
	TransientLocal
)

// QoS bundles the policies that govern a single publisher or subscriber
// endpoint. Only the policies in scope for phase 1 are represented here;
// additional policies (deadline, lifespan, partition, etc.) will be added
// as the pure-Go RTPS implementation matures.
type QoS struct {
	Reliability  ReliabilityKind
	Durability   DurabilityKind
	HistoryDepth int // 0 means implementation default (typically 1)
}

// DefaultQoS is BestEffort + Volatile with implementation-default history.
// Appropriate for live vehicle telemetry and VISS request/response traffic.
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

// Sample is a single data sample delivered to a Subscriber. Payload is the
// raw bytes written by the corresponding Publisher; Topic is the DDS topic
// name on which the sample arrived.
type Sample struct {
	Topic   string
	Payload []byte
}

// Participant is the DDS domain participant — the root factory for all DDS
// entities. Create one per process per domain. A Participant is safe for
// concurrent use from multiple goroutines.
type Participant interface {
	// NewPublisher creates a writer for the named topic using the given QoS.
	// The topic is created if it does not already exist in this domain.
	NewPublisher(topic string, qos QoS) (Publisher, error)

	// NewSubscriber creates a reader for the named topic using the given QoS.
	// Samples arrive on the channel returned by Subscriber.C().
	NewSubscriber(topic string, qos QoS) (Subscriber, error)

	// Close releases all DDS resources held by this participant, including
	// all publishers and subscribers it created. Calling Close more than
	// once is a no-op.
	Close() error
}

// Publisher writes samples to a single DDS topic.
// A Publisher is safe for concurrent use from multiple goroutines.
type Publisher interface {
	// Write publishes payload to the topic. The call returns after the
	// sample has been handed to the DDS transport layer; it does not wait
	// for acknowledgement from subscribers (even under Reliable QoS).
	Write(payload []byte) error

	// Close releases the publisher. After Close, Write returns an error.
	Close() error
}

// Subscriber reads samples from a single DDS topic as a Go channel.
// A Subscriber is safe for concurrent use from multiple goroutines.
type Subscriber interface {
	// C returns the channel on which inbound samples are delivered.
	// The channel is closed when Close is called.
	C() <-chan Sample

	// Close stops sample delivery and closes the channel returned by C.
	// Calling Close more than once is a no-op.
	Close() error
}
