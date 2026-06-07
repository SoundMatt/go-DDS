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
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	dds "github.com/SoundMatt/go-DDS"
)

// globalBroker is the process-wide in-memory pub/sub hub.
var globalBroker = &broker{
	subs:       make(map[string][]subscription),
	lastSample: make(map[string]*dds.Sample),
}

// subscription holds a subscriber channel together with its optional filter.
type subscription struct {
	ch     chan dds.Sample
	filter func(dds.Sample) bool
}

// broker is the central in-memory routing hub.
type broker struct {
	mu         sync.RWMutex
	subs       map[string][]subscription
	lastSample map[string]*dds.Sample

	// Metrics counters (global across all participants sharing this broker).
	writes       atomic.Uint64
	delivers     atomic.Uint64
	drops        atomic.Uint64
	bytesWritten atomic.Uint64
	bytesDeliv   atomic.Uint64
}

func (b *broker) subscribe(topic string, qos dds.QoS, filter func(dds.Sample) bool) chan dds.Sample {
	ch := make(chan dds.Sample, 64)
	sub := subscription{ch: ch, filter: filter}
	b.mu.Lock()
	b.subs[topic] = append(b.subs[topic], sub)
	var last *dds.Sample
	if qos.Durability == dds.TransientLocal {
		last = b.lastSample[topic]
	}
	b.mu.Unlock()
	if last != nil {
		if filter == nil || filter(*last) {
			select {
			case ch <- *last:
			default:
			}
		}
	}
	return ch
}

func (b *broker) unsubscribe(topic string, ch chan dds.Sample) {
	b.mu.Lock()
	defer b.mu.Unlock()
	list := b.subs[topic]
	for i, s := range list {
		if s.ch == ch {
			b.subs[topic] = append(list[:i], list[i+1:]...)
			close(ch)
			return
		}
	}
}

func (b *broker) publish(topic string, payload []byte, qos dds.QoS) {
	cp := make([]byte, len(payload))
	copy(cp, payload)
	sample := dds.Sample{Topic: topic, Payload: cp}

	b.writes.Add(1)
	b.bytesWritten.Add(uint64(len(payload)))

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
		select {
		case sub.ch <- sample:
			b.delivers.Add(1)
			b.bytesDeliv.Add(uint64(len(payload)))
		default:
			b.drops.Add(1)
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

// New creates a mock DDS Participant for the given domain. Domain is accepted
// for API compatibility but has no effect — all mock participants share the
// same global broker regardless of domain.
func New(domain dds.Domain, opts ...Option) (dds.Participant, error) {
	p := &participant{}
	for _, o := range opts {
		o(p)
	}
	return p, nil
}

// participant implements dds.Participant.
type participant struct {
	mu         sync.Mutex
	closed     bool
	deadlineCb func(string)
}

func (p *participant) NewPublisher(topic string, qos dds.QoS) (dds.Publisher, error) {
	if topic == "" {
		return nil, fmt.Errorf("mock: %w", dds.ErrTopicEmpty)
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return nil, fmt.Errorf("mock: %w", dds.ErrClosed)
	}
	pub := &publisher{topic: topic, qos: qos, deadlineCb: p.deadlineCb}
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
	ch := globalBroker.subscribe(topic, qos, cfg.Filter)
	return &subscriber{topic: topic, ch: ch}, nil
}

func (p *participant) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.closed = true
	return nil
}

// Metrics implements dds.MetricsProvider.
func (p *participant) Metrics() dds.Metrics {
	return dds.Metrics{
		WriteCount:     globalBroker.writes.Load(),
		DeliverCount:   globalBroker.delivers.Load(),
		DropCount:      globalBroker.drops.Load(),
		BytesWritten:   globalBroker.bytesWritten.Load(),
		BytesDelivered: globalBroker.bytesDeliv.Load(),
	}
}

// publisher implements dds.Publisher.
type publisher struct {
	topic         string
	qos           dds.QoS
	deadlineCb    func(string)
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
	if pub.deadlineTimer != nil {
		pub.deadlineTimer.Reset(pub.qos.Deadline)
	}
	globalBroker.publish(pub.topic, payload, pub.qos)
	return nil
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
	topic string
	ch    chan dds.Sample
	once  sync.Once
}

func (sub *subscriber) C() <-chan dds.Sample { return sub.ch }

func (sub *subscriber) Close() error {
	sub.once.Do(func() { globalBroker.unsubscribe(sub.topic, sub.ch) })
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
