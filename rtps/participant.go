// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Package rtps provides a pure-Go RTPS/UDP implementation of the dds
// interfaces. It requires no CGo and no installed system libraries; it speaks
// the OMG RTPS 2.3 wire protocol over UDP multicast/unicast so it can
// interoperate with any standards-compliant DDS implementation.
//
// Create a participant with rtps.New. For reliable delivery, pass
// dds.ReliableQoS to NewPublisher/NewSubscriber. For payload-level security,
// pass rtps.WithSecurity to rtps.New:
//
//	p, err := rtps.New(dds.Domain(0), rtps.WithSecurity(myPlugin))
package rtps

//fusa:req REQ-RT-001
//fusa:req REQ-REL-004
//fusa:req REQ-PART-002
//fusa:req REQ-PART-004
//fusa:req REQ-PART-005
//fusa:req REQ-PART-006
//fusa:req REQ-PART-007
//fusa:req REQ-PART-008
//fusa:req REQ-PART-009
//fusa:req REQ-PART-010
//fusa:req REQ-PART-011
//fusa:req REQ-PUB-001
//fusa:req REQ-PUB-002
//fusa:req REQ-PUB-003
//fusa:req REQ-PUB-004
//fusa:req REQ-PUB-005
//fusa:req REQ-PUB-006
//fusa:req REQ-PUB-007
//fusa:req REQ-SUB-001
//fusa:req REQ-SUB-002
//fusa:req REQ-SUB-003
//fusa:req REQ-SUB-004
//fusa:req REQ-SUB-005
//fusa:req REQ-SUB-006
//fusa:req REQ-QOS-001
//fusa:req REQ-QOS-002
//fusa:req REQ-QOS-003
//fusa:req REQ-QOS-004
//fusa:req REQ-GUID-001
//fusa:req REQ-GUID-002
//fusa:req REQ-SEOOC-001
//fusa:req REQ-SEOOC-004
//fusa:req REQ-SEOOC-005
//fusa:req REQ-SEOOC-010

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"reflect"
	"sync"
	"sync/atomic"
	"time"

	dds "github.com/SoundMatt/go-DDS"
	"github.com/SoundMatt/go-DDS/config"
	"github.com/SoundMatt/go-DDS/tsn"
)

// ── Per-topic metrics ─────────────────────────────────────────────────────────

// topicCounter accumulates per-topic write, deliver, and drop statistics.
// All fields are incremented atomically; no lock is required.
type topicCounter struct {
	writes   atomic.Uint64
	delivers atomic.Uint64
	drops    atomic.Uint64
	bytesW   atomic.Uint64
	bytesD   atomic.Uint64
}

// ── Options ───────────────────────────────────────────────────────────────────

// SecurityPlugin is satisfied by any type that can seal (encrypt / sign) and
// open (decrypt / verify) a DDS payload byte slice. Built-in implementations
// live in the security sub-package; NullPlugin (pass-through) is also
// available there for development and testing.
type SecurityPlugin interface {
	Seal(plaintext []byte) ([]byte, error)
	Open(ciphertext []byte) ([]byte, error)
}

// DiscoveryPlugin authenticates SPDP participant-discovery announcements.
// When configured via [WithDiscoverySecurity], outbound announcements are
// tagged with a token produced by SignDiscovery, and inbound announcements
// whose token does not verify are silently discarded.
//
// The built-in implementation is security.HMACDiscoveryPlugin.
type DiscoveryPlugin interface {
	// SignDiscovery returns an authentication tag for the given GUID prefix
	// (12 bytes). The tag is embedded in the SPDP announcement.
	SignDiscovery(guidPrefix []byte) []byte
	// VerifyDiscovery returns true when tag is a valid authentication tag for
	// guidPrefix. A nil or empty tag must return false.
	VerifyDiscovery(guidPrefix, tag []byte) bool
}

// EndpointPlugin optionally extends a DiscoveryPlugin to authenticate SEDP
// endpoint announcements. Participants that implement this interface sign
// outbound endpoint announcements and reject inbound announcements whose tag
// does not verify. The built-in implementation is security.HMACDiscoveryPlugin.
type EndpointPlugin interface {
	// SignEndpoint returns a tag for the endpoint identified by guidPrefix and
	// topicName. The tag is embedded in the SEDP announcement.
	SignEndpoint(guidPrefix []byte, topic string) []byte
	// VerifyEndpoint returns true when tag is a valid authentication tag for
	// the given guidPrefix and topicName. A nil or empty tag must return false.
	VerifyEndpoint(guidPrefix []byte, topic string, tag []byte) bool
}

// Option configures a Participant at creation time.
type Option func(*participant)

// WithSecurity returns an Option that applies plugin to every payload transmitted
// and received by this participant. All peers that communicate with this
// participant must use the same plugin and key material.
// WithDiscoverySecurity returns an Option that applies plugin to SPDP
// discovery announcements. Outbound announcements are signed; inbound
// announcements with missing or invalid tokens are rejected.
func WithDiscoverySecurity(plugin DiscoveryPlugin) Option {
	return func(p *participant) { p.discoveryPlugin = plugin }
}

func WithSecurity(plugin SecurityPlugin) Option {
	return func(p *participant) { p.security = plugin }
}

// AccessController authorises endpoint creation per topic. security.AccessPolicy
// satisfies it. When configured via [WithAccessControl], NewPublisher is rejected
// for topics that fail CanWrite and NewSubscriber for topics that fail CanRead.
type AccessController interface {
	CanRead(topic string) bool
	CanWrite(topic string) bool
}

// ReplayChecker rejects replayed samples. security.ReplayGuard satisfies it.
// When configured via [WithAntiReplay], every inbound DATA sample is checked and
// dropped (not delivered) if Check reports it as a replay.
type ReplayChecker interface {
	Check(seq uint64, ts time.Time) error
}

// WithAccessControl enforces a topic ACL on this participant. Enforcement is
// opt-in: with no controller configured, all topics are permitted. It composes
// with WithSecurity (encryption) and WithAntiReplay.
func WithAccessControl(ac AccessController) Option {
	return func(p *participant) { p.accessControl = ac }
}

// WithAntiReplay enables anti-replay protection on inbound samples. Enforcement
// is opt-in: with no checker configured, no samples are dropped. It composes
// with WithSecurity (encryption) and WithAccessControl.
func WithAntiReplay(rc ReplayChecker) Option {
	return func(p *participant) { p.antiReplay = rc }
}

// WithContext returns an Option that closes the participant when ctx is done.
// This is the idiomatic Go shutdown pattern: pass a context with a cancel
// function or deadline to tie the participant's lifetime to an outer scope.
func WithContext(ctx context.Context) Option {
	return func(p *participant) { p.cancelCtx = ctx }
}

// WithIPv6 enables the IPv6 multicast transport. When set, the participant
// binds an additional pair of IPv6 UDP sockets and joins the RTPS IPv6
// discovery group (FF03::1). IPv4 sockets are still created so the participant
// is reachable from both IPv4 and IPv6 peers.
func WithIPv6() Option {
	return func(p *participant) { p.ipv6 = true }
}

// WithNoMulticast disables SPDP multicast discovery. The participant will not
// join or send to 239.255.0.1; use WithPeerLocators to supply peers manually.
func WithNoMulticast() Option {
	return func(p *participant) { p.noMulticast = true }
}

// WithPeerLocators adds static peer unicast addresses for unicast-only
// discovery. Each address must be a host:port string parseable by net.ResolveUDPAddr.
func WithPeerLocators(addrs ...string) Option {
	return func(p *participant) { p.peerLocators = append(p.peerLocators, addrs...) }
}

// WithDeadlineCallback sets a function that is called when a publisher has not
// written for longer than its QoS.Deadline period.
func WithDeadlineCallback(fn func(topic string)) Option {
	return func(p *participant) { p.deadlineCb = fn }
}

// WithLogger sets the structured logger used by this participant.
// Passing nil (the default) disables all log output with zero overhead.
func WithLogger(l *slog.Logger) Option {
	return func(p *participant) { p.log = plog{l} }
}

// WithLivelinessCallback registers fn to be called when a remote participant is
// discovered (LivelinessGained) or loses its lease (LivelinessLost).
// The GUID passed is the 16-byte participant GUID (prefix + built-in entity 0x000001c1).
func WithLivelinessCallback(fn func(dds.GUID, dds.LivelinessEvent)) Option {
	return func(p *participant) { p.livelinessCb = fn }
}

// WithTracer wires an OpenTelemetry-compatible Tracer into the participant.
// Pass dds.NoopTracer (the default) to disable tracing with zero cost.
func WithTracer(t dds.Tracer) Option {
	return func(p *participant) { p.tracer = t }
}

// WithSPDPInterval sets the SPDP participant announcement interval.
// The default is 2 seconds. TSN networks with bounded-latency requirements
// may prefer longer intervals (e.g. 10 s) to reduce discovery overhead.
func WithSPDPInterval(d time.Duration) Option {
	return func(p *participant) { p.spdpInterval = d }
}

// WithSPDPJitter adds a random delay of up to d before each SPDP announcement.
// This prevents synchronised floods when many participants start simultaneously
// on a TSN segment. A typical value is 500 ms.
func WithSPDPJitter(d time.Duration) Option {
	return func(p *participant) { p.spdpJitter = d }
}

// WithStaticPeers adds static peer unicast addresses for unicast-only
// discovery on TSN networks where SPDP multicast is undesirable.
// Equivalent to WithPeerLocators; provided for TSN configuration clarity.
func WithStaticPeers(addrs ...string) Option {
	return WithPeerLocators(addrs...)
}

// WithHeartbeatPeriod sets the period of the periodic HEARTBEAT ticker used by
// reliable writers. The default is 200 ms. Use shorter values for
// low-latency reliable delivery; use longer values to reduce control traffic.
func WithHeartbeatPeriod(d time.Duration) Option {
	return func(p *participant) { p.heartbeatPeriodOverride = d }
}

// WithConfig applies all fields from cfg to the participant. It is equivalent
// to calling the corresponding WithXxx options individually and is intended for
// use with JSON configuration files loaded via [config.LoadConfig].
func WithConfig(cfg *config.ParticipantConfig) Option {
	return func(p *participant) {
		if cfg.HeartbeatPeriodDur > 0 {
			p.heartbeatPeriodOverride = cfg.HeartbeatPeriodDur
		}
		if cfg.SPDPIntervalDur > 0 {
			p.spdpInterval = cfg.SPDPIntervalDur
		}
		if cfg.SPDPJitterDur > 0 {
			p.spdpJitter = cfg.SPDPJitterDur
		}
		if cfg.NoMulticast {
			p.noMulticast = true
		}
		p.peerLocators = append(p.peerLocators, cfg.PeerLocators...)
	}
}

// WithTSNConfig registers a TSN stream configuration with the participant.
// When a publisher is created for a topic in the config, the participant
// allocates a dedicated socket for that traffic class, marks it with
// SO_PRIORITY / IP_TOS, and (on Linux) enables SO_TXTIME if TxOffsetUS > 0.
func WithTSNConfig(cfg *tsn.StreamConfig) Option {
	return func(p *participant) { p.tsnConfig = cfg }
}

// ── Participant ───────────────────────────────────────────────────────────────

// participant implements dds.Participant over real RTPS/UDP.
type participant struct {
	domain     dds.Domain
	guidPrefix GuidPrefix

	// Flags set by options.
	ipv6         bool
	noMulticast  bool
	peerLocators []string
	deadlineCb   func(string)
	persistDir   string
	log          plog
	livelinessCb func(dds.GUID, dds.LivelinessEvent)
	tracer       dds.Tracer
	cancelCtx    context.Context

	// Configurable heartbeat period (0 = use package-level heartbeatPeriod constant).
	heartbeatPeriodOverride time.Duration

	// TSN options.
	tsnConfig    *tsn.StreamConfig
	spdpInterval time.Duration        // 0 = use spdpAnnouncePeriod (2 s)
	spdpJitter   time.Duration        // 0 = no jitter
	tsnSocks     map[uint8]*udpSocket // per-PCP traffic-class sockets; keyed by PCP (0–7)
	tsnMu        sync.Mutex

	// Sockets (IPv4).
	mcastSock     *udpSocket // SPDP multicast receive
	metaSock      *udpSocket // SPDP send + SEDP send/receive (unicast)
	dataSock      *udpSocket // User DATA / HEARTBEAT / ACKNACK (unicast)
	dataMcastSock *udpSocket // User DATA multicast receive (nil when noMulticast)

	// Sockets (IPv6, non-nil only when ipv6 == true).
	mcastSockV6 *udpSocket // SPDP IPv6 multicast receive
	metaSockV6  *udpSocket // SEDP IPv6 meta unicast
	dataSockV6  *udpSocket // User data IPv6 unicast

	// Discovery services.
	spdp *spdpService
	sedp *sedpService

	// Optional security plugin (nil = no security).
	security SecurityPlugin
	// Optional topic ACL (nil = all topics permitted).
	accessControl AccessController
	// Optional anti-replay checker (nil = no replay filtering).
	antiReplay ReplayChecker
	// Optional discovery security plugin (nil = unauthenticated discovery).
	discoveryPlugin DiscoveryPlugin

	// Endpoint registry.
	mu             sync.Mutex
	closed         bool
	writers        map[EntityId]*rtpsWriter
	readers        map[EntityId]*rtpsReader
	writerLocators map[GUID]Locator
	entityCounter  uint32
	// TransientLocal last-value cache: topic (string) → *dds.Sample.
	// Uses sync.Map to avoid lock-ordering issues: Write holds w.mu and must
	// not also acquire p.mu (which Close holds while iterating writers).
	lastSample sync.Map

	// Participant-level metrics counters — incremented atomically, never need p.mu.
	mWrites       atomic.Uint64
	mDelivers     atomic.Uint64
	mDrops        atomic.Uint64
	mBytesWritten atomic.Uint64
	mBytesDeliv   atomic.Uint64

	// Per-topic metrics: topic string → *topicCounter (sync.Map, no lock needed).
	topicMetrics sync.Map
}

// effectiveHeartbeatPeriod returns the heartbeat period configured by
// WithHeartbeatPeriod (or WithConfig), falling back to the package constant.
func (p *participant) effectiveHeartbeatPeriod() time.Duration {
	if p.heartbeatPeriodOverride > 0 {
		return p.heartbeatPeriodOverride
	}
	return heartbeatPeriod
}

// topicCounterFor returns (creating on first access) the per-topic counter for
// the given topic name. The returned pointer is safe to use concurrently.
func (p *participant) topicCounterFor(topic string) *topicCounter {
	if v, ok := p.topicMetrics.Load(topic); ok {
		if tc, ok2 := v.(*topicCounter); ok2 {
			return tc
		}
	}
	tc := &topicCounter{}
	actual, loaded := p.topicMetrics.LoadOrStore(topic, tc)
	_ = loaded
	if tc2, ok := actual.(*topicCounter); ok {
		return tc2
	}
	return tc
}

// Domain implements dds.Participant.
func (p *participant) Domain() dds.Domain { return p.domain }

// New creates an RTPS participant joined to the given DDS domain.
// It binds UDP sockets, starts SPDP/SEDP, and returns a dds.Participant.
func New(domain dds.Domain, opts ...Option) (dds.Participant, error) {
	return newParticipant(domain, opts...)
}

func newParticipant(domain dds.Domain, opts ...Option) (*participant, error) {
	if err := dds.ValidateDomain(domain); err != nil {
		return nil, fmt.Errorf("rtps: domain %d: %w", domain, err)
	}
	d := int(domain)
	guidPrefix := newGuidPrefix()

	// Allocate ports — try participant index 0..15.
	var metaSock, dataSock *udpSocket
	var participantIdx int
	for i := 0; i < 16; i++ {
		var err error
		metaSock, err = newUnicastSocket(metaUnicastPort(d, i))
		if err != nil {
			continue
		}
		dataSock, err = newUnicastSocket(userUnicastPort(d, i))
		if err != nil {
			metaSock.close()
			continue
		}
		participantIdx = i
		break
	}
	if metaSock == nil {
		return nil, fmt.Errorf("rtps: no free port pair for domain %d", domain)
	}
	_ = participantIdx

	mcastSock, err := newMulticastReceiveSocket(spdpMulticastAddr, metaMulticastPort(d))
	if err != nil {
		metaSock.close()
		dataSock.close()
		return nil, fmt.Errorf("rtps: SPDP multicast: %w", err)
	}

	p := &participant{
		domain:         domain,
		guidPrefix:     guidPrefix,
		mcastSock:      mcastSock,
		metaSock:       metaSock,
		dataSock:       dataSock,
		writers:        make(map[EntityId]*rtpsWriter),
		readers:        make(map[EntityId]*rtpsReader),
		writerLocators: make(map[GUID]Locator),
	}
	for _, o := range opts {
		o(p)
	}

	// Optional IPv6 sockets. Failures are soft: if the OS has no IPv6 support
	// the participant continues with IPv4 only.
	if p.ipv6 {
		if mcastV6, err := newMulticastReceiveSocketV6(spdpMulticastAddrV6, metaMulticastPort(d)); err == nil {
			p.mcastSockV6 = mcastV6
		}
		if metaV6, err := newUnicastSocketV6(metaUnicastPort(d, participantIdx)); err == nil {
			p.metaSockV6 = metaV6
		}
		if dataV6, err := newUnicastSocketV6(userUnicastPort(d, participantIdx)); err == nil {
			p.dataSockV6 = dataV6
		}
	}

	// Default tracer: no-op (zero cost).
	if p.tracer == nil {
		p.tracer = dds.NoopTracer
	}

	// Optional user-data multicast socket (for one-packet-per-write delivery).
	if !p.noMulticast {
		if dmSock, err2 := newMulticastReceiveSocket(userDataMulticastAddr, userMulticastPort(d)); err2 == nil {
			p.dataMcastSock = dmSock
		}
		// Failure is soft: fall back to unicast-only delivery.
	}

	p.log.info("rtps participant starting domain=%d prefix=%x", domain, guidPrefix)

	p.spdp = newSPDPService(p)
	p.sedp = newSEDPService(p)
	p.spdp.start()
	p.sedp.start()

	go p.dataReceiveLoop()

	if p.cancelCtx != nil {
		done := p.cancelCtx.Done()
		go func(done <-chan struct{}) {
			<-done
			_ = p.Close()
		}(done)
	}

	return p, nil
}

func (p *participant) NewPublisher(topic string, qos dds.QoS) (dds.Publisher, error) {
	if topic == "" {
		return nil, fmt.Errorf("rtps: %w", dds.ErrTopicEmpty)
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return nil, fmt.Errorf("rtps: %w", dds.ErrClosed)
	}
	if p.accessControl != nil && !p.accessControl.CanWrite(topic) {
		return nil, fmt.Errorf("rtps: publish %q: %w", topic, dds.ErrAccessDenied)
	}
	n := atomic.AddUint32(&p.entityCounter, 1)
	eid := entityIdForWriter(n)
	w := &rtpsWriter{
		p:        p,
		topic:    topic,
		eid:      eid,
		qos:      qos,
		reliable: qos.Reliability == dds.Reliable,
		hbPeriod: p.effectiveHeartbeatPeriod(),
	}
	if w.reliable {
		w.history = newSendHistory()
		w.hbDone = make(chan struct{})
		w.drainCh = make(chan struct{})
		// Pass the channel by value so heartbeatLoop never reads w.hbDone
		// after the goroutine starts — Close() can then safely nil the field
		// under w.mu without racing with the goroutine.
		go w.heartbeatLoop(w.hbDone)
	}
	if qos.Deadline > 0 && p.deadlineCb != nil {
		w.deadlineTimer = time.AfterFunc(qos.Deadline, func() { p.deadlineCb(topic) })
	}
	// Wire TSN stream: config match takes priority, TransportPriority QoS
	// field acts as a fallback PCP selector when no config entry exists.
	if stream := p.tsnConfig.StreamForTopic(topic); stream != nil {
		w.tsnStream = stream
		w.tsnSock = p.tsnSocketForPCP(stream.PCP, stream.DSCP, stream.TxOffsetUS > 0)
	} else if qos.TransportPriority > 0 {
		pcp := uint8(qos.TransportPriority)
		if pcp > 7 {
			pcp = 7
		}
		w.tsnSock = p.tsnSocketForPCP(pcp, 0, false)
	}
	p.writers[eid] = w
	p.sedp.registerWriter(eid, topic)
	p.log.debug("new publisher topic=%s reliable=%v tsn=%v", topic, w.reliable, w.tsnSock != nil)
	return w, nil
}

func (p *participant) NewSubscriber(topic string, qos dds.QoS, opts ...dds.SubscriberOption) (dds.Subscriber, error) {
	if topic == "" {
		return nil, fmt.Errorf("rtps: %w", dds.ErrTopicEmpty)
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return nil, fmt.Errorf("rtps: %w", dds.ErrClosed)
	}
	if p.accessControl != nil && !p.accessControl.CanRead(topic) {
		return nil, fmt.Errorf("rtps: subscribe %q: %w", topic, dds.ErrAccessDenied)
	}
	cfg := dds.ApplySubscriberOpts(opts)
	depth := cfg.ChanDepth(64)
	n := atomic.AddUint32(&p.entityCounter, 1)
	eid := entityIdForReader(n)
	r := &rtpsReader{
		p:            p,
		topic:        topic,
		eid:          eid,
		ch:           make(chan dds.Sample, depth),
		reliable:     qos.Reliability == dds.Reliable,
		filter:       cfg.Filter,
		backPressure: cfg.BackPressure,
	}
	if qos.Deadline > 0 && cfg.DeadlineMissedCallback != nil {
		fn := cfg.DeadlineMissedCallback
		dur := qos.Deadline
		var tp atomic.Pointer[time.Timer]
		tp.Store(time.AfterFunc(dur, func() {
			fn()
			tp.Load().Reset(dur)
		}))
		r.deadlineTimer = tp.Load()
		r.resetDeadline = func() { tp.Load().Reset(dur) }
	}
	p.log.debug("new subscriber topic=%s depth=%d backpressure=%d", topic, depth, cfg.BackPressure)
	p.readers[eid] = r
	p.sedp.registerReader(eid, topic, r)
	// TransientLocal: deliver the last published sample to the new subscriber.
	// Also check disk-backed persistent history if no in-memory sample exists.
	if qos.Durability == dds.TransientLocal {
		if v, ok := p.lastSample.Load(topic); ok {
			if last, ok2 := v.(*dds.Sample); ok2 {
				if cfg.Filter == nil || cfg.Filter(*last) {
					select {
					case r.ch <- *last:
					default:
					}
				}
			}
		} else if p.persistDir != "" {
			if payload, err := persistLoad(p.persistDir, topic); err == nil && payload != nil {
				sample := dds.Sample{Topic: topic, Payload: payload}
				p.lastSample.Store(topic, &sample)
				if cfg.Filter == nil || cfg.Filter(sample) {
					select {
					case r.ch <- sample:
					default:
					}
				}
			}
		}
	}
	return r, nil
}

// Metrics implements dds.MetricsProvider.
func (p *participant) Metrics() dds.Metrics {
	return dds.Metrics{
		WriteCount:     p.mWrites.Load(),
		DeliverCount:   p.mDelivers.Load(),
		DropCount:      p.mDrops.Load(),
		BytesWritten:   p.mBytesWritten.Load(),
		BytesDelivered: p.mBytesDeliv.Load(),
	}
}

// DiscoveryMetrics implements dds.DiscoveryMetricsProvider.
func (p *participant) DiscoveryMetrics() dds.DiscoveryMetrics {
	p.spdp.mu.RLock()
	peers := uint64(len(p.spdp.peers))
	p.spdp.mu.RUnlock()
	return dds.DiscoveryMetrics{
		AnnouncesSent:     p.spdp.announcesSent.Load(),
		AnnouncesReceived: p.spdp.announcesReceived.Load(),
		PeersKnown:        peers,
		PeerEvictions:     p.spdp.peerEvictions.Load(),
		EndpointMatches:   p.sedp.endpointMatches.Load(),
	}
}

// TopicMetrics implements dds.TopicMetricsProvider.
func (p *participant) TopicMetrics() []dds.TopicMetrics {
	var result []dds.TopicMetrics
	p.topicMetrics.Range(func(k, v any) bool {
		topic, ok := k.(string)
		if !ok {
			return true
		}
		tc, ok2 := v.(*topicCounter)
		if !ok2 {
			return true
		}
		result = append(result, dds.TopicMetrics{
			Topic:          topic,
			WriteCount:     tc.writes.Load(),
			DeliverCount:   tc.delivers.Load(),
			DropCount:      tc.drops.Load(),
			BytesWritten:   tc.bytesW.Load(),
			BytesDelivered: tc.bytesD.Load(),
		})
		return true
	})
	return result
}

// Health implements dds.HealthProvider.
func (p *participant) Health() dds.Health {
	p.mu.Lock()
	closed := p.closed
	writers := len(p.writers)
	readers := len(p.readers)
	p.mu.Unlock()

	if closed {
		return dds.Health{Status: dds.HealthDown, Details: `{"state":"closed"}`}
	}
	return dds.Health{
		Status:  dds.HealthOK,
		Details: fmt.Sprintf(`{"writers":%d,"readers":%d}`, writers, readers),
	}
}

func (p *participant) Close() error {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil
	}
	p.closed = true
	// Snapshot writers so we can call w.Close() without holding p.mu.
	// Write holds w.mu while calling dispatchToReaders (which acquires p.mu);
	// holding p.mu here while calling w.Close() (which acquires w.mu) would
	// invert that order and deadlock. p.closed=true prevents any new writers
	// from being registered after we release the lock.
	ws := make([]*rtpsWriter, 0, len(p.writers))
	for _, w := range p.writers {
		ws = append(ws, w)
	}
	p.mu.Unlock()

	for _, w := range ws {
		_ = w.Close()
	}
	p.spdp.close()
	p.sedp.close()
	p.mcastSock.close()
	p.metaSock.close()
	p.dataSock.close()
	if p.dataMcastSock != nil {
		p.dataMcastSock.close()
	}
	if p.mcastSockV6 != nil {
		p.mcastSockV6.close()
	}
	if p.metaSockV6 != nil {
		p.metaSockV6.close()
	}
	if p.dataSockV6 != nil {
		p.dataSockV6.close()
	}
	// Close any TSN traffic-class sockets.
	p.tsnMu.Lock()
	for _, sock := range p.tsnSocks {
		sock.close()
	}
	p.tsnMu.Unlock()
	return nil
}

// CloseWithDrain implements dds.Drainer. It waits until all reliable writers
// have received ACKNACK confirmation from remote readers (or ctx is cancelled),
// then calls Close. Unreliable writers drain immediately.
func (p *participant) CloseWithDrain(ctx context.Context) error {
	p.mu.Lock()
	ws := make([]*rtpsWriter, 0, len(p.writers))
	for _, w := range p.writers {
		if w.reliable {
			ws = append(ws, w)
		}
	}
	p.mu.Unlock()

	for _, w := range ws {
		if err := w.waitDrain(ctx); err != nil {
			_ = p.Close()
			return err
		}
	}
	return p.Close()
}

// ── Receive loop ──────────────────────────────────────────────────────────────

func (p *participant) dataReceiveLoop() {
	// Collect all active data receive channels.
	chans := []<-chan udpPacket{p.dataSock.recv}
	if p.dataSockV6 != nil {
		chans = append(chans, p.dataSockV6.recv)
	}
	if p.dataMcastSock != nil {
		chans = append(chans, p.dataMcastSock.recv)
	}

	if len(chans) == 1 {
		for pkt := range chans[0] {
			p.handleDataPacket(pkt.data, pkt.from)
		}
		return
	}
	// Fan-in from all sockets using reflect.Select so the count can vary.
	cases := make([]reflect.SelectCase, len(chans))
	for i, ch := range chans {
		cases[i] = reflect.SelectCase{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(ch)}
	}
	active := len(cases)
	for active > 0 {
		chosen, recv, ok := reflect.Select(cases)
		if !ok {
			cases[chosen] = reflect.SelectCase{Dir: reflect.SelectDefault}
			active--
			continue
		}
		pkt, valid := recv.Interface().(udpPacket)
		if !valid {
			continue
		}
		p.handleDataPacket(pkt.data, pkt.from)
	}
}

func (p *participant) handleDataPacket(data []byte, from *net.UDPAddr) {
	hdr, ok := parseHeader(data)
	if !ok {
		return
	}
	// pendingTS carries the most-recently parsed INFO_TS timestamp within this
	// message so it can be attached to the following DATA submessage.
	var pendingTS time.Time
	_ = parseSubmessages(data[20:], func(id, _ byte, body []byte) error {
		switch id {
		case submsgINFO_TS:
			if ts, ok2 := parseInfoTS(body); ok2 {
				pendingTS = ts
			}
		case submsgDATA:
			ds, ok := parseDataSubmessage(flagEndianness|flagData, body)
			if !ok || ds.Payload == nil {
				return nil
			}
			rawPayload, ok := cdrUnwrapPayload(ds.Payload)
			if !ok {
				return nil
			}
			if p.security != nil {
				opened, err := p.security.Open(rawPayload)
				if err != nil {
					return nil // drop; tampered or wrong key
				}
				rawPayload = opened
			}
			if p.antiReplay != nil {
				if err := p.antiReplay.Check(snToU64(ds.SeqNum), pendingTS); err != nil {
					return nil // drop; replayed or duplicate sequence number
				}
			}
			sourceGUID := GUID{Prefix: hdr.GuidPrefix, Entity: ds.WriterEntityId}
			p.notifyReliableReaders(sourceGUID, ds.SeqNum, from)
			p.dispatchToReaders(sourceGUID, "", rawPayload, pendingTS, uint64(ds.SeqNum.Low))

		case submsgHEARTBEAT:
			hb, ok := parseHeartbeat(body)
			if !ok {
				return nil
			}
			writerGUID := GUID{Prefix: hdr.GuidPrefix, Entity: hb.WriterEntityId}
			p.handleHeartbeat(writerGUID, hb, from)

		case submsgACKNACK:
			an, ok := parseAckNack(body)
			if !ok {
				return nil
			}
			p.handleAckNack(an, from)
		}
		return nil
	})
}

// notifyReliableReaders updates the recvTracker of any reliable reader that
// accepts this writer, and sends ACKNACK if there are gaps.
func (p *participant) notifyReliableReaders(writerGUID GUID, seqNum SequenceNumber, writerAddr *net.UDPAddr) {
	p.mu.Lock()
	readers := make([]*rtpsReader, 0, len(p.readers))
	for _, r := range p.readers {
		readers = append(readers, r)
	}
	p.mu.Unlock()

	for _, r := range readers {
		if !r.reliable || !r.acceptsSource(writerGUID) {
			continue
		}
		tracker := r.trackerFor(writerGUID)
		sn := snToU64(seqNum)
		tracker.record(sn)
		// The writer's history reaches at least this SN, so NACK any gap below it.
		base, bitmap, needAck := tracker.missing(sn)
		if !needAck || writerAddr == nil {
			continue
		}
		an := AckNack{
			ReaderEntityId: r.eid,
			WriterEntityId: writerGUID.Entity,
			Base:           u64ToSN(base),
			Bitmap:         bitmap,
			Count:          tracker.nextAckCount(),
		}
		msg := wrapInRTPSMessage(p.guidPrefix, marshalAckNack(an))
		_ = p.dataSock.send(writerAddr, msg)
	}
}

// handleHeartbeat responds with ACKNACK if we have gaps for this writer.
func (p *participant) handleHeartbeat(writerGUID GUID, hb Heartbeat, from *net.UDPAddr) {
	p.mu.Lock()
	readers := make([]*rtpsReader, 0, len(p.readers))
	for _, r := range p.readers {
		readers = append(readers, r)
	}
	p.mu.Unlock()

	for _, r := range readers {
		if !r.reliable || !r.acceptsSource(writerGUID) {
			continue
		}
		tracker := r.trackerFor(writerGUID)
		// On first contact, anchor the cumulative-ACK base at the writer's
		// FirstSN so the reader can request the writer's whole live history.
		tracker.initExpected(snToU64(hb.FirstSN))
		// Re-NACK every SN still missing up to the writer's LastSN. Because the
		// watermark never skips a gap, a lost retransmit is requested again on
		// each periodic HEARTBEAT until it arrives.
		base, bitmap, needAck := tracker.missing(snToU64(hb.LastSN))
		if !needAck || from == nil {
			continue
		}
		an := AckNack{
			ReaderEntityId: r.eid,
			WriterEntityId: writerGUID.Entity,
			Base:           u64ToSN(base),
			Bitmap:         bitmap,
			Count:          tracker.nextAckCount(),
		}
		msg := wrapInRTPSMessage(p.guidPrefix, marshalAckNack(an))
		_ = p.dataSock.send(from, msg)
	}
}

// handleAckNack retransmits missing samples from the writer's history and sends
// a GAP for any requested sequence numbers that have been evicted.
func (p *participant) handleAckNack(an AckNack, from *net.UDPAddr) {
	p.mu.Lock()
	w, ok := p.writers[an.WriterEntityId]
	p.mu.Unlock()
	if !ok || !w.reliable {
		return
	}
	// Advance the drain watermark: ackBase is the first SN not yet confirmed.
	ackBase := snToU64(an.Base)
	w.advanceAcked(ackBase)

	histFirst, _, histOK := w.history.firstLast()

	// Retransmit samples that are still in history.
	for bit := uint64(0); bit < 32; bit++ {
		if an.Bitmap&(1<<uint(bit)) == 0 {
			continue
		}
		seq := ackBase + bit
		msg := w.history.get(seq)
		if msg == nil {
			continue
		}
		for _, loc := range p.matchedReaderLocators(w.topic) {
			if dst := loc.udpAddr(); dst != nil {
				_ = p.dataSock.send(dst, msg)
			}
		}
	}

	// Send a GAP for the leading portion of the NACK range that has been
	// evicted from history. This allows the reader to advance its expected-SN
	// pointer instead of stalling waiting for samples we can never provide.
	if histOK && ackBase < histFirst {
		gapEnd := histFirst - 1
		// Cap to the 32-bit NACK bitmap range so we don't over-declare.
		if maxBit := ackBase + 31; gapEnd > maxBit {
			gapEnd = maxBit
		}
		g := Gap{
			ReaderEntityId: an.ReaderEntityId,
			WriterEntityId: an.WriterEntityId,
			GapStart:       u64ToSN(ackBase),
			GapEnd:         u64ToSN(gapEnd),
		}
		gapMsg := wrapInRTPSMessage(p.guidPrefix, marshalGAP(g))
		// Send directly to the requesting reader if we know its address.
		if from != nil {
			_ = p.dataSock.send(from, gapMsg)
		}
		// Also send to all matched readers so any reader on this topic can advance.
		for _, loc := range p.matchedReaderLocators(w.topic) {
			if dst := loc.udpAddr(); dst != nil {
				_ = p.dataSock.send(dst, gapMsg)
			}
		}
	}
}

// ── Dispatch ──────────────────────────────────────────────────────────────────

// dispatchToReaders delivers payload to all readers whose topic matches and
// whose accept-list includes source. topicFilter="" disables topic filtering
// (used for UDP paths where the topic is resolved via SEDP source GUID).
// ts is the source timestamp from INFO_TS (zero if not present).
// seqNum is the writer's sequence number for this sample (0 = not set).
func (p *participant) dispatchToReaders(source GUID, topicFilter string, payload []byte, ts time.Time, seqNum uint64) {
	ctx, span := p.tracer.Start(context.Background(), "dds.dispatch",
		dds.SpanAttribute{Key: "topic", Value: topicFilter},
	)
	defer span.End()
	_ = ctx

	var writerDDS dds.GUID
	copy(writerDDS[:12], source.Prefix[:])
	copy(writerDDS[12:], source.Entity[:])

	p.mu.Lock()
	readers := make([]*rtpsReader, 0, len(p.readers))
	for _, r := range p.readers {
		readers = append(readers, r)
	}
	p.mu.Unlock()

	for _, r := range readers {
		if topicFilter != "" && r.topic != topicFilter && !TopicMatches(r.topic, topicFilter) {
			continue
		}
		if !r.acceptsSource(source) {
			continue
		}
		sample := dds.Sample{
			Topic:          r.topic,
			Payload:        payload,
			Timestamp:      ts,
			WriterGUID:     writerDDS,
			SequenceNumber: seqNum,
		}
		if r.filter != nil && !r.filter(sample) {
			continue
		}
		p.deliverToReader(r, sample)
	}
}

// deliverToReader sends sample to r according to r.backPressure.
func (p *participant) deliverToReader(r *rtpsReader, sample dds.Sample) {
	byteLen := uint64(len(sample.Payload))
	tc := p.topicCounterFor(r.topic)
	delivered := false
	switch r.backPressure {
	case dds.DropOldest:
		select {
		case r.ch <- sample:
			p.mDelivers.Add(1)
			p.mBytesDeliv.Add(byteLen)
			tc.delivers.Add(1)
			tc.bytesD.Add(byteLen)
			delivered = true
		default:
			select {
			case <-r.ch:
				p.mDrops.Add(1)
				tc.drops.Add(1)
			default:
			}
			select {
			case r.ch <- sample:
				p.mDelivers.Add(1)
				p.mBytesDeliv.Add(byteLen)
				tc.delivers.Add(1)
				tc.bytesD.Add(byteLen)
				delivered = true
			default:
				p.mDrops.Add(1)
				tc.drops.Add(1)
			}
		}
	case dds.Block:
		r.ch <- sample
		p.mDelivers.Add(1)
		p.mBytesDeliv.Add(byteLen)
		tc.delivers.Add(1)
		tc.bytesD.Add(byteLen)
		delivered = true
	default: // DropNewest
		select {
		case r.ch <- sample:
			p.mDelivers.Add(1)
			p.mBytesDeliv.Add(byteLen)
			tc.delivers.Add(1)
			tc.bytesD.Add(byteLen)
			delivered = true
		default:
			p.mDrops.Add(1)
			tc.drops.Add(1)
		}
	}
	if delivered && r.resetDeadline != nil {
		r.resetDeadline()
	}
}

// tsnSocketForPCP returns the traffic-class socket for the given PCP value,
// creating it on first use. The socket has SO_PRIORITY set to pcp, IP_TOS
// set to dscp<<2, and (on Linux ≥ 4.19) SO_TXTIME enabled when wantTxTime.
// Returns nil on allocation failure; callers fall back to dataSock.
func (p *participant) tsnSocketForPCP(pcp, dscp uint8, wantTxTime bool) *udpSocket {
	p.tsnMu.Lock()
	defer p.tsnMu.Unlock()
	if p.tsnSocks == nil {
		p.tsnSocks = make(map[uint8]*udpSocket)
	}
	if sock, ok := p.tsnSocks[pcp]; ok {
		return sock
	}
	// Port 0 → OS assigns an ephemeral port.
	sock, err := newUnicastSocket(0)
	if err != nil {
		return nil
	}
	_ = setSockPriority(sock.conn, int(pcp))
	if dscp > 0 {
		_ = setSockTOS(sock.conn, dscp)
	}
	if wantTxTime {
		_ = enableTxTime(sock.conn) // best-effort; silently ignored on older kernels
	}
	p.tsnSocks[pcp] = sock
	return sock
}

// readerByEID calls fn with the reader matching eid, if any.
func (p *participant) readerByEID(eid EntityId, fn func(*rtpsReader)) {
	p.mu.Lock()
	r, ok := p.readers[eid]
	p.mu.Unlock()
	if ok {
		fn(r)
	}
}

// addWriterLocator stores the data-delivery locator for a remote writer.
func (p *participant) addWriterLocator(g GUID, l Locator) {
	p.mu.Lock()
	p.writerLocators[g] = l
	p.mu.Unlock()
}

// matchedReaderLocators returns the data-unicast locators for all remote
// participants that have an active subscription to topicName. Locators are
// deduplicated so a participant with multiple readers on the same topic
// receives only one copy of each DATA packet.
func (p *participant) matchedReaderLocators(topicName string) []Locator {
	p.sedp.mu.RLock()
	defer p.sedp.mu.RUnlock()
	var locators []Locator
	seen := make(map[Locator]bool)
	for guid, ri := range p.sedp.remoteReaders {
		if ri.topicName != topicName {
			continue
		}
		loc, ok := p.sedp.remoteReaderLocs[guid]
		if !ok || loc.Kind == LocatorKindInvalid {
			continue
		}
		if !seen[loc] {
			seen[loc] = true
			locators = append(locators, loc)
		}
	}
	return locators
}

// ── Writer ────────────────────────────────────────────────────────────────────

type rtpsWriter struct {
	p             *participant
	topic         string
	eid           EntityId
	qos           dds.QoS
	mu            sync.Mutex
	closed        bool
	seq           uint64 // next sequence number to assign (full 64-bit)
	acked         uint64 // highest sequence number fully acknowledged by all readers
	reliable      bool
	history       *sendHistory  // non-nil when reliable == true
	hbDone        chan struct{} // closed to stop the heartbeat goroutine
	drainCh       chan struct{} // closed when ackedLo >= seqLo (all ACKs in)
	deadlineTimer *time.Timer   // non-nil when QoS.Deadline > 0
	hbPeriod      time.Duration // heartbeat ticker period; set from participant option
	// TSN fields — nil when not a TSN writer.
	tsnStream *tsn.Stream // matching stream descriptor
	tsnSock   *udpSocket  // priority-marked socket (nil = use dataSock)
}

// fragmentSize returns the per-fragment payload cap for this writer.
// TSN streams use Stream.MaxFragPayload() to enforce the frame-size bound.
// All other writers use the default maxFragmentPayload constant.
func (w *rtpsWriter) fragmentSize() int {
	if w.tsnStream != nil {
		if n := w.tsnStream.MaxFragPayload(); n > 0 {
			return n
		}
	}
	return maxFragmentPayload
}

// sendSock returns the UDP socket to use for remote sends.
func (w *rtpsWriter) sendSock() *udpSocket {
	if w.tsnSock != nil {
		return w.tsnSock
	}
	return w.p.dataSock
}

func (w *rtpsWriter) Write(payload []byte) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return fmt.Errorf("rtps: %w", dds.ErrClosed)
	}
	// Enforce MaxSampleSize QoS before doing any work.
	if w.qos.MaxSampleSize > 0 && len(payload) > w.qos.MaxSampleSize {
		return fmt.Errorf("rtps: %w: got %d bytes, limit %d",
			dds.ErrPayloadTooLarge, len(payload), w.qos.MaxSampleSize)
	}
	if w.deadlineTimer != nil {
		w.deadlineTimer.Reset(w.qos.Deadline)
	}
	w.p.mWrites.Add(1)
	w.p.mBytesWritten.Add(uint64(len(payload)))
	topicTC := w.p.topicCounterFor(w.topic)
	topicTC.writes.Add(1)
	topicTC.bytesW.Add(uint64(len(payload)))
	w.seq++
	seqNum := u64ToSN(w.seq)
	now := time.Now()

	// Apply security before wrapping in CDR/RTPS.
	wirePayload := payload
	if w.p.security != nil {
		sealed, err := w.p.security.Seal(payload)
		if err != nil {
			return fmt.Errorf("rtps: security seal: %w", err)
		}
		wirePayload = sealed
	}
	wrapped := cdrWrapPayload(wirePayload)
	tsSubmsg := marshalInfoTS(now)

	// Determine whether to fragment: use the TSN-aware fragment size.
	fragSize := w.fragmentSize()
	sock := w.sendSock()

	var msgs [][]byte
	if len(wrapped) > fragSize {
		// Large payload: split into DATA_FRAG submessages.
		frags := splitIntoFragmentsN(w.eid, seqNum, wrapped, fragSize)
		msgs = make([][]byte, len(frags))
		for i, frag := range frags {
			fragSubmsg := marshalDataFrag(frag)
			msgs[i] = wrapInRTPSMessage(w.p.guidPrefix, append(tsSubmsg, fragSubmsg...))
		}
	} else {
		submsg := marshalDataSubmessage(w.eid, EntityIdUnknown, seqNum, wrapped)
		msgs = [][]byte{wrapInRTPSMessage(w.p.guidPrefix, append(tsSubmsg, submsg...))}
	}

	if w.reliable {
		// Store the full RTPS message (first fragment packet, or single DATA)
		// for heartbeat / retransmit. For fragmented payloads this stores only
		// the first fragment; a future enhancement can store per-fragment msgs.
		w.history.store(w.seq, msgs[0])
	}

	// Deliver locally (same process). Copy so caller mutations don't affect
	// the already-queued sample.
	localCopy := make([]byte, len(payload))
	copy(localCopy, payload)
	// Record for TransientLocal late-joiner delivery. sync.Map is safe here
	// without holding p.mu (avoids w.mu → p.mu lock inversion with Close).
	sample := dds.Sample{Topic: w.topic, Payload: localCopy, Timestamp: now}
	w.p.lastSample.Store(w.topic, &sample)
	persistFlush(w.p.persistDir, w.topic, localCopy)
	w.p.dispatchToReaders(GUID{Prefix: w.p.guidPrefix, Entity: w.eid}, w.topic, localCopy, now, w.seq)

	// Deliver to remote peers.
	locs := w.p.matchedReaderLocators(w.topic)
	// Compute scheduled transmit time for TSN streams (nanoseconds since TAI epoch).
	var txTimeNS uint64
	if w.tsnStream != nil && w.tsnStream.TxOffsetUS > 0 {
		if taiNow, err := clockTAINow(); err == nil {
			// Next interval boundary + TxOffset.
			interval := w.tsnStream.Interval()
			offset := w.tsnStream.TxOffset()
			if interval > 0 {
				sinceLast := taiNow.UnixNano() % int64(interval)
				nextBoundary := taiNow.Add(time.Duration(int64(interval)-sinceLast) + offset)
				txTimeNS = uint64(nextBoundary.UnixNano())
			}
		}
	}

	if len(locs) > 0 && w.p.dataMcastSock != nil && w.tsnSock == nil {
		// Multicast only when not a TSN writer (TSN streams must use per-class socket).
		dst := &net.UDPAddr{IP: userDataMulticastAddr, Port: userMulticastPort(int(w.p.domain))}
		for _, msg := range msgs {
			_ = w.p.dataSock.send(dst, msg)
		}
	} else {
		for _, loc := range locs {
			dst := loc.udpAddr()
			if dst == nil {
				continue
			}
			for _, msg := range msgs {
				if txTimeNS > 0 {
					_ = scheduledSend(sock.conn, dst, msg, txTimeNS)
				} else {
					_ = sock.send(dst, msg)
				}
			}
		}
	}

	// Send HEARTBEAT immediately after each reliable write so remote readers
	// can detect gaps without waiting for the periodic ticker.
	if w.reliable {
		w.sendHeartbeatLocked()
	}
	return nil
}

// sendHeartbeatLocked builds and sends a HEARTBEAT to all known reader
// locators. Caller must hold w.mu (or call only from the heartbeatLoop).
func (w *rtpsWriter) sendHeartbeatLocked() {
	first, last, ok := w.history.firstLast()
	if !ok {
		return
	}
	hb := Heartbeat{
		ReaderEntityId: EntityIdUnknown,
		WriterEntityId: w.eid,
		FirstSN:        u64ToSN(first),
		LastSN:         u64ToSN(last),
		Count:          w.history.hbCount.Add(1),
	}
	msg := wrapInRTPSMessage(w.p.guidPrefix, marshalHeartbeat(hb))
	sock := w.sendSock()
	for _, loc := range w.p.matchedReaderLocators(w.topic) {
		if dst := loc.udpAddr(); dst != nil {
			_ = sock.send(dst, msg)
		}
	}
}

// waitDrain blocks until all previously written sequence numbers have been
// acknowledged (ackedLo >= seqLo) or ctx is cancelled.
func (w *rtpsWriter) waitDrain(ctx context.Context) error {
	w.mu.Lock()
	if w.drainCh == nil || w.acked >= w.seq {
		w.mu.Unlock()
		return nil
	}
	ch := w.drainCh
	w.mu.Unlock()
	select {
	case <-ch:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// advanceAcked records that the remote reader has acknowledged up to (but not
// including) ackBase. When ackBase > seqLo, the drain channel is closed.
func (w *rtpsWriter) advanceAcked(ackBase uint64) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if ackBase == 0 {
		return
	}
	confirmed := ackBase - 1
	if confirmed > w.acked {
		w.acked = confirmed
	}
	if w.drainCh != nil && w.acked >= w.seq {
		select {
		case <-w.drainCh: // already closed
		default:
			close(w.drainCh)
		}
	}
}

// heartbeatLoop periodically sends a HEARTBEAT for as long as the writer is
// open, so remote readers can detect and recover from losses.
func (w *rtpsWriter) heartbeatLoop(done <-chan struct{}) {
	ticker := time.NewTicker(w.hbPeriod)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			w.mu.Lock()
			if !w.closed {
				w.sendHeartbeatLocked()
			}
			w.mu.Unlock()
		case <-done:
			return
		}
	}
}

// WriteCtx writes payload, returning ctx.Err() immediately if ctx is already
// done. Because RTPS writes are non-blocking, the context is only checked
// before the write begins.
func (w *rtpsWriter) WriteCtx(ctx context.Context, payload []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return w.Write(payload)
}

func (w *rtpsWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return nil
	}
	w.closed = true
	if w.hbDone != nil {
		close(w.hbDone)
		w.hbDone = nil // safe: heartbeatLoop captured the channel at startup
	}
	if w.deadlineTimer != nil {
		w.deadlineTimer.Stop()
		w.deadlineTimer = nil
	}
	return nil
}

// ── Reader ────────────────────────────────────────────────────────────────────

type rtpsReader struct {
	p             *participant
	topic         string
	eid           EntityId
	ch            chan dds.Sample
	mu            sync.RWMutex
	sources       map[GUID]struct{}     // SEDP-matched remote writer GUIDs
	trackers      map[GUID]*recvTracker // reliability trackers, one per remote writer
	reliable      bool
	filter        func(dds.Sample) bool // nil = no filter
	backPressure  dds.BackPressurePolicy
	unsubOnce     sync.Once   // guards deregistration from the participant
	closeOnce     sync.Once   // guards channel close
	resetDeadline func()      // nil if no deadline configured
	deadlineTimer *time.Timer // non-nil when QoS.Deadline > 0 and callback set
}

func (r *rtpsReader) addSourceGUID(g GUID) {
	r.mu.Lock()
	if r.sources == nil {
		r.sources = make(map[GUID]struct{})
	}
	r.sources[g] = struct{}{}
	r.mu.Unlock()
}

// trackerFor returns (creating if necessary) the recvTracker for a remote writer.
func (r *rtpsReader) trackerFor(writerGUID GUID) *recvTracker {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.trackers == nil {
		r.trackers = make(map[GUID]*recvTracker)
	}
	if t, ok := r.trackers[writerGUID]; ok {
		return t
	}
	t := &recvTracker{}
	r.trackers[writerGUID] = t
	return t
}

func (r *rtpsReader) acceptsSource(g GUID) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if len(r.sources) == 0 {
		return g.Prefix == r.p.guidPrefix
	}
	if g.Prefix == r.p.guidPrefix {
		return true
	}
	_, ok := r.sources[g]
	return ok
}

func (r *rtpsReader) C() <-chan dds.Sample { return r.ch }

// TryRead attempts a non-blocking read. Returns (zero, false) if empty or closed.
func (r *rtpsReader) TryRead() (dds.Sample, bool) {
	select {
	case s, ok := <-r.ch:
		if !ok {
			return dds.Sample{}, false
		}
		return s, true
	default:
		return dds.Sample{}, false
	}
}

// Unsubscribe removes this reader from the participant's endpoint registry so
// no new samples are dispatched. The channel remains open; call Close to also
// close the channel and release all reader resources.
func (r *rtpsReader) Unsubscribe() {
	r.unsubOnce.Do(func() {
		if r.deadlineTimer != nil {
			r.deadlineTimer.Stop()
		}
		r.p.mu.Lock()
		delete(r.p.readers, r.eid)
		r.p.mu.Unlock()
	})
}

func (r *rtpsReader) Close() error {
	r.Unsubscribe()
	r.closeOnce.Do(func() {
		if r.deadlineTimer != nil {
			r.deadlineTimer.Stop()
		}
		close(r.ch)
	})
	return nil
}
