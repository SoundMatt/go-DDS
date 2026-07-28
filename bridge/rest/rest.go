// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Package rest provides an HTTP/SSE gateway that bridges a DDS participant to
// HTTP clients.
//
// Subscribers receive a real-time Server-Sent Events (SSE) stream over
// GET /topics/{topic}; publishers POST raw bytes to /topics/{topic}; and
// GET /topics lists all currently-subscribed topics as JSON.
//
// Usage:
//
//	p, _ := rtps.New(dds.Domain(0))
//	bridge := rest.New(p, rest.Options{})
//	http.ListenAndServe(":8090", bridge)
package rest

//fusa:req REQ-BRIDGE-003
//fusa:req REQ-BRIDGE-004
//fusa:req REQ-BRIDGE-005

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	dds "github.com/SoundMatt/go-DDS"
)

// Options configures a Bridge.
type Options struct {
	// AuthToken, if non-empty, requires every request to carry the header
	//   Authorization: Bearer <AuthToken>
	// Requests without a valid token receive 401 Unauthorized.
	AuthToken string

	// QoS is applied to all subscribers and publishers created by the bridge.
	// Zero value uses dds.DefaultQoS.
	QoS dds.QoS

	// SSEKeepalive is how often a comment (:keepalive) line is written to idle
	// SSE streams to prevent proxy timeouts. Default: 15s. Zero disables.
	SSEKeepalive time.Duration
}

func (o Options) qos() dds.QoS {
	// dds.QoS gained a []string field (Partition) in Milestone 14 "QoS
	// Enforcement — Active Policy", making it non-comparable with ==;
	// reflect.DeepEqual is the equivalent zero-value check.
	if reflect.DeepEqual(o.QoS, dds.QoS{}) {
		return dds.DefaultQoS
	}
	return o.QoS
}

func (o Options) keepalive() time.Duration {
	if o.SSEKeepalive == 0 {
		return 15 * time.Second
	}
	return o.SSEKeepalive
}

// Bridge exposes a DDS participant over HTTP.
// It is safe for concurrent use.
type Bridge struct {
	p    dds.Participant
	opts Options

	mu   sync.Mutex
	subs map[string]dds.Subscriber // topic -> active subscriber
	pubs map[string]dds.Publisher  // topic -> active publisher

	seq atomic.Uint64
}

// New creates a Bridge wrapping p. Subscribers and publishers are created
// lazily on first request.
func New(p dds.Participant, opts Options) *Bridge {
	return &Bridge{
		p:    p,
		opts: opts,
		subs: make(map[string]dds.Subscriber),
		pubs: make(map[string]dds.Publisher),
	}
}

// Close closes the Bridge and all open subscribers and publishers.
func (b *Bridge) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, sub := range b.subs {
		_ = sub.Close()
	}
	for _, pub := range b.pubs {
		_ = pub.Close()
	}
	b.subs = make(map[string]dds.Subscriber)
	b.pubs = make(map[string]dds.Publisher)
	return nil
}

// ServeHTTP routes incoming requests:
//
//	GET  /topics        → JSON list of subscribed topics
//	GET  /topics/{t}   → SSE stream of samples on topic t
//	POST /topics/{t}   → publish request body as a sample on topic t
func (b *Bridge) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !b.authorize(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/topics")
	if path == "" || path == "/" {
		if r.Method == http.MethodGet {
			b.handleList(w, r)
			return
		}
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	topic := strings.TrimPrefix(path, "/")
	if topic == "" {
		http.Error(w, "topic name required", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodGet:
		b.handleSubscribe(w, r, topic)
	case http.MethodPost:
		b.handlePublish(w, r, topic)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleList returns a JSON array of currently-subscribed topic names.
func (b *Bridge) handleList(w http.ResponseWriter, _ *http.Request) {
	b.mu.Lock()
	topics := make([]string, 0, len(b.subs))
	for t := range b.subs {
		topics = append(topics, t)
	}
	b.mu.Unlock()
	sort.Strings(topics)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(topics)
}

// handleSubscribe opens an SSE stream for the given topic. Each DDS sample is
// sent as:
//
//	id: <monotonic-sequence-number>
//	event: message
//	data: <base64-encoded payload>
//	(blank line)
func (b *Bridge) handleSubscribe(w http.ResponseWriter, r *http.Request, topic string) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	sub, err := b.getOrCreateSub(topic)
	if err != nil {
		http.Error(w, fmt.Sprintf("subscriber: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	var keepalive <-chan time.Time
	if d := b.opts.keepalive(); d > 0 {
		t := time.NewTicker(d)
		defer t.Stop()
		keepalive = t.C
	}

	for {
		select {
		case s, ok := <-sub.C():
			if !ok {
				return
			}
			id := b.seq.Add(1)
			encoded := base64.StdEncoding.EncodeToString(s.Payload)
			writeN, writeErr := fmt.Fprintf(w, "id: %d\nevent: message\ndata: %s\n\n", id, encoded)
			_ = writeN
			if writeErr != nil {
				return
			}
			flusher.Flush()
		case <-keepalive:
			writeN, writeErr := fmt.Fprintf(w, ": keepalive\n\n")
			_ = writeN
			if writeErr != nil {
				return
			}
			flusher.Flush()
		case <-r.Context().Done():
			return
		}
	}
}

// handlePublish reads the request body and publishes it as a DDS sample on topic.
func (b *Bridge) handlePublish(w http.ResponseWriter, r *http.Request, topic string) {
	const maxBody = 16 << 20 // 16 MiB cap
	payload, err := io.ReadAll(io.LimitReader(r.Body, maxBody))
	if err != nil {
		http.Error(w, "read body: "+err.Error(), http.StatusBadRequest)
		return
	}

	pub, err := b.getOrCreatePub(topic)
	if err != nil {
		http.Error(w, "publisher: "+err.Error(), http.StatusInternalServerError)
		return
	}

	if err := pub.WriteCtx(r.Context(), payload); err != nil {
		http.Error(w, "publish: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (b *Bridge) authorize(r *http.Request) bool {
	if b.opts.AuthToken == "" {
		return true
	}
	auth := r.Header.Get("Authorization")
	return auth == "Bearer "+b.opts.AuthToken
}

func (b *Bridge) getOrCreateSub(topic string) (dds.Subscriber, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if sub, ok := b.subs[topic]; ok {
		return sub, nil
	}
	sub, err := b.p.NewSubscriber(topic, b.opts.qos())
	if err != nil {
		return nil, err
	}
	b.subs[topic] = sub
	return sub, nil
}

func (b *Bridge) getOrCreatePub(topic string) (dds.Publisher, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if pub, ok := b.pubs[topic]; ok {
		return pub, nil
	}
	pub, err := b.p.NewPublisher(topic, b.opts.qos())
	if err != nil {
		return nil, err
	}
	b.pubs[topic] = pub
	return pub, nil
}
