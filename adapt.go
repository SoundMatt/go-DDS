// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// This file is the RELAY adapter module for go-DDS (RELAY spec §13.6/§13.7).
// Per §13.7.1 the adapter module MUST be named `adapt`; in Go that is the
// `adapt.go` file in the package root, exposing the entry point Adapt() and the
// canonical Sample↔relay.Message conversions ToMessage/FromMessage (§15.7).

package dds

import (
	"context"
	"encoding/hex"
	"sync"

	relay "github.com/SoundMatt/RELAY/v2"
)

// ── canonical type conversion (§15.7) ──────────────────────────────────────────

//fusa:req REQ-LLR-006

// ToMessage converts this Sample to a relay.Message envelope (spec §15.7.2).
func (s Sample) ToMessage() relay.Message {
	return relay.Message{
		Protocol:  relay.DDS,
		ID:        s.Topic,
		Payload:   s.Payload,
		Timestamp: s.Timestamp,
		Seq:       s.SequenceNumber,
		Meta:      map[string]string{"dds.writer_guid": hex.EncodeToString(s.WriterGUID[:])},
	}
}

// FromMessage converts a relay.Message envelope back to a Sample (spec §15.7.2).
func FromMessage(m relay.Message) (Sample, error) {
	s := Sample{
		Topic:          m.ID,
		Payload:        m.Payload,
		Timestamp:      m.Timestamp,
		SequenceNumber: m.Seq,
	}
	if g, ok := m.Meta["dds.writer_guid"]; ok {
		if b, err := hex.DecodeString(g); err == nil && len(b) == 16 {
			copy(s.WriterGUID[:], b)
		}
	}
	return s, nil
}

// ── relay.Node adapter ────────────────────────────────────────────────────────

// Adapt wraps p as a relay.Node (spec §10.3). Send publishes to the topic
// named by msg.ID, caching publishers by topic for efficiency. Subscribe
// returns a channel that closes when the node closes; DDS subscriptions are
// topic-specific — use Participant.NewSubscriber for per-topic receive paths.
//
//nolint:ireturn
func Adapt(p Participant) relay.Node {
	return &ddsNode{p: p, done: make(chan struct{})}
}

type ddsNode struct {
	p    Participant
	pubs sync.Map // map[string]Publisher — cached by topic
	subs sync.Map // map[Subscriber]struct{} — live subscriptions
	once sync.Once
	done chan struct{}
}

func (n *ddsNode) Protocol() relay.Protocol { return relay.DDS }

func (n *ddsNode) Send(ctx context.Context, msg relay.Message) error {
	select {
	case <-n.done:
		return ErrClosed
	default:
	}
	if msg.ID == "" {
		return ErrTopicEmpty
	}
	if v, ok := n.pubs.Load(msg.ID); ok {
		if p, ok2 := v.(Publisher); ok2 {
			return p.WriteCtx(ctx, msg.Payload)
		}
	}
	pub, err := n.p.NewPublisher(msg.ID, DefaultQoS)
	if err != nil {
		return err
	}
	if actual, loaded := n.pubs.LoadOrStore(msg.ID, pub); loaded {
		_ = pub.Close()
		if p, ok2 := actual.(Publisher); ok2 {
			return p.WriteCtx(ctx, msg.Payload)
		}
	}
	return pub.WriteCtx(ctx, msg.Payload)
}

// Subscribe creates a DDS subscription routed by the topic carried in the
// relay.WithTopic option (spec §14.1). It returns ErrNotConnected when no topic
// is supplied, since a DDS subscription cannot exist without a topic. Inbound
// samples are forwarded as relay.Message via Sample.ToMessage(). The channel is
// closed when the node closes or the underlying subscription ends (spec §6.3).
func (n *ddsNode) Subscribe(opts ...relay.SubscriberOption) (<-chan relay.Message, error) {
	select {
	case <-n.done:
		ch := make(chan relay.Message)
		close(ch)
		return ch, ErrClosed
	default:
	}
	cfg := relay.ApplySubscriberOpts(opts)
	if cfg.TopicName == "" {
		return nil, ErrNotConnected
	}
	sub, err := n.p.NewSubscriber(cfg.TopicName, DefaultQoS)
	if err != nil {
		return nil, err
	}
	n.subs.Store(sub, struct{}{})

	ch := make(chan relay.Message, cfg.ChanDepth(64))
	go func() {
		defer close(ch)
		defer func() {
			n.subs.Delete(sub)
			_ = sub.Close()
		}()
		for {
			select {
			case <-n.done:
				return
			case s, ok := <-sub.C():
				if !ok {
					return
				}
				if !n.deliver(ch, s.ToMessage(), cfg.BackPressure) {
					return
				}
			}
		}
	}()
	return ch, nil
}

// deliver forwards msg to ch honoring the back-pressure policy (spec §9). It
// returns false only when the node is closing, signalling the reader to stop.
func (n *ddsNode) deliver(ch chan relay.Message, msg relay.Message, policy relay.BackPressurePolicy) bool {
	if policy == relay.Block {
		select {
		case ch <- msg:
			return true
		case <-n.done:
			return false
		}
	}
	// DropNewest (default) and DropOldest are non-blocking on a full channel.
	select {
	case ch <- msg:
		return true
	case <-n.done:
		return false
	default:
	}
	if policy == relay.DropOldest {
		select {
		case <-ch: // discard the oldest buffered message
		default:
		}
		select {
		case ch <- msg:
		default:
		}
	}
	// DropNewest: the arriving message is dropped.
	return true
}

func (n *ddsNode) Close() error {
	n.once.Do(func() {
		close(n.done)
		n.pubs.Range(func(_, v any) bool {
			if p, ok := v.(Publisher); ok {
				_ = p.Close()
			}
			return true
		})
		n.subs.Range(func(k, _ any) bool {
			if s, ok := k.(Subscriber); ok {
				_ = s.Close()
			}
			return true
		})
	})
	return n.p.Close()
}
