// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Package mock provides an in-process, CGo-free implementation of the dds
// interfaces. All participants within the same process share a global
// in-memory broker: a Publisher.Write on topic T is delivered synchronously
// to every Subscriber.C() for topic T.
//
// The mock is the default implementation used by unit tests. Switch to the
// rtps or cyclone package when a real DDS domain (multi-process, multi-host)
// is required.
package mock

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	dds "github.com/SoundMatt/go-DDS"
)

// mockTopicCounter tracks per-topic publish/deliver/drop statistics in the broker.
type mockTopicCounter struct {
	writes   atomic.Uint64
	delivers atomic.Uint64
	drops    atomic.Uint64
	bytesW   atomic.Uint64
	bytesD   atomic.Uint64
}

// globalBroker is the process-wide in-memory pub/sub hub.
var globalBroker = &broker{
	subs:       make(map[string][]subscription),
	lastSample: make(map[string]*dds.Sample),
}

// subscription holds a subscriber channel together with its optional filter
// and back-pressure policy.
type subscription struct {
	ch           chan dds.Sample
	filter       func(dds.Sample) bool
	backPressure dds.BackPressurePolicy
}

// broker is the central in-memory routing hub.
type broker struct {
	mu         sync.RWMutex
	subs       map[string][]subscription
	lastSample map[string]*dds.Sample

	// Participant-level metrics counters (global across all participants sharing this broker).
	writes       atomic.Uint64
	delivers     atomic.Uint64
	drops        atomic.Uint64
	bytesWritten atomic.Uint64
	bytesDeliv   atomic.Uint64

	// Per-topic metrics: topic string → *mockTopicCounter (sync.Map, no lock needed).
	topicMetrics sync.Map
}

// topicCounterFor returns (creating on first access) the per-topic counter for topic.
func (b *broker) topicCounterFor(topic string) *mockTopicCounter {
	if v, ok := b.topicMetrics.Load(topic); ok {
		if tc, ok2 := v.(*mockTopicCounter); ok2 {
			return tc
		}
	}
	tc := &mockTopicCounter{}
	actual, _ := b.topicMetrics.LoadOrStore(topic, tc)
	if tc2, ok := actual.(*mockTopicCounter); ok {
		return tc2
	}
	return tc
}

const defaultChanDepth = 64

func (b *broker) subscribe(topic string, qos dds.QoS, cfg dds.SubscriberConfig) chan dds.Sample {
	depth := cfg.ChanDepth(defaultChanDepth)
	ch := make(chan dds.Sample, depth)
	sub := subscription{ch: ch, filter: cfg.Filter, backPressure: cfg.BackPressure}
	b.mu.Lock()
	b.subs[topic] = append(b.subs[topic], sub)
	var last *dds.Sample
	if qos.Durability == dds.TransientLocal {
		last = b.lastSample[topic]
	}
	b.mu.Unlock()
	if last != nil {
		if cfg.Filter == nil || cfg.Filter(*last) {
			select {
			case ch <- *last:
			default:
			}
		}
	}
	return ch
}

// removeSubscription removes the channel from the broker's subscription list
// without closing it. Used by Subscriber.Unsubscribe().
func (b *broker) removeSubscription(topic string, ch chan dds.Sample) {
	b.mu.Lock()
	defer b.mu.Unlock()
	list := b.subs[topic]
	for i, s := range list {
		if s.ch == ch {
			b.subs[topic] = append(list[:i], list[i+1:]...)
			return
		}
	}
}

func (b *broker) publish(topic string, payload []byte, qos dds.QoS) {
	cp := make([]byte, len(payload))
	copy(cp, payload)
	sample := dds.Sample{Topic: topic, Payload: cp, Timestamp: time.Now()}

	b.writes.Add(1)
	b.bytesWritten.Add(uint64(len(payload)))
	ptc := b.topicCounterFor(topic)
	ptc.writes.Add(1)
	ptc.bytesW.Add(uint64(len(payload)))

	b.mu.Lock()
	if qos.Durability == dds.TransientLocal {
		b.lastSample[topic] = &sample
	}
	subs := b.subs[topic]
	// Also deliver to wildcard subscribers whose pattern matches topic.
	for t, list := range b.subs {
		if t != topic && topicMatches(t, topic) {
			subs = append(subs, list...)
		}
	}
	b.mu.Unlock()

	for _, sub := range subs {
		if sub.filter != nil && !sub.filter(sample) {
			continue
		}
		b.deliver(sub, sample, uint64(len(payload)))
	}
}

// deliver routes sample to sub according to its back-pressure policy.
func (b *broker) deliver(sub subscription, sample dds.Sample, byteLen uint64) {
	tc := b.topicCounterFor(sample.Topic)
	switch sub.backPressure {
	case dds.DropOldest:
		select {
		case sub.ch <- sample:
			b.delivers.Add(1)
			b.bytesDeliv.Add(byteLen)
			tc.delivers.Add(1)
			tc.bytesD.Add(byteLen)
		default:
			// Evict oldest, then retry.
			select {
			case <-sub.ch:
				b.drops.Add(1)
				tc.drops.Add(1)
			default:
			}
			select {
			case sub.ch <- sample:
				b.delivers.Add(1)
				b.bytesDeliv.Add(byteLen)
				tc.delivers.Add(1)
				tc.bytesD.Add(byteLen)
			default:
				b.drops.Add(1)
				tc.drops.Add(1)
			}
		}
	case dds.Block:
		sub.ch <- sample
		b.delivers.Add(1)
		b.bytesDeliv.Add(byteLen)
		tc.delivers.Add(1)
		tc.bytesD.Add(byteLen)
	default: // DropNewest
		select {
		case sub.ch <- sample:
			b.delivers.Add(1)
			b.bytesDeliv.Add(byteLen)
			tc.delivers.Add(1)
			tc.bytesD.Add(byteLen)
		default:
			b.drops.Add(1)
			tc.drops.Add(1)
		}
	}
}

// Option configures a mock participant.
type Option func(*participant)

// WithDeadlineCallback registers fn to be called when a publisher has not
// written within its QoS.Deadline period.
func WithDeadlineCallback(fn func(topic string)) Option {
	return func(p *participant) { p.deadlineCb = fn }
}

// WithLogger sets the structured logger used by this participant.
// Log output is zero-cost when l is nil.
func WithLogger(l *slog.Logger) Option {
	return func(p *participant) { p.log = l }
}

// WithLivelinessCallback registers fn to be called when a participant is
// discovered or lost. In the mock, participants fire LivelinessGained on New
// and LivelinessLost on Close.
func WithLivelinessCallback(fn func(dds.GUID, dds.LivelinessEvent)) Option {
	return func(p *participant) { p.livelinessCb = fn }
}

// IsolatedBroker creates the participant with its own independent broker that
// is not shared with any other participant. Use this when testing components
// that bridge between separate DDS domains (e.g. WAN or domain bridges), where
// publishing on one participant must not echo to subscribers on another.
func IsolatedBroker() Option {
	return func(p *participant) {
		p.broker = &broker{
			subs:       make(map[string][]subscription),
			lastSample: make(map[string]*dds.Sample),
		}
	}
}

// New creates a mock DDS Participant for the given domain. Domain is accepted
// for API compatibility but has no effect — all mock participants share the
// same global broker regardless of domain (unless IsolatedBroker is used).
func New(domain dds.Domain, opts ...Option) (dds.Participant, error) {
	p := &participant{broker: globalBroker, domain: domain}
	for _, o := range opts {
		o(p)
	}
	// Assign a random GUID for this participant and fire LivelinessGained.
	p.guid = newMockGUID()
	if p.livelinessCb != nil {
		p.livelinessCb(p.guid, dds.LivelinessGained)
	}
	p.logf("new mock participant guid=%x domain=%d", p.guid, domain)
	return p, nil
}

// participant implements dds.Participant.
type participant struct {
	mu           sync.Mutex
	closed       bool
	broker       *broker
	domain       dds.Domain
	deadlineCb   func(string)
	log          *slog.Logger
	livelinessCb func(dds.GUID, dds.LivelinessEvent)
	guid         dds.GUID
}

func (p *participant) logf(msg string, args ...any) {
	if p.log != nil {
		p.log.Debug(fmt.Sprintf(msg, args...))
	}
}

// Domain implements dds.Participant.
func (p *participant) Domain() dds.Domain { return p.domain }

func (p *participant) NewPublisher(topic string, qos dds.QoS) (dds.Publisher, error) {
	if topic == "" {
		return nil, fmt.Errorf("mock: %w", dds.ErrTopicEmpty)
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return nil, fmt.Errorf("mock: %w", dds.ErrClosed)
	}
	p.logf("new publisher topic=%s reliability=%d", topic, qos.Reliability)
	pub := &publisher{broker: p.broker, topic: topic, qos: qos, deadlineCb: p.deadlineCb, log: p.log}
	if qos.Deadline > 0 && p.deadlineCb != nil {
		pub.deadlineTimer = time.AfterFunc(qos.Deadline, func() {
			p.deadlineCb(topic)
		})
	}
	return pub, nil
}

func (p *participant) NewSubscriber(topic string, qos dds.QoS, opts ...dds.SubscriberOption) (dds.Subscriber, error) {
	if topic == "" {
		return nil, fmt.Errorf("mock: %w", dds.ErrTopicEmpty)
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return nil, fmt.Errorf("mock: %w", dds.ErrClosed)
	}
	cfg := dds.ApplySubscriberOpts(opts)
	p.logf("new subscriber topic=%s depth=%d backpressure=%d", topic, cfg.ChanDepth(defaultChanDepth), cfg.BackPressure)
	ch := p.broker.subscribe(topic, qos, cfg)
	return &subscriber{broker: p.broker, topic: topic, ch: ch}, nil
}

func (p *participant) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return nil
	}
	p.closed = true
	if p.livelinessCb != nil {
		p.livelinessCb(p.guid, dds.LivelinessLost)
	}
	p.logf("participant closed guid=%x", p.guid)
	return nil
}

// CloseWithDrain implements dds.Drainer. In the mock, all writes are
// synchronous so there are no in-flight ACKs; this is equivalent to Close.
func (p *participant) CloseWithDrain(_ context.Context) error {
	return p.Close()
}

// Metrics implements dds.MetricsProvider.
func (p *participant) Metrics() dds.Metrics {
	return dds.Metrics{
		WriteCount:     p.broker.writes.Load(),
		DeliverCount:   p.broker.delivers.Load(),
		DropCount:      p.broker.drops.Load(),
		BytesWritten:   p.broker.bytesWritten.Load(),
		BytesDelivered: p.broker.bytesDeliv.Load(),
	}
}

// DiscoveryMetrics implements dds.DiscoveryMetricsProvider.
// The mock has no real network discovery; this always returns zero values.
func (p *participant) DiscoveryMetrics() dds.DiscoveryMetrics {
	return dds.DiscoveryMetrics{}
}

// TopicMetrics implements dds.TopicMetricsProvider.
func (p *participant) TopicMetrics() []dds.TopicMetrics {
	var result []dds.TopicMetrics
	p.broker.topicMetrics.Range(func(k, v any) bool {
		topic, ok := k.(string)
		if !ok {
			return true
		}
		tc, ok2 := v.(*mockTopicCounter)
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
	p.mu.Unlock()
	if closed {
		return dds.Health{
			Status:  dds.HealthDown,
			Details: map[string]string{"state": "closed"},
		}
	}
	return dds.Health{Status: dds.HealthOK}
}

// publisher implements dds.Publisher.
type publisher struct {
	broker        *broker
	topic         string
	qos           dds.QoS
	deadlineCb    func(string)
	log           *slog.Logger
	deadlineTimer *time.Timer
	mu            sync.Mutex
	closed        bool
}

func (pub *publisher) Write(payload []byte) error {
	pub.mu.Lock()
	defer pub.mu.Unlock()
	if pub.closed {
		return fmt.Errorf("mock: %w", dds.ErrClosed)
	}
	if pub.qos.MaxSampleSize > 0 && len(payload) > pub.qos.MaxSampleSize {
		return fmt.Errorf("mock: %w: got %d bytes, limit %d",
			dds.ErrPayloadTooLarge, len(payload), pub.qos.MaxSampleSize)
	}
	if pub.deadlineTimer != nil {
		pub.deadlineTimer.Reset(pub.qos.Deadline)
	}
	if pub.log != nil {
		pub.log.Debug("publish", "topic", pub.topic, "bytes", len(payload))
	}
	pub.broker.publish(pub.topic, payload, pub.qos)
	return nil
}

// WriteCtx writes payload, returning ctx.Err() immediately if the context is
// already cancelled. Note: Block back-pressure policy does not honour ctx after
// the Write call begins.
func (pub *publisher) WriteCtx(ctx context.Context, payload []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return pub.Write(payload)
}

func (pub *publisher) Close() error {
	pub.mu.Lock()
	defer pub.mu.Unlock()
	if pub.deadlineTimer != nil {
		pub.deadlineTimer.Stop()
		pub.deadlineTimer = nil
	}
	pub.closed = true
	return nil
}

// subscriber implements dds.Subscriber.
type subscriber struct {
	broker    *broker
	topic     string
	ch        chan dds.Sample
	unsubOnce sync.Once
	closeOnce sync.Once
}

func (sub *subscriber) C() <-chan dds.Sample { return sub.ch }

// Unsubscribe removes this subscriber from the broker without closing its
// channel. After Unsubscribe no new samples are delivered, but the channel
// remains readable for any buffered samples.
func (sub *subscriber) Unsubscribe() error {
	sub.unsubOnce.Do(func() { sub.broker.removeSubscription(sub.topic, sub.ch) })
	return nil
}

func (sub *subscriber) Close() error {
	_ = sub.Unsubscribe()
	sub.closeOnce.Do(func() { close(sub.ch) })
	return nil
}

// topicMatches returns true when pattern (which may contain MQTT-style + and #
// wildcards) matches the concrete topic name.
// "foo/" and "foo" are distinct topics (two levels vs one level).
func topicMatches(pattern, topic string) bool {
	return matchSlices(strings.Split(pattern, "/"), strings.Split(topic, "/"))
}

func matchSlices(pSegs, tSegs []string) bool {
	if len(pSegs) == 0 {
		return len(tSegs) == 0
	}
	if pSegs[0] == "#" {
		return true
	}
	if len(tSegs) == 0 {
		return false
	}
	if pSegs[0] == "+" || pSegs[0] == tSegs[0] {
		return matchSlices(pSegs[1:], tSegs[1:])
	}
	return false
}

// newMockGUID returns a pseudo-random 16-byte GUID backed by the current time.
// Not cryptographically random; sufficient for in-process participant identity.
func newMockGUID() dds.GUID {
	var g dds.GUID
	ns := time.Now().UnixNano()
	for i := 0; i < 8; i++ {
		g[i] = byte(ns >> (i * 8))
	}
	return g
}
