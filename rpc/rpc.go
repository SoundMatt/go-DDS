// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Package rpc implements OMG DDS-RPC style request-reply over a DDS participant.
// A Requester sends typed requests and receives correlated replies. A Replier
// receives requests and sends replies. Both use two topics internally:
// <base>/request and <base>/reply.
//
// Wire format: every payload is prefixed with a 16-byte CorrelationID so that
// the Requester can match replies to outstanding requests without any
// broker-level correlation support.
package rpc

//fusa:req REQ-RPC-001
//fusa:req REQ-RPC-002
//fusa:req REQ-RPC-003
//fusa:req REQ-RPC-004
//fusa:req REQ-RPC-005
//fusa:req REQ-RPC-006

import (
	"context"
	"crypto/rand"
	"fmt"
	"sync"

	dds "github.com/SoundMatt/go-DDS"
)

// ErrNoReply is returned by Requester.Request when the context expires before
// a correlated reply arrives.
var ErrNoReply = fmt.Errorf("rpc: no reply received")

// CorrelationID is the 16-byte identifier embedded in every request and reply
// payload to match replies to outstanding requests.
type CorrelationID [16]byte

// Request is an inbound RPC request delivered on the Replier.Requests channel.
type Request[Req any] struct {
	ID    CorrelationID
	Value Req
}

func newCorrID() CorrelationID {
	var id CorrelationID
	writeN, writeErr := rand.Read(id[:])
	_ = writeN
	_ = writeErr // crypto/rand.Read never returns an error since Go 1.20
	return id
}

func encodeRPC(id CorrelationID, payload []byte) []byte {
	out := make([]byte, 16+len(payload))
	copy(out[:16], id[:])
	copy(out[16:], payload)
	return out
}

func decodeRPC(data []byte) (CorrelationID, []byte, error) {
	if len(data) < 16 {
		return CorrelationID{}, nil, fmt.Errorf("rpc: payload too short (%d bytes)", len(data))
	}
	var id CorrelationID
	copy(id[:], data[:16])
	return id, data[16:], nil
}

// Requester sends typed RPC requests and waits for correlated replies.
// Create with NewRequester; call Close when done.
type Requester[Req, Rep any] struct {
	pub       dds.Publisher
	sub       dds.Subscriber
	reqCodec  dds.Codec[Req]
	repCodec  dds.Codec[Rep]
	mu        sync.Mutex
	pending   map[CorrelationID]chan dds.Sample
	done      chan struct{}
	closeOnce sync.Once
}

// NewRequester creates a Requester on the given participant.
// It publishes requests on <topic>/request and receives replies on <topic>/reply.
func NewRequester[Req, Rep any](
	p dds.Participant,
	topic string,
	reqCodec dds.Codec[Req],
	repCodec dds.Codec[Rep],
	qos dds.QoS,
) (*Requester[Req, Rep], error) {
	pub, err := p.NewPublisher(topic+"/request", qos)
	if err != nil {
		return nil, fmt.Errorf("rpc: requester publisher: %w", err)
	}
	sub, err := p.NewSubscriber(topic+"/reply", qos)
	if err != nil {
		_ = pub.Close()
		return nil, fmt.Errorf("rpc: requester subscriber: %w", err)
	}
	r := &Requester[Req, Rep]{
		pub:      pub,
		sub:      sub,
		reqCodec: reqCodec,
		repCodec: repCodec,
		pending:  make(map[CorrelationID]chan dds.Sample),
		done:     make(chan struct{}),
	}
	go r.demux()
	return r, nil
}

func (r *Requester[Req, Rep]) demux() {
	for {
		select {
		case s, ok := <-r.sub.C():
			if !ok {
				return
			}
			id, _, err := decodeRPC(s.Payload)
			if err != nil {
				continue
			}
			r.mu.Lock()
			ch, ok2 := r.pending[id]
			if ok2 {
				delete(r.pending, id)
			}
			r.mu.Unlock()
			if ok2 {
				select {
				case ch <- s:
				default:
				}
			}
		case <-r.done:
			return
		}
	}
}

// Request sends req and blocks until a correlated reply arrives or ctx expires.
func (r *Requester[Req, Rep]) Request(ctx context.Context, req Req) (Rep, error) {
	var zero Rep
	encoded, err := r.reqCodec.Marshal(req)
	if err != nil {
		return zero, fmt.Errorf("rpc: encode request: %w", err)
	}
	id := newCorrID()
	replyCh := make(chan dds.Sample, 1)
	r.mu.Lock()
	r.pending[id] = replyCh
	r.mu.Unlock()

	payload := encodeRPC(id, encoded)
	if err := r.pub.WriteCtx(ctx, payload); err != nil {
		r.mu.Lock()
		delete(r.pending, id)
		r.mu.Unlock()
		return zero, fmt.Errorf("rpc: send request: %w", err)
	}
	select {
	case s := <-replyCh:
		_, repPayload, decErr := decodeRPC(s.Payload)
		if decErr != nil {
			return zero, fmt.Errorf("rpc: decode reply header: %w", decErr)
		}
		v, unmErr := r.repCodec.Unmarshal(repPayload)
		if unmErr != nil {
			return zero, fmt.Errorf("rpc: unmarshal reply: %w", unmErr)
		}
		return v, nil
	case <-ctx.Done():
		r.mu.Lock()
		delete(r.pending, id)
		r.mu.Unlock()
		return zero, fmt.Errorf("%w: %v", ErrNoReply, ctx.Err())
	case <-r.done:
		return zero, fmt.Errorf("rpc: %w", dds.ErrClosed)
	}
}

// Close shuts down the requester and releases DDS resources.
func (r *Requester[Req, Rep]) Close() error {
	r.closeOnce.Do(func() { close(r.done) })
	_ = r.pub.Close()
	_ = r.sub.Close()
	return nil
}

// Replier receives typed RPC requests and dispatches correlated replies.
// Create with NewReplier; call Close when done.
type Replier[Req, Rep any] struct {
	pub       dds.Publisher
	sub       dds.Subscriber
	reqCodec  dds.Codec[Req]
	repCodec  dds.Codec[Rep]
	requests  chan Request[Req]
	done      chan struct{}
	closeOnce sync.Once
}

// NewReplier creates a Replier on the given participant.
// It listens on <topic>/request and replies on <topic>/reply.
func NewReplier[Req, Rep any](
	p dds.Participant,
	topic string,
	reqCodec dds.Codec[Req],
	repCodec dds.Codec[Rep],
	qos dds.QoS,
) (*Replier[Req, Rep], error) {
	sub, err := p.NewSubscriber(topic+"/request", qos)
	if err != nil {
		return nil, fmt.Errorf("rpc: replier subscriber: %w", err)
	}
	pub, err := p.NewPublisher(topic+"/reply", qos)
	if err != nil {
		_ = sub.Close()
		return nil, fmt.Errorf("rpc: replier publisher: %w", err)
	}
	rl := &Replier[Req, Rep]{
		pub:      pub,
		sub:      sub,
		reqCodec: reqCodec,
		repCodec: repCodec,
		requests: make(chan Request[Req], 64),
		done:     make(chan struct{}),
	}
	go rl.pump()
	return rl, nil
}

func (rl *Replier[Req, Rep]) pump() {
	defer close(rl.requests)
	for {
		select {
		case s, ok := <-rl.sub.C():
			if !ok {
				return
			}
			id, payload, err := decodeRPC(s.Payload)
			if err != nil {
				continue
			}
			v, err := rl.reqCodec.Unmarshal(payload)
			if err != nil {
				continue
			}
			select {
			case rl.requests <- Request[Req]{ID: id, Value: v}:
			case <-rl.done:
				return
			}
		case <-rl.done:
			return
		}
	}
}

// Requests returns the channel on which decoded RPC requests are delivered.
// Iterate with range or a select; close is handled by Close.
func (rl *Replier[Req, Rep]) Requests() <-chan Request[Req] { return rl.requests }

// Reply sends a reply correlated to req. ctx controls the write deadline.
func (rl *Replier[Req, Rep]) Reply(ctx context.Context, req Request[Req], rep Rep) error {
	encoded, err := rl.repCodec.Marshal(rep)
	if err != nil {
		return fmt.Errorf("rpc: encode reply: %w", err)
	}
	if err := rl.pub.WriteCtx(ctx, encodeRPC(req.ID, encoded)); err != nil {
		return fmt.Errorf("rpc: send reply: %w", err)
	}
	return nil
}

// Close stops the pump goroutine and releases DDS resources.
func (rl *Replier[Req, Rep]) Close() error {
	rl.closeOnce.Do(func() { close(rl.done) })
	_ = rl.sub.Close()
	_ = rl.pub.Close()
	return nil
}
