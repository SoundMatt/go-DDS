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
)

// ── Options ───────────────────────────────────────────────────────────────────

// SecurityPlugin is satisfied by any type that can seal (encrypt / sign) and
// open (decrypt / verify) a DDS payload byte slice. Built-in implementations
// live in the security sub-package; NullPlugin (pass-through) is also
// available there for development and testing.
type SecurityPlugin interface {
	Seal(plaintext []byte) ([]byte, error)
	Open(ciphertext []byte) ([]byte, error)
}

// Option configures a Participant at creation time.
type Option func(*participant)

// WithSecurity returns an Option that applies plugin to every payload transmitted
// and received by this participant. All peers that communicate with this
// participant must use the same plugin and key material.
func WithSecurity(plugin SecurityPlugin) Option {
	return func(p *participant) { p.security = plugin }
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

	// Metrics counters — incremented atomically, never need p.mu.
	mWrites       atomic.Uint64
	mDelivers     atomic.Uint64
	mDrops        atomic.Uint64
	mBytesWritten atomic.Uint64
	mBytesDeliv   atomic.Uint64
}

// New creates an RTPS participant joined to the given DDS domain.
// It binds UDP sockets, starts SPDP/SEDP, and returns a dds.Participant.
func New(domain dds.Domain, opts ...Option) (dds.Participant, error) {
	return newParticipant(domain, opts...)
}

func newParticipant(domain dds.Domain, opts ...Option) (*participant, error) {
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
	n := atomic.AddUint32(&p.entityCounter, 1)
	eid := entityIdForWriter(n)
	w := &rtpsWriter{
		p:        p,
		topic:    topic,
		eid:      eid,
		qos:      qos,
		reliable: qos.Reliability == dds.Reliable,
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
	p.writers[eid] = w
	p.sedp.registerWriter(eid, topic)
	p.log.debug("new publisher topic=%s reliable=%v", topic, w.reliable)
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
		pkt := recv.Interface().(udpPacket)
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
			sourceGUID := GUID{Prefix: hdr.GuidPrefix, Entity: ds.WriterEntityId}
			p.notifyReliableReaders(sourceGUID, ds.SeqNum, from)
			p.dispatchToReaders(sourceGUID, "", rawPayload, pendingTS)

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
		base, bitmap, needAck := tracker.receive(seqNum.Low)
		if !needAck || writerAddr == nil {
			continue
		}
		an := AckNack{
			ReaderEntityId: r.eid,
			WriterEntityId: writerGUID.Entity,
			Base:           SequenceNumber{Low: base},
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
		// If expected < firstSN the reader has never received anything from this
		// writer; ACKNACK base = firstSN with bitmap=0 to confirm empty state.
		if tracker.expected == 0 {
			tracker.mu.Lock()
			tracker.expected = hb.FirstSN.Low
			tracker.mu.Unlock()
		}
		base, bitmap, needAck := tracker.receive(hb.LastSN.Low)
		// Even if there's no gap at LastSN, check whether expected < lastSN
		// (i.e. we're behind) and send a cumulative ACKNACK.
		_ = base
		_ = bitmap
		if !needAck {
			continue
		}
		if from == nil {
			continue
		}
		an := AckNack{
			ReaderEntityId: r.eid,
			WriterEntityId: writerGUID.Entity,
			Base:           SequenceNumber{Low: tracker.expected},
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
	w.advanceAcked(an.Base.Low)

	histFirst, _, histOK := w.history.firstLast()

	// Retransmit samples that are still in history.
	for bit := uint32(0); bit < 32; bit++ {
		if an.Bitmap&(1<<bit) == 0 {
			continue
		}
		seqLo := an.Base.Low + bit
		msg := w.history.get(seqLo)
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
	if histOK && an.Base.Low < histFirst {
		gapEnd := histFirst - 1
		// Cap to the 32-bit NACK bitmap range so we don't over-declare.
		if maxBit := an.Base.Low + 31; gapEnd > maxBit {
			gapEnd = maxBit
		}
		g := Gap{
			ReaderEntityId: an.ReaderEntityId,
			WriterEntityId: an.WriterEntityId,
			GapStart:       SequenceNumber{Low: an.Base.Low},
			GapEnd:         SequenceNumber{Low: gapEnd},
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
func (p *participant) dispatchToReaders(source GUID, topicFilter string, payload []byte, ts time.Time) {
	ctx, span := p.tracer.Start(context.Background(), "dds.dispatch",
		dds.SpanAttribute{Key: "topic", Value: topicFilter},
	)
	defer span.End()
	_ = ctx

	p.mu.Lock()
	readers := make([]*rtpsReader, 0, len(p.readers))
	for _, r := range p.readers {
		readers = append(readers, r)
	}
	p.mu.Unlock()

	for _, r := range readers {
		if topicFilter != "" && r.topic != topicFilter {
			continue
		}
		if !r.acceptsSource(source) {
			continue
		}
		sample := dds.Sample{Topic: r.topic, Payload: payload, Timestamp: ts}
		if r.filter != nil && !r.filter(sample) {
			continue
		}
		p.deliverToReader(r, sample)
	}
}

// deliverToReader sends sample to r according to r.backPressure.
func (p *participant) deliverToReader(r *rtpsReader, sample dds.Sample) {
	byteLen := uint64(len(sample.Payload))
	switch r.backPressure {
	case dds.DropOldest:
		select {
		case r.ch <- sample:
			p.mDelivers.Add(1)
			p.mBytesDeliv.Add(byteLen)
		default:
			select {
			case <-r.ch:
				p.mDrops.Add(1)
			default:
			}
			select {
			case r.ch <- sample:
				p.mDelivers.Add(1)
				p.mBytesDeliv.Add(byteLen)
			default:
				p.mDrops.Add(1)
			}
		}
	case dds.Block:
		r.ch <- sample
		p.mDelivers.Add(1)
		p.mBytesDeliv.Add(byteLen)
	default: // DropNewest
		select {
		case r.ch <- sample:
			p.mDelivers.Add(1)
			p.mBytesDeliv.Add(byteLen)
		default:
			p.mDrops.Add(1)
		}
	}
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
	seqHi         int32
	seqLo         uint32
	ackedLo       uint32 // highest sequence number fully acknowledged by all readers
	reliable      bool
	history       *sendHistory  // non-nil when reliable == true
	hbDone        chan struct{} // closed to stop the heartbeat goroutine
	drainCh       chan struct{} // closed when ackedLo >= seqLo (all ACKs in)
	deadlineTimer *time.Timer   // non-nil when QoS.Deadline > 0
}

func (w *rtpsWriter) Write(payload []byte) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return fmt.Errorf("rtps: %w", dds.ErrClosed)
	}
	if w.deadlineTimer != nil {
		w.deadlineTimer.Reset(w.qos.Deadline)
	}
	w.p.mWrites.Add(1)
	w.p.mBytesWritten.Add(uint64(len(payload)))
	w.seqLo++
	seqNum := SequenceNumber{High: w.seqHi, Low: w.seqLo}
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
	submsg := marshalDataSubmessage(w.eid, EntityIdUnknown, seqNum, wrapped)
	// Prepend INFO_TS so remote readers can attach the source timestamp.
	tsSubmsg := marshalInfoTS(now)
	msg := wrapInRTPSMessage(w.p.guidPrefix, append(tsSubmsg, submsg...))

	if w.reliable {
		w.history.store(w.seqLo, msg)
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
	w.p.dispatchToReaders(GUID{Prefix: w.p.guidPrefix, Entity: w.eid}, w.topic, localCopy, now)

	// Deliver to remote peers. Use a single multicast send when available
	// (one packet instead of N unicast sends); fall back to unicast otherwise.
	locs := w.p.matchedReaderLocators(w.topic)
	if len(locs) > 0 && w.p.dataMcastSock != nil {
		dst := &net.UDPAddr{IP: userDataMulticastAddr, Port: userMulticastPort(int(w.p.domain))}
		_ = w.p.dataSock.send(dst, msg)
	} else {
		for _, loc := range locs {
			dst := loc.udpAddr()
			if dst == nil {
				continue
			}
			_ = w.p.dataSock.send(dst, msg)
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
		FirstSN:        SequenceNumber{Low: first},
		LastSN:         SequenceNumber{Low: last},
		Count:          w.history.hbCount.Add(1),
	}
	msg := wrapInRTPSMessage(w.p.guidPrefix, marshalHeartbeat(hb))
	for _, loc := range w.p.matchedReaderLocators(w.topic) {
		if dst := loc.udpAddr(); dst != nil {
			_ = w.p.dataSock.send(dst, msg)
		}
	}
}

// waitDrain blocks until all previously written sequence numbers have been
// acknowledged (ackedLo >= seqLo) or ctx is cancelled.
func (w *rtpsWriter) waitDrain(ctx context.Context) error {
	w.mu.Lock()
	if w.drainCh == nil || w.ackedLo >= w.seqLo {
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
func (w *rtpsWriter) advanceAcked(ackBase uint32) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if ackBase == 0 {
		return
	}
	confirmed := ackBase - 1
	if confirmed > w.ackedLo {
		w.ackedLo = confirmed
	}
	if w.drainCh != nil && w.ackedLo >= w.seqLo {
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
	ticker := time.NewTicker(heartbeatPeriod)
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
	p            *participant
	topic        string
	eid          EntityId
	ch           chan dds.Sample
	mu           sync.RWMutex
	sources      map[GUID]struct{}     // SEDP-matched remote writer GUIDs
	trackers     map[GUID]*recvTracker // reliability trackers, one per remote writer
	reliable     bool
	filter       func(dds.Sample) bool // nil = no filter
	backPressure dds.BackPressurePolicy
	once         sync.Once
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

func (r *rtpsReader) Close() error {
	r.once.Do(func() {
		r.p.mu.Lock()
		delete(r.p.readers, r.eid)
		r.p.mu.Unlock()
		close(r.ch)
	})
	return nil
}
