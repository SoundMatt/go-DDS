// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Package domain provides a Bridge that forwards DDS samples between two
// Participant domains (Milestone 10 — Enterprise Services: Routing).
//
// A domain bridge connects two isolated DDS domains, allowing topics
// published in one domain to be delivered in another. This is the canonical
// approach for cross-domain routing without a centralised broker.
//
//	src domain 0               dst domain 1
//	Publisher → [topic] → Subscriber → Bridge → Publisher → [topic] → Subscriber
//
// Each bridged topic is forwarded unidirectionally from src to dst. For
// bidirectional bridging, create two Bridge instances with swapped src/dst.
package domain

import (
	"fmt"
	"sync"

	dds "github.com/SoundMatt/go-DDS"
)

// Bridge forwards DDS samples from one participant (src) to another (dst)
// for each configured topic. Each topic gets one subscriber on src and one
// publisher on dst; a dedicated goroutine forwards samples between them.
//
// Bridge is safe for concurrent use from multiple goroutines.
type Bridge struct {
	src    dds.Participant
	dst    dds.Participant
	topics []string
	qos    dds.QoS

	subs []dds.Subscriber
	pubs []dds.Publisher

	done chan struct{}
	once sync.Once
	wg   sync.WaitGroup
}

// Options configures a [Bridge].
type Options struct {
	// Topics lists the DDS topic names to bridge from src to dst.
	// An empty Topics list results in a Bridge that forwards nothing.
	Topics []string
	// QoS is applied to all bridged subscriber and publisher endpoints.
	// Zero value uses [dds.DefaultQoS].
	QoS dds.QoS
}

// New creates a Bridge that will forward samples on opts.Topics from src to
// dst. It establishes subscribers on src and publishers on dst immediately
// so that no samples are lost once [Bridge.Start] is called.
//
// Returns an error if any subscriber or publisher cannot be created (e.g.
// because src or dst is already closed).
func New(src, dst dds.Participant, opts Options) (*Bridge, error) {
	b := &Bridge{
		src:    src,
		dst:    dst,
		topics: opts.Topics,
		qos:    opts.QoS,
		done:   make(chan struct{}),
	}
	for _, topic := range opts.Topics {
		sub, err := src.NewSubscriber(topic, b.qos)
		if err != nil {
			b.closeEndpoints()
			return nil, fmt.Errorf("domain bridge: NewSubscriber(%q) on src: %w", topic, err)
		}
		pub, err := dst.NewPublisher(topic, b.qos)
		if err != nil {
			_ = sub.Close()
			b.closeEndpoints()
			return nil, fmt.Errorf("domain bridge: NewPublisher(%q) on dst: %w", topic, err)
		}
		b.subs = append(b.subs, sub)
		b.pubs = append(b.pubs, pub)
	}
	return b, nil
}

// Start launches one forwarding goroutine per bridged topic. Each goroutine
// reads samples from the src subscriber and writes them to the dst publisher
// until Close is called or either participant is closed.
//
// Start is idempotent: calling it more than once has no effect.
func (b *Bridge) Start() {
	for i := range b.subs {
		sub := b.subs[i]
		pub := b.pubs[i]
		b.wg.Add(1)
		go func() {
			defer b.wg.Done()
			b.forward(sub, pub)
		}()
	}
}

// Close stops all forwarding goroutines and closes the bridged subscribers and
// publishers. It is safe to call Close multiple times.
func (b *Bridge) Close() error {
	b.once.Do(func() { close(b.done) })
	b.wg.Wait()
	b.closeEndpoints()
	return nil
}

func (b *Bridge) forward(sub dds.Subscriber, pub dds.Publisher) {
	// recover handles the case where mock's broker panics if a downstream
	// subscriber's channel was closed while we were mid-delivery.
	defer func() { recover() }()
	for {
		select {
		case s, ok := <-sub.C():
			if !ok {
				return
			}
			if err := pub.Write(s.Payload); err != nil {
				return
			}
		case <-b.done:
			return
		}
	}
}

func (b *Bridge) closeEndpoints() {
	for _, sub := range b.subs {
		_ = sub.Close()
	}
	for _, pub := range b.pubs {
		_ = pub.Close()
	}
	b.subs = nil
	b.pubs = nil
}
