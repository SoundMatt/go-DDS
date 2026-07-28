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
	"bytes"
	"context"
	"encoding/gob"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"time"

	relay "github.com/SoundMatt/RELAY"
	"google.golang.org/protobuf/proto"
)

// SpecVersion is the RELAY spec version this package conforms to (§17 req 12).
// It tracks the linked RELAY module's constant so the two never drift; bumping
// the RELAY dependency advances this automatically.
const SpecVersion = relay.SpecVersion

// ── Sentinel errors ───────────────────────────────────────────────────────────
//
// Mandatory sentinels wrap the relay package equivalents so errors.Is works
// across both the dds-specific and relay-level error hierarchy (spec §5.1–§5.2).

// ErrClosed is returned when an operation is attempted on a closed entity.
var ErrClosed = fmt.Errorf("dds: entity is closed: %w", relay.ErrClosed)

// ErrNotConnected is returned before a Participant has joined its domain or
// when socket allocation fails at startup.
var ErrNotConnected = fmt.Errorf("dds: not connected: %w", relay.ErrNotConnected)

// ErrTimeout is returned when an operation does not complete within the
// permitted time (e.g. WriteCtx with an expired context).
var ErrTimeout = fmt.Errorf("dds: timeout: %w", relay.ErrTimeout)

// ErrPayloadTooLarge is returned when Write is called with a payload that
// exceeds the MaxSampleSize set in the publisher's QoS.
var ErrPayloadTooLarge = fmt.Errorf("dds: payload exceeds QoS MaxSampleSize: %w", relay.ErrPayloadTooLarge)

// ErrDomainOutOfRange is returned when a Domain value is outside 0–232
// inclusive (spec §15.2 dds.Domain). It wraps ErrNotConnected so callers that
// already test for a participant start-up failure continue to match.
var ErrDomainOutOfRange = fmt.Errorf("dds: domain out of range [0,232]: %w", ErrNotConnected)

// ErrTopicEmpty is returned when an empty topic string is passed. Wraps
// ErrNotConnected per spec §5.4.
var ErrTopicEmpty = fmt.Errorf("dds: topic name must not be empty: %w", ErrNotConnected)

// ErrQoSMismatch is returned when a publisher and subscriber have incompatible
// QoS policies. Wraps ErrNotConnected per spec §5.4.
var ErrQoSMismatch = fmt.Errorf("dds: QoS incompatibility between publisher and subscriber: %w", ErrNotConnected)

// ErrAccessDenied is returned when a topic ACL forbids creating a publisher or
// subscriber for the requested topic. It is only ever returned when an access
// controller has been configured on the participant.
var ErrAccessDenied = errors.New("dds: access denied by topic ACL")

// ErrDeadlineMissed is returned when a subscriber receives no sample within its
// QoS.Deadline period. Wraps relay.ErrTimeout per spec §5.3.
var ErrDeadlineMissed = fmt.Errorf("dds: deadline missed — no sample within QoS.Deadline period: %w", relay.ErrTimeout)

// ErrSampleRejected is returned when a sample is rejected because resource
// limits are exceeded. Wraps relay.ErrPayloadTooLarge per spec §5.3.
var ErrSampleRejected = fmt.Errorf("dds: sample rejected — resource limits exceeded: %w", relay.ErrPayloadTooLarge)

// ErrResourceLimits is returned when a resource limit is exceeded. Wraps
// ErrPayloadTooLarge per spec §5.4.
var ErrResourceLimits = fmt.Errorf("dds: resource limit exceeded: %w", ErrPayloadTooLarge)

// ErrLoanBuffer is returned when a loaned buffer cannot be obtained (pool
// exhausted or size exceeds pool capacity) or when Commit is called with a
// buffer that was not issued by the same LoaningPublisher. Wraps ErrClosed
// per spec §5.4.
var ErrLoanBuffer = fmt.Errorf("dds: loan buffer unavailable or invalid: %w", ErrClosed)

// ErrLivelinessLost is delivered to a subscriber's LivelinessLostCallback (see
// WithLivelinessLost) when a matched writer configured with
// QoS.LivelinessLeaseDuration stops asserting liveliness within its lease
// period (Milestone 14, "QoS Enforcement — Active Policy"). Wraps
// relay.ErrTimeout per spec §5.3, matching ErrDeadlineMissed's pattern for a
// liveness-timeout condition.
var ErrLivelinessLost = fmt.Errorf("dds: writer liveliness lost: %w", relay.ErrTimeout)

// ErrContentFilterUnsupported is returned by NewFilteredSubscriber when p does
// not implement ContentFilteredSubscriberFactory (Milestone 15,
// "Content-Filtered Topics"). Wraps ErrNotConnected per spec §5.2, matching
// the "capability not available on this backend" pattern used elsewhere.
var ErrContentFilterUnsupported = fmt.Errorf("dds: participant does not support content-filtered subscribers: %w", ErrNotConnected)

// ErrTypeNameUnsupported is returned by NewPublisherWithType/NewSubscriberWithType
// when p does not implement TypedEndpointFactory (Milestone 17, "ROS 2 / rmw
// Compatibility"). Wraps ErrNotConnected per spec §5.2, matching the
// "capability not available on this backend" pattern ErrContentFilterUnsupported
// already established.
var ErrTypeNameUnsupported = fmt.Errorf("dds: participant does not support typed endpoints: %w", ErrNotConnected)

// ── Domain ────────────────────────────────────────────────────────────────────

//fusa:req REQ-PART-001
//fusa:req REQ-PART-006

// Domain is a DDS domain identifier (0–232 inclusive per the DDS spec).
// Participants on the same domain and network segment discover each other
// automatically without a broker.
type Domain int

// ValidateDomain returns ErrDomainOutOfRange if d is outside 0–232 inclusive,
// matching the RELAY dds.Domain canonical range (spec §15.2). It is the single
// source of truth for domain validation across all transports.
func ValidateDomain(d Domain) error {
	if d < 0 || d > 232 {
		return ErrDomainOutOfRange
	}
	return nil
}

// ── QoS ──────────────────────────────────────────────────────────────────────

//fusa:req REQ-QOS-001
//fusa:req REQ-QOS-002
//fusa:req REQ-QOS-003
//fusa:req REQ-QOS-004
//fusa:req REQ-QOS-005
//fusa:req REQ-QOS-006
//fusa:req REQ-QOS-007

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
	Reliability  ReliabilityKind `json:"reliability"`
	Durability   DurabilityKind  `json:"durability"`
	HistoryDepth int             `json:"history_depth"` // 0 means implementation default (typically 1)
	Deadline     time.Duration   `json:"deadline"`      // 0 = disabled

	// TSN v0.5 extensions — only used when a TSN-capable transport is active.

	// TransportPriority sets the network-level priority (maps to VLAN PCP /
	// SO_PRIORITY on Linux). 0 = normal, 1–7 = elevated; 7 is highest.
	TransportPriority int `json:"transport_priority"`
	// LatencyBudget is the acceptable end-to-end delivery latency for this
	// endpoint. 0 = unspecified. Informational in v0.5; future releases may
	// enforce it via qdisc admission control.
	LatencyBudget time.Duration `json:"latency_budget"`
	// Lifespan is the sample time-to-live measured from the write timestamp.
	// Samples older than Lifespan are dropped before delivery. 0 = infinite.
	Lifespan time.Duration `json:"lifespan"`
	// PublishPeriod is the periodic publish rate for TSN streams. 0 = aperiodic.
	// The application is responsible for calling Write at this rate; the value
	// is used by TSN stream reservation and scheduling.
	PublishPeriod time.Duration `json:"publish_period"`
	// MaxSampleSize is the maximum Write payload size in bytes. Write returns
	// ErrPayloadTooLarge if the payload exceeds this limit. 0 = unlimited.
	MaxSampleSize int `json:"max_sample_size"`

	// Milestone 14 "QoS Enforcement — Active Policy" extensions.

	// Liveliness selects how a writer's liveliness is asserted. The default,
	// AutomaticLiveliness, is always satisfied while the owning participant is
	// running (matching pre-v1.0 behaviour); it has no additional effect
	// unless LivelinessLeaseDuration is also set.
	Liveliness LivelinessKind `json:"liveliness"`
	// LivelinessLeaseDuration is the maximum interval between liveliness
	// assertions from a writer before matched subscribers consider it dead and
	// invoke their LivelinessLostCallback (see WithLivelinessLost) with
	// ErrLivelinessLost. 0 disables active liveliness enforcement — the
	// writer's samples and heartbeats still update the passive
	// participant-level lease tracked by WithLivelinessCallback, but no
	// per-writer timeout is enforced.
	LivelinessLeaseDuration time.Duration `json:"liveliness_lease_duration"`

	// Ownership selects whether multiple writers may publish the same topic
	// concurrently (SharedOwnership, the default) or whether only the
	// highest-OwnershipStrength writer's samples are delivered
	// (ExclusiveOwnership).
	Ownership OwnershipKind `json:"ownership"`
	// OwnershipStrength ranks writers when Ownership is ExclusiveOwnership;
	// higher wins. Ties are broken deterministically by GUID so all
	// subscribers converge on the same active writer. Ignored when Ownership
	// is SharedOwnership.
	OwnershipStrength int32 `json:"ownership_strength"`

	// Partition scopes topic matching to a logical namespace within a domain:
	// a publisher and subscriber on the same topic only match when their
	// Partition sets intersect. An empty (or nil) Partition is the default
	// partition — matches only other endpoints that also use the default
	// partition, not arbitrary named partitions (standard DDS semantics).
	Partition []string `json:"partition,omitempty"`

	// MinSeparation is the Time-Based Filter QoS: the subscriber drops any
	// sample from a given writer that arrives less than MinSeparation after
	// the previous delivered sample from that same writer. 0 disables
	// filtering (every sample is delivered, subject to other QoS). Applies at
	// the subscriber; has no effect when set on a publisher.
	MinSeparation time.Duration `json:"min_separation"`
}

// LivelinessKind controls how a writer's liveliness is asserted (DDS
// LIVELINESS QoS policy).
type LivelinessKind int

const (
	// AutomaticLiveliness ties liveliness to the writer's owning participant:
	// as long as the process is running, the writer is considered alive. This
	// is the default and matches pre-v1.0 behaviour.
	AutomaticLiveliness LivelinessKind = iota
	// ManualByTopicLiveliness requires the application to publish (or the
	// implementation to send periodic liveliness assertions on its behalf, as
	// go-DDS's rtps and mock backends do) within LivelinessLeaseDuration, or
	// matched subscribers consider the writer dead.
	ManualByTopicLiveliness
)

// OwnershipKind controls whether multiple writers may concurrently publish a
// topic (DDS OWNERSHIP QoS policy).
type OwnershipKind int

const (
	// SharedOwnership allows every matched writer's samples to be delivered
	// concurrently. This is the default and matches pre-v1.0 behaviour.
	SharedOwnership OwnershipKind = iota
	// ExclusiveOwnership allows only the matched writer with the highest
	// OwnershipStrength to have its samples delivered; all other writers on
	// the topic are silenced until the active writer is closed, evicted, or
	// loses liveliness.
	ExclusiveOwnership
)

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

//fusa:req REQ-SUB-001
//fusa:req REQ-SUB-002
//fusa:req REQ-SUB-003

// Sample is a single data sample delivered to a Subscriber.
// Timestamp is the source time of the write; zero means no timestamp was set
// (INFO_TS was not present in the RTPS message, or the mock transport was used).
type Sample struct {
	Topic          string    `json:"topic"`
	Payload        []byte    `json:"payload"`
	Timestamp      time.Time `json:"timestamp"`
	SequenceNumber uint64    `json:"seq"`         // monotonically increasing per writer; 0 = not set
	WriterGUID     GUID      `json:"writer_guid"` // identity of the publishing endpoint; zero = not set
}

// Sample↔relay.Message conversion (ToMessage/FromMessage) lives in the RELAY
// adapter module — see adapt.go (spec §13.7, §15.7).

// ── BackPressurePolicy ────────────────────────────────────────────────────────

//fusa:req REQ-QOS-005
//fusa:req REQ-QOS-006
//fusa:req REQ-QOS-007

// BackPressurePolicy controls what happens when a subscriber's channel is full.
type BackPressurePolicy int

const (
	// DropNewest silently discards the incoming sample when the channel is full.
	// This is the default policy and matches pre-v0.4 behaviour.
	DropNewest BackPressurePolicy = iota
	// DropOldest evicts the oldest queued sample to make room for the new one.
	DropOldest
	// Block waits until the channel has capacity (may block the writer goroutine).
	Block
)

// ── GUID ──────────────────────────────────────────────────────────────────────

//fusa:req REQ-GUID-001
//fusa:req REQ-GUID-002

// GUID is a globally unique 16-byte DDS participant or endpoint identifier.
// The first 12 bytes are the GuidPrefix; the last 4 bytes are the EntityId.
type GUID [16]byte

// ── Liveliness ────────────────────────────────────────────────────────────────

//fusa:req REQ-DISC-006
//fusa:req REQ-DISC-007

// LivelinessEvent reports whether a remote participant has been discovered or
// lost its lease.
type LivelinessEvent int

const (
	// LivelinessGained fires when a new remote participant is discovered via SPDP.
	LivelinessGained LivelinessEvent = iota
	// LivelinessLost fires when a remote participant's lease has expired.
	LivelinessLost
)

// ── Tracer (OpenTelemetry-compatible interface) ────────────────────────────────

// SpanAttribute is a key/value pair attached to a tracing span.
type SpanAttribute struct {
	Key   string
	Value string
}

// Span is a single tracing span. Call End when the operation is complete.
type Span interface {
	// SetAttribute attaches a key/value attribute to the span.
	SetAttribute(key, value string)
	// End finalises the span and records it to the tracer backend.
	End()
}

// Tracer is satisfied by any OpenTelemetry-compatible tracer implementation.
// go-DDS does not import go.opentelemetry.io/otel; callers bridge their
// tracer to this interface using a thin adapter.
type Tracer interface {
	Start(ctx context.Context, spanName string, attrs ...SpanAttribute) (context.Context, Span)
}

// noopSpan implements Span with zero allocations.
type noopSpan struct{}

func (noopSpan) SetAttribute(_, _ string) {}
func (noopSpan) End()                     {}

// noopTracerImpl is the default Tracer used when no tracer is configured.
type noopTracerImpl struct{}

func (noopTracerImpl) Start(ctx context.Context, _ string, _ ...SpanAttribute) (context.Context, Span) {
	return ctx, noopSpan{}
}

// NoopTracer is the zero-cost tracer used when no OTel backend is configured.
var NoopTracer Tracer = noopTracerImpl{}

// ── SubscriberOption ──────────────────────────────────────────────────────────

//fusa:req REQ-SUB-006
//fusa:req REQ-QOS-005
//fusa:req REQ-QOS-006
//fusa:req REQ-QOS-007

// SubscriberConfig holds per-subscriber options applied at construction time.
// It is exported so that implementation packages (mock, rtps, cyclone) can
// read the resolved configuration without duplicating the option-merge logic.
type SubscriberConfig struct {
	Filter                 func(Sample) bool
	ChannelDepth           int                // 0 = implementation default (64)
	BackPressure           BackPressurePolicy // default: DropNewest
	DeadlineMissedCallback func()             // called when subscriber deadline expires; nil = disabled
	// LivelinessLostCallback is called with the writer's GUID when a matched
	// writer configured with QoS.LivelinessLeaseDuration stops asserting
	// liveliness within its lease period. nil = disabled (default): no
	// per-writer liveliness monitoring is performed. See WithLivelinessLost
	// and ErrLivelinessLost.
	LivelinessLostCallback func(GUID)
}

// SubscriberOption configures a subscriber at creation time.
type SubscriberOption func(*SubscriberConfig)

// WithFilter returns a SubscriberOption that applies fn as a content filter.
// Only samples for which fn returns true are delivered to the subscriber's
// channel; non-matching samples are discarded silently.
func WithFilter(fn func(Sample) bool) SubscriberOption {
	return func(c *SubscriberConfig) { c.Filter = fn }
}

// WithChannelDepth sets the capacity of the subscriber's internal channel.
// A depth of 0 uses the implementation default (typically 64).
func WithChannelDepth(n int) SubscriberOption {
	return func(c *SubscriberConfig) { c.ChannelDepth = n }
}

// WithBackPressure sets the back-pressure policy applied when the subscriber
// channel is full. The default policy is DropNewest.
func WithBackPressure(policy BackPressurePolicy) SubscriberOption {
	return func(c *SubscriberConfig) { c.BackPressure = policy }
}

// WithDeadlineMissed registers fn to be called when the subscriber has not
// received a sample within its QoS.Deadline period. fn must be non-nil.
// Has no effect when QoS.Deadline == 0 on the subscriber.
func WithDeadlineMissed(fn func()) SubscriberOption {
	return func(c *SubscriberConfig) { c.DeadlineMissedCallback = fn }
}

// WithLivelinessLost registers fn to be called with a matched writer's GUID
// when that writer stops asserting liveliness within its
// QoS.LivelinessLeaseDuration (Milestone 14, "QoS Enforcement — Active
// Policy"). fn must be non-nil. Has no effect for writers that do not set
// LivelinessLeaseDuration. See ErrLivelinessLost.
func WithLivelinessLost(fn func(GUID)) SubscriberOption {
	return func(c *SubscriberConfig) { c.LivelinessLostCallback = fn }
}

// ApplySubscriberOpts merges a slice of SubscriberOption into a SubscriberConfig.
func ApplySubscriberOpts(opts []SubscriberOption) SubscriberConfig {
	var c SubscriberConfig
	for _, o := range opts {
		o(&c)
	}
	return c
}

// ChanDepth returns the resolved channel depth: cfg.ChannelDepth if > 0,
// otherwise the provided default.
func (c SubscriberConfig) ChanDepth(defaultDepth int) int {
	if c.ChannelDepth > 0 {
		return c.ChannelDepth
	}
	return defaultDepth
}

// ── Metrics ───────────────────────────────────────────────────────────────────

//fusa:req REQ-MON-001
//fusa:req REQ-MON-002

// Metrics holds cumulative statistics for a participant.
type Metrics struct {
	WriteCount     uint64 `json:"write_count"`
	DeliverCount   uint64 `json:"deliver_count"`
	DropCount      uint64 `json:"drop_count"`
	BytesWritten   uint64 `json:"bytes_written"`
	BytesDelivered uint64 `json:"bytes_delivered"`
	ErrorCount     uint64 `json:"error_count"`
}

// MetricsProvider is implemented by participants that expose runtime statistics.
type MetricsProvider interface {
	Metrics() Metrics
}

// ── Discovery Metrics ─────────────────────────────────────────────────────────

//fusa:req REQ-DISC-012
//fusa:req REQ-DISC-013

// DiscoveryMetrics holds cumulative discovery statistics for a participant.
type DiscoveryMetrics struct {
	AnnouncesSent     uint64 // SPDP announcements sent
	AnnouncesReceived uint64 // SPDP announcements received from remote peers
	PeersKnown        uint64 // current number of known remote participants
	PeerEvictions     uint64 // cumulative peers evicted due to lease expiry
	EndpointMatches   uint64 // cumulative topic endpoint matches (local↔remote)
}

// DiscoveryMetricsProvider is implemented by participants that expose
// discovery-layer statistics.
type DiscoveryMetricsProvider interface {
	DiscoveryMetrics() DiscoveryMetrics
}

// ── Per-Topic Metrics ─────────────────────────────────────────────────────────

//fusa:req REQ-MON-003
//fusa:req REQ-MON-004

// TopicMetrics holds per-topic statistics for a single DDS topic.
type TopicMetrics struct {
	Topic          string
	WriteCount     uint64
	DeliverCount   uint64
	DropCount      uint64
	BytesWritten   uint64
	BytesDelivered uint64
}

// TopicMetricsProvider is implemented by participants that expose per-topic
// statistics. The returned slice contains one entry per observed topic.
type TopicMetricsProvider interface {
	TopicMetrics() []TopicMetrics
}

// ── Health ────────────────────────────────────────────────────────────────────

//fusa:req REQ-MON-005
//fusa:req REQ-MON-006

// HealthStatus is the overall operational status of a participant.
type HealthStatus int

const (
	// HealthOK means the participant is running normally.
	HealthOK HealthStatus = iota
	// HealthDegraded means the participant is running with reduced capability.
	HealthDegraded
	// HealthDown means the participant has been closed or has failed.
	HealthDown
)

// String returns a lowercase, JSON-friendly representation of the status.
func (h HealthStatus) String() string {
	switch h {
	case HealthOK:
		return "ok"
	case HealthDegraded:
		return "degraded"
	default:
		return "down"
	}
}

// Health is a point-in-time health snapshot for a participant.
type Health struct {
	// Status is the overall health classification.
	Status HealthStatus `json:"status"`
	// Details carries an optional human-readable or JSON-encoded string
	// describing per-subsystem state. Empty string means no details.
	Details string `json:"details,omitempty"`
}

// HealthProvider is implemented by participants that expose health reporting.
type HealthProvider interface {
	Health() Health
}

// ── Drainer ───────────────────────────────────────────────────────────────────

// Drainer is optionally implemented by Participants that support graceful
// shutdown: waiting for all in-flight reliable writes to be acknowledged before
// closing. If a Participant does not implement Drainer, CloseWithDrain falls
// back to a plain Close.
type Drainer interface {
	CloseWithDrain(ctx context.Context) error
}

// CloseWithDrain waits for all pending reliable ACKs then closes p.
// If p does not implement Drainer, it calls p.Close() directly.
func CloseWithDrain(ctx context.Context, p Participant) error {
	if d, ok := p.(Drainer); ok {
		return d.CloseWithDrain(ctx)
	}
	return p.Close()
}

// ── PublicAddresser ──────────────────────────────────────────────────────────

// PublicAddresser is optionally implemented by Participants that support
// STUN-based public-address discovery (Milestone 15, "NAT Traversal / Cloud
// Gateway" — e.g. the rtps package's WithSTUNServer). If a Participant does
// not implement PublicAddresser, or discovery failed or was never
// requested, PublicAddr returns "".
type PublicAddresser interface {
	// PublicAddr returns this participant's server-reflexive ("host:port")
	// address as discovered via STUN, or "" if none was discovered.
	PublicAddr() string
}

// PublicAddr returns p's STUN-discovered public address, or "" if p does not
// implement [PublicAddresser] or no address was discovered.
func PublicAddr(p Participant) string {
	if a, ok := p.(PublicAddresser); ok {
		return a.PublicAddr()
	}
	return ""
}

// ── ContentFilteredSubscriberFactory ────────────────────────────────────────

//fusa:req REQ-CFILT-006
//fusa:req REQ-CFILT-007
//fusa:req REQ-CFILT-008

// ContentFilteredSubscriberFactory is optionally implemented by Participants
// that support server-side content-filtered subscriptions (Milestone 15,
// "Content-Filtered Topics"; mock, rtps, and shmem all implement it). Unlike
// WithFilter — whose func(Sample) bool only discards non-matching samples
// after they have already been delivered locally or received over the
// network — a content-filtered subscriber's predicate is propagated to every
// matched publisher and evaluated there before a sample is transmitted, so
// non-matching samples never cross the network in the first place.
type ContentFilteredSubscriberFactory interface {
	// NewFilteredSubscriber creates a subscriber for topic that additionally
	// only receives samples for which expr — a small SQL-like predicate, e.g.
	// "x > 42 AND status = 'active'" (see the cfilter package for the full
	// grammar) — evaluates true against the sample Payload's JSON object
	// fields. params binds %0, %1, ... placeholders in expr. topic may itself
	// be an MQTT-style +/# wildcard pattern, exactly like NewSubscriber.
	NewFilteredSubscriber(topic, expr string, params []string, qos QoS, opts ...SubscriberOption) (Subscriber, error)
}

// NewFilteredSubscriber creates a server-side content-filtered subscriber on
// p (Milestone 15, "Content-Filtered Topics"). It returns
// ErrContentFilterUnsupported if p does not implement
// [ContentFilteredSubscriberFactory], and otherwise returns whatever error
// (e.g. a cfilter.Parse failure) p's own implementation returns.
func NewFilteredSubscriber(p Participant, topic, expr string, params []string, qos QoS, opts ...SubscriberOption) (Subscriber, error) {
	f, ok := p.(ContentFilteredSubscriberFactory)
	if !ok {
		return nil, ErrContentFilterUnsupported
	}
	return f.NewFilteredSubscriber(topic, expr, params, qos, opts...)
}

// ── TypedEndpointFactory ─────────────────────────────────────────────────────

// TypedEndpointFactory is optionally implemented by Participants that can
// carry a real DDS type name across discovery (Milestone 17, "ROS 2 / rmw
// Compatibility"; rtps implements it) instead of the opaque "CDR_BLOB"
// sentinel NewPublisher/NewSubscriber otherwise advertise. Other DDS
// vendors — including every ROS 2 rmw implementation (FastDDS,
// CycloneDDS) — match publications to subscriptions in part on type name,
// so a participant that wants a real ROS 2 node to see it as a compatible
// publisher/subscriber for a given message type must announce that type's
// name, not "CDR_BLOB". See the ros2 package, which uses this to announce
// ROS 2's "pkg::msg::dds_::Type_" type-name convention.
type TypedEndpointFactory interface {
	// NewPublisherWithType behaves like Participant.NewPublisher, except the
	// SEDP/discovery announcement carries typeName instead of the default
	// opaque sentinel.
	NewPublisherWithType(topic, typeName string, qos QoS) (Publisher, error)
	// NewSubscriberWithType behaves like Participant.NewSubscriber, except
	// the SEDP/discovery announcement carries typeName instead of the
	// default opaque sentinel.
	NewSubscriberWithType(topic, typeName string, qos QoS, opts ...SubscriberOption) (Subscriber, error)
}

// NewPublisherWithType creates a publisher on p that announces typeName as
// its DDS type name (Milestone 17, "ROS 2 / rmw Compatibility"). It returns
// ErrTypeNameUnsupported if p does not implement [TypedEndpointFactory].
func NewPublisherWithType(p Participant, topic, typeName string, qos QoS) (Publisher, error) {
	f, ok := p.(TypedEndpointFactory)
	if !ok {
		return nil, ErrTypeNameUnsupported
	}
	return f.NewPublisherWithType(topic, typeName, qos)
}

// NewSubscriberWithType creates a subscriber on p that announces typeName as
// its DDS type name (Milestone 17, "ROS 2 / rmw Compatibility"). It returns
// ErrTypeNameUnsupported if p does not implement [TypedEndpointFactory].
func NewSubscriberWithType(p Participant, topic, typeName string, qos QoS, opts ...SubscriberOption) (Subscriber, error) {
	f, ok := p.(TypedEndpointFactory)
	if !ok {
		return nil, ErrTypeNameUnsupported
	}
	return f.NewSubscriberWithType(topic, typeName, qos, opts...)
}

// ── Interfaces ────────────────────────────────────────────────────────────────

//fusa:req REQ-PART-001
//fusa:req REQ-PART-002
//fusa:req REQ-PART-004
//fusa:req REQ-PART-005
//fusa:req REQ-PART-006
//fusa:req REQ-PART-007
//fusa:req REQ-PART-008
//fusa:req REQ-PUB-001
//fusa:req REQ-PUB-002
//fusa:req REQ-PUB-003
//fusa:req REQ-PUB-004
//fusa:req REQ-PUB-005
//fusa:req REQ-PUB-006
//fusa:req REQ-SUB-001
//fusa:req REQ-SUB-002
//fusa:req REQ-SUB-003
//fusa:req REQ-SUB-004
//fusa:req REQ-SUB-005
//fusa:req REQ-SUB-006
//fusa:req REQ-SUB-007
//fusa:req REQ-CODEC-001
//fusa:req REQ-CODEC-002
//fusa:req REQ-CODEC-003

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

	// Domain returns the DDS domain this participant joined.
	Domain() Domain

	// Close releases all DDS resources held by this participant.
	Close() error
}

// Publisher writes samples to a single DDS topic.
// A Publisher is safe for concurrent use from multiple goroutines.
type Publisher interface {
	Write(payload []byte) error
	// WriteCtx is Write with context cancellation support. If ctx is already
	// done when WriteCtx is called it returns ctx.Err() immediately.
	WriteCtx(ctx context.Context, payload []byte) error
	Close() error
}

// LoaningPublisher extends Publisher with zero-copy loaned-sample support.
// Use Loan to obtain a pre-allocated buffer from the publisher's internal pool,
// write payload data into it, then call Commit to publish and return the buffer.
// Commit calls Write internally; the buffer is returned to the pool afterwards
// regardless of whether Write succeeds.
//
// The loaned buffer must not be used after Commit returns.
type LoaningPublisher interface {
	Publisher
	// Loan returns a pre-allocated byte slice of the given size backed by the
	// publisher's internal pool. Returns ErrLoanBuffer if the pool is exhausted
	// or size exceeds the pool's configured capacity.
	Loan(size int) ([]byte, error)
	// Commit publishes the loaned buffer (by calling Write) and returns it to
	// the pool. buf must be a slice previously returned by Loan on this same
	// publisher. The buffer must not be used after Commit returns.
	Commit(buf []byte) error
}

// Subscriber reads samples from a single DDS topic as a Go channel.
// A Subscriber is safe for concurrent use from multiple goroutines.
type Subscriber interface {
	C() <-chan Sample
	// TryRead attempts a non-blocking read. Returns (zero, false) when the
	// channel is empty or closed.
	TryRead() (Sample, bool)
	// Unsubscribe removes this subscriber from the topic without closing its
	// channel. After Unsubscribe the channel remains open but no new samples
	// are delivered. Call Close to stop delivery AND close the channel.
	// Idempotent; second call is a no-op (spec §6.4).
	Unsubscribe()
	Close() error
}

// ── WaitSet ───────────────────────────────────────────────────────────────────

//fusa:req REQ-RT-002
//fusa:req REQ-RT-003
//fusa:req REQ-RT-004
//fusa:req REQ-RT-005
//fusa:req REQ-REL-004
//fusa:req REQ-SEOOC-006

// WaitSet multiplexes over a dynamic set of Subscribers. Use NewWaitSet to
// construct one and Attach/Detach to modify the set at any time. Wait blocks
// until one of the attached subscribers delivers a sample.
type WaitSet struct {
	mu   sync.RWMutex
	subs []Subscriber
}

// NewWaitSet creates a WaitSet that monitors the given subscribers.
func NewWaitSet(subs ...Subscriber) *WaitSet {
	s := make([]Subscriber, len(subs))
	copy(s, subs)
	return &WaitSet{subs: s}
}

// Attach adds subs to the WaitSet. Changes take effect on the next Wait call;
// an in-progress Wait sees the snapshot taken at its start.
func (ws *WaitSet) Attach(subs ...Subscriber) *WaitSet {
	ws.mu.Lock()
	ws.subs = append(ws.subs, subs...)
	ws.mu.Unlock()
	return ws
}

// Detach removes subs from the WaitSet. Changes take effect on the next Wait
// call; an in-progress Wait sees the snapshot taken at its start.
func (ws *WaitSet) Detach(subs ...Subscriber) *WaitSet {
	ws.mu.Lock()
	for _, target := range subs {
		for i, s := range ws.subs {
			if s == target {
				ws.subs = append(ws.subs[:i], ws.subs[i+1:]...)
				break
			}
		}
	}
	ws.mu.Unlock()
	return ws
}

// Wait blocks until a sample is available on any attached subscriber, or until
// ctx is cancelled. It snapshots the subscriber list at call time; Attach/Detach
// calls from other goroutines take effect on the next Wait invocation.
func (ws *WaitSet) Wait(ctx context.Context) (Sample, Subscriber, error) {
	ws.mu.RLock()
	snapshot := make([]Subscriber, len(ws.subs))
	copy(snapshot, ws.subs)
	ws.mu.RUnlock()

	cases := make([]reflect.SelectCase, 1+len(snapshot))
	cases[0] = reflect.SelectCase{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(ctx.Done())}
	for i, sub := range snapshot {
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
				// All subscriber channels are closed. Prefer context error when
				// both occur simultaneously; otherwise signal channel exhaustion.
				if err := ctx.Err(); err != nil {
					return Sample{}, nil, err
				}
				return Sample{}, nil, ErrClosed
			}
			continue
		}
		s, ok2 := recv.Interface().(Sample)
		if !ok2 {
			continue
		}
		return s, snapshot[chosen-1], nil
	}
}

// ── Typed generics ────────────────────────────────────────────────────────────

// Codec[T] marshals and unmarshals values of type T to/from []byte.
// Implement this interface to bind a schema (JSON, protobuf, msgpack, …) to a
// TypedPublisher or TypedSubscriber.
type Codec[T any] interface {
	Marshal(v T) ([]byte, error)
	Unmarshal(data []byte) (T, error)
}

// TypedSample[T] is a decoded sample delivered by TypedSubscriber[T].
type TypedSample[T any] struct {
	Topic          string
	Value          T
	Timestamp      time.Time
	SequenceNumber uint64
	WriterGUID     GUID
}

// TypedPublisher[T] wraps a Publisher to encode values with a Codec before writing.
type TypedPublisher[T any] struct {
	pub   Publisher
	codec Codec[T]
}

// NewTypedPublisher wraps pub so that Write accepts T values, encoding them
// with codec before passing bytes to the underlying Publisher.
func NewTypedPublisher[T any](pub Publisher, codec Codec[T]) *TypedPublisher[T] {
	return &TypedPublisher[T]{pub: pub, codec: codec}
}

// Write encodes v with the configured Codec and writes it to the underlying publisher.
func (tp *TypedPublisher[T]) Write(v T) error {
	data, err := tp.codec.Marshal(v)
	if err != nil {
		return err
	}
	return tp.pub.Write(data)
}

// WriteCtx encodes v and writes it, honouring ctx cancellation.
func (tp *TypedPublisher[T]) WriteCtx(ctx context.Context, v T) error {
	data, err := tp.codec.Marshal(v)
	if err != nil {
		return err
	}
	return tp.pub.WriteCtx(ctx, data)
}

// Close closes the underlying publisher.
func (tp *TypedPublisher[T]) Close() error { return tp.pub.Close() }

// TypedSubscriber[T] wraps a Subscriber to decode samples with a Codec.
// Samples that fail to decode are silently dropped.
type TypedSubscriber[T any] struct {
	sub   Subscriber
	codec Codec[T]
	ch    chan TypedSample[T]
	done  chan struct{}
	once  sync.Once
}

// NewTypedSubscriber wraps sub so that its channel delivers TypedSample[T]
// values decoded with codec. A background goroutine pumps samples from sub.C().
func NewTypedSubscriber[T any](sub Subscriber, codec Codec[T]) *TypedSubscriber[T] {
	ts := &TypedSubscriber[T]{
		sub:   sub,
		codec: codec,
		ch:    make(chan TypedSample[T], 64),
		done:  make(chan struct{}),
	}
	go ts.pump()
	return ts
}

func (ts *TypedSubscriber[T]) pump() {
	defer close(ts.ch)
	for {
		select {
		case s, ok := <-ts.sub.C():
			if !ok {
				return
			}
			v, err := ts.codec.Unmarshal(s.Payload)
			if err != nil {
				continue // decode error: drop this sample
			}
			select {
			case ts.ch <- TypedSample[T]{Topic: s.Topic, Value: v, Timestamp: s.Timestamp, SequenceNumber: s.SequenceNumber, WriterGUID: s.WriterGUID}:
			case <-ts.done:
				return
			}
		case <-ts.done:
			return
		}
	}
}

// C returns the typed sample channel.
func (ts *TypedSubscriber[T]) C() <-chan TypedSample[T] { return ts.ch }

// Close stops the pump goroutine and closes the underlying subscriber.
func (ts *TypedSubscriber[T]) Close() error {
	ts.once.Do(func() { close(ts.done) })
	return ts.sub.Close()
}

// ── JSONCodec ─────────────────────────────────────────────────────────────────

// JSONCodec[T] implements Codec[T] using encoding/json.
// It is a zero-size struct; the zero value is ready to use.
type JSONCodec[T any] struct{}

// Marshal encodes v to JSON bytes.
func (JSONCodec[T]) Marshal(v T) ([]byte, error) { return json.Marshal(v) }

// Unmarshal decodes data from JSON into T.
func (JSONCodec[T]) Unmarshal(data []byte) (T, error) {
	var v T
	return v, json.Unmarshal(data, &v)
}

// ── GobCodec ──────────────────────────────────────────────────────────────────

// GobCodec[T] implements Codec[T] using encoding/gob.
// It is a zero-size struct; the zero value is ready to use.
// Suitable for concrete struct types exchanged within a single Go binary.
type GobCodec[T any] struct{}

// Marshal encodes v using gob encoding.
func (GobCodec[T]) Marshal(v T) ([]byte, error) {
	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(v); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// Unmarshal decodes data from gob encoding into T.
func (GobCodec[T]) Unmarshal(data []byte) (T, error) {
	var v T
	return v, gob.NewDecoder(bytes.NewReader(data)).Decode(&v)
}

// ── ProtoCodec ────────────────────────────────────────────────────────────────

// ProtoCodec[T] implements Codec[T] using google.golang.org/protobuf.
// T must be a pointer to a protobuf-generated message struct (e.g. *mypkg.Msg).
// The zero value is ready to use.
type ProtoCodec[T proto.Message] struct{}

// Marshal encodes v to protobuf wire format.
func (ProtoCodec[T]) Marshal(v T) ([]byte, error) {
	return proto.Marshal(v)
}

// Unmarshal decodes data from protobuf wire format into a new T.
func (ProtoCodec[T]) Unmarshal(data []byte) (T, error) {
	var zero T
	msg, ok := reflect.New(reflect.TypeOf(zero).Elem()).Interface().(T)
	if !ok {
		return zero, fmt.Errorf("dds: ProtoCodec[%T]: type assertion failed", zero)
	}
	return msg, proto.Unmarshal(data, msg)
}
