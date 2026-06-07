// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Package mock provides an in-process, CGo-free implementation of the dds
// interfaces. All participants within the same process share a global
// in-memory broker: a Publisher.Write on topic T is delivered synchronously
// to every Subscriber.C() for topic T.
//
// The mock is the default implementation used by vissr and its test suite.
// It requires no installed system libraries and works on every platform.
// Switch to the cyclone package when a real DDS domain (multi-process,
// multi-host) is required.
package mock

import (
	"fmt"
	"sync"

	dds "github.com/SoundMatt/go-DDS"
)

// globalBroker is the process-wide in-memory pub/sub hub.
var globalBroker = &broker{subs: make(map[string][]chan dds.Sample)}

type broker struct {
	mu   sync.RWMutex
	subs map[string][]chan dds.Sample
}

func (b *broker) subscribe(topic string) chan dds.Sample {
	ch := make(chan dds.Sample, 64)
	b.mu.Lock()
	b.subs[topic] = append(b.subs[topic], ch)
	b.mu.Unlock()
	return ch
}

func (b *broker) unsubscribe(topic string, ch chan dds.Sample) {
	b.mu.Lock()
	defer b.mu.Unlock()
	list := b.subs[topic]
	for i, s := range list {
		if s == ch {
			b.subs[topic] = append(list[:i], list[i+1:]...)
			close(ch)
			return
		}
	}
}

func (b *broker) publish(topic string, payload []byte) {
	cp := make([]byte, len(payload))
	copy(cp, payload)
	b.mu.RLock()
	defer b.mu.RUnlock()
	for _, ch := range b.subs[topic] {
		select {
		case ch <- dds.Sample{Topic: topic, Payload: cp}:
		default:
			// Subscriber is not reading; drop rather than block the publisher.
		}
	}
}

// New creates a mock DDS Participant for the given domain. The domain
// parameter is accepted for API compatibility but has no effect — all mock
// participants share the same global broker regardless of domain.
func New(domain dds.Domain) (dds.Participant, error) {
	return &participant{}, nil
}

// participant implements dds.Participant.
type participant struct {
	mu     sync.Mutex
	closed bool
}

func (p *participant) NewPublisher(topic string, _ dds.QoS) (dds.Publisher, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return nil, fmt.Errorf("mock: participant closed")
	}
	return &publisher{topic: topic}, nil
}

func (p *participant) NewSubscriber(topic string, _ dds.QoS) (dds.Subscriber, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return nil, fmt.Errorf("mock: participant closed")
	}
	return &subscriber{topic: topic, ch: globalBroker.subscribe(topic)}, nil
}

func (p *participant) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.closed = true
	return nil
}

// publisher implements dds.Publisher.
type publisher struct {
	topic  string
	mu     sync.Mutex
	closed bool
}

func (pub *publisher) Write(payload []byte) error {
	pub.mu.Lock()
	defer pub.mu.Unlock()
	if pub.closed {
		return fmt.Errorf("mock: publisher closed")
	}
	globalBroker.publish(pub.topic, payload)
	return nil
}

func (pub *publisher) Close() error {
	pub.mu.Lock()
	defer pub.mu.Unlock()
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
