// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Package ws provides a WebSocket gateway that bridges a DDS participant to
// WebSocket clients (Milestone 16, ROADMAP.md "WebSocket Transport",
// "bridge/ws/ package: HTTP upgrade handler, participant bridge to RTPS
// domain").
//
// This is a *different*, additional thing from rtps.WithWSAddr (the native
// RTPS-over-WebSocket transport, rtps/transport_ws.go): WithWSAddr lets any
// go-DDS-speaking peer — including a browser or Wasm participant — join a
// domain directly as a genuine RTPS peer performing its own SPDP/SEDP
// discovery, with no bridge at all, which is the literal Milestone 16
// success criterion ("...without a protocol bridge"). This package is for
// the simpler case: a JavaScript/TypeScript client (see js/dds-client) that
// would rather speak a small JSON pub/sub protocol over one WebSocket
// connection than implement RTPS discovery itself — mirroring bridge/rest's
// HTTP/SSE gateway, but bidirectional over a single long-lived connection
// instead of one-way SSE plus separate POSTs.
//
// Usage:
//
//	p, _ := rtps.New(dds.Domain(0))
//	bridge := ws.New(p, ws.Options{})
//	http.ListenAndServe(":8091", bridge)
//
// Wire protocol: after the standard RFC 6455 WebSocket upgrade, every
// message on the connection (in either direction) is a single JSON object.
// This gateway always sends TEXT frames; it accepts either TEXT or BINARY
// frames from the client (the payload is parsed identically as JSON bytes
// either way).
//
// Client -> server:
//
//	{"op":"subscribe","topic":"<name>"}
//	{"op":"unsubscribe","topic":"<name>"}
//	{"op":"publish","topic":"<name>","data":"<base64 payload>"}
//
// Server -> client:
//
//	{"op":"subscribed","topic":"<name>"}
//	{"op":"unsubscribed","topic":"<name>"}
//	{"op":"sample","topic":"<name>","data":"<base64 payload>"}
//	{"op":"error","message":"<text>"}
package ws

//fusa:req REQ-BRIDGE-015
//fusa:req REQ-BRIDGE-016

import (
	"bufio"
	"context"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"reflect"
	"sync"

	dds "github.com/SoundMatt/go-DDS"
)

// Options configures a Bridge.
type Options struct {
	// AuthToken, if non-empty, requires every connecting client to carry
	// the header
	//
	//	Authorization: Bearer <AuthToken>
	//
	// on its HTTP upgrade request. A missing or mismatched token is
	// rejected with 401 Unauthorized before the WebSocket handshake
	// completes — the same policy rest.Options.AuthToken applies.
	AuthToken string

	// QoS is applied to every subscriber and publisher a client's
	// "subscribe"/"publish" messages create. Zero value uses
	// dds.DefaultQoS.
	QoS dds.QoS
}

func (o Options) qos() dds.QoS {
	// dds.QoS carries a []string Partition field, making it non-comparable
	// with ==; reflect.DeepEqual is the equivalent zero-value check — the
	// same pattern rest.Options.qos uses.
	if reflect.DeepEqual(o.QoS, dds.QoS{}) {
		return dds.DefaultQoS
	}
	return o.QoS
}

// clientMessage is the JSON envelope a client sends — see the package doc
// comment's wire protocol.
type clientMessage struct {
	Op    string `json:"op"`
	Topic string `json:"topic"`
	Data  string `json:"data,omitempty"`
}

// serverMessage is the JSON envelope this gateway sends.
type serverMessage struct {
	Op      string `json:"op"`
	Topic   string `json:"topic,omitempty"`
	Data    string `json:"data,omitempty"`
	Message string `json:"message,omitempty"`
}

// Bridge exposes a DDS participant to WebSocket clients speaking this
// package's JSON pub/sub protocol. It is safe for concurrent use, including
// concurrent ServeHTTP calls for multiple simultaneous client connections.
type Bridge struct {
	p    dds.Participant
	opts Options

	mu       sync.Mutex
	sessions map[*session]struct{}
	closed   bool
}

// New creates a Bridge wrapping p. Subscribers and publishers are created
// lazily, per client session, as that client's "subscribe"/"publish"
// messages request them.
func New(p dds.Participant, opts Options) *Bridge {
	return &Bridge{
		p:        p,
		opts:     opts,
		sessions: make(map[*session]struct{}),
	}
}

// Close closes every currently-connected client session (and, transitively,
// every subscriber/publisher those sessions created). It does not close the
// underlying participant p — the caller owns that, exactly as
// rest.Bridge.Close leaves its participant open.
func (b *Bridge) Close() error {
	b.mu.Lock()
	b.closed = true
	sessions := make([]*session, 0, len(b.sessions))
	for s := range b.sessions {
		sessions = append(sessions, s)
	}
	b.mu.Unlock()

	for _, s := range sessions {
		s.close()
	}
	return nil
}

// ServeHTTP implements http.Handler: it performs the RFC 6455 opening
// handshake (after enforcing AuthToken, if configured) and then runs the
// client's session until the connection closes. It never returns until the
// session ends, matching the long-lived-connection contract callers already
// expect from rest.Bridge's SSE handler.
func (b *Bridge) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !b.authorize(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	key := r.Header.Get("Sec-WebSocket-Key")
	if key == "" || !headerContainsToken(r.Header.Get("Upgrade"), "websocket") ||
		!headerContainsToken(r.Header.Get("Connection"), "upgrade") {
		http.Error(w, "expected a WebSocket upgrade request", http.StatusBadRequest)
		return
	}

	hj, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}
	nc, rw, err := hj.Hijack()
	if err != nil {
		http.Error(w, fmt.Sprintf("hijack: %v", err), http.StatusInternalServerError)
		return
	}
	// From here on, nc's lifetime is owned by the session created below
	// (see run's defer s.close(), and Bridge.Close for the
	// externally-triggered case) — no defer nc.Close() here, to avoid a
	// double-close race with session.close() running concurrently from
	// Bridge.Close.

	resp := "HTTP/1.1 101 Switching Protocols\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Accept: " + acceptKey(key) + "\r\n\r\n"
	if _, err := rw.WriteString(resp); err != nil {
		_ = nc.Close()
		return
	}
	if err := rw.Flush(); err != nil {
		_ = nc.Close()
		return
	}

	s := newSession(b, nc, rw.Reader)
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		_ = nc.Close()
		return
	}
	b.sessions[s] = struct{}{}
	b.mu.Unlock()

	s.run() // blocks until the client disconnects or Close is called

	b.mu.Lock()
	delete(b.sessions, s)
	b.mu.Unlock()
}

// authorize reports whether r carries the configured AuthToken, checked
// either as a standard "Authorization: Bearer <token>" header (the
// rest.Options.AuthToken convention, and what any non-browser client — a
// cloud function, a Node.js client, go-DDS's own tooling — can set freely)
// or, if that header is absent, as a "?token=<token>" query parameter. The
// query-parameter fallback exists because the browser WebSocket API
// (unlike XHR/fetch) provides no way for page JavaScript to set arbitrary
// request headers on the opening handshake — see js/dds-client's README —
// so a header-only check would make AuthToken entirely unusable from an
// actual browser tab, undermining this milestone's own success criterion.
// Both comparisons are constant-time (crypto/subtle) to avoid a timing
// side-channel on the token value.
func (b *Bridge) authorize(r *http.Request) bool {
	if b.opts.AuthToken == "" {
		return true
	}
	want := []byte(b.opts.AuthToken)
	if got := r.Header.Get("Authorization"); got != "" {
		return subtle.ConstantTimeCompare([]byte(got), []byte("Bearer "+b.opts.AuthToken)) == 1
	}
	if got := r.URL.Query().Get("token"); got != "" {
		return subtle.ConstantTimeCompare([]byte(got), want) == 1
	}
	return false
}

// ── session ──────────────────────────────────────────────────────────────────

// session is one connected client's state: its WebSocket connection plus
// the subscribers/publishers its messages have created so far.
type session struct {
	b  *Bridge
	nc net.Conn
	br *bufio.Reader
	// writeMu serialises writes to nc: writeMessage is called both from
	// run's own read/dispatch loop (replies, "sample" pushes triggered
	// synchronously) and concurrently from each topic's forwardSamples
	// goroutine.
	writeMu sync.Mutex

	mu     sync.Mutex
	closed bool
	done   chan struct{} // closed once, by close(), to stop every forwardSamples goroutine
	subs   map[string]dds.Subscriber
	pubs   map[string]dds.Publisher
}

func newSession(b *Bridge, nc net.Conn, br *bufio.Reader) *session {
	return &session{
		b:    b,
		nc:   nc,
		br:   br,
		done: make(chan struct{}),
		subs: make(map[string]dds.Subscriber),
		pubs: make(map[string]dds.Publisher),
	}
}

// run reads and dispatches client messages until the connection errors,
// closes, or sends a Close frame. It always cleans up every subscriber/
// publisher this session created before returning.
func (s *session) run() {
	defer s.close()
	for {
		opcode, payload, err := readMessage(s.br, s.writer())
		if err != nil {
			return
		}
		if opcode == opClose {
			return
		}
		if opcode != opText && opcode != opBinary {
			continue
		}
		var msg clientMessage
		if err := json.Unmarshal(payload, &msg); err != nil {
			s.sendError(fmt.Sprintf("invalid JSON: %v", err))
			continue
		}
		s.dispatch(msg)
	}
}

// writer returns an io.Writer that serialises through writeMu — used for
// readMessage's own transparent Pong replies, which must not interleave
// with a concurrent application-level write.
func (s *session) writer() *lockedWriter {
	return &lockedWriter{s: s}
}

// lockedWriter routes writes through session.writeMu so control-frame
// replies (Pong) and application messages never interleave on the wire.
type lockedWriter struct{ s *session }

func (lw *lockedWriter) Write(p []byte) (int, error) {
	lw.s.writeMu.Lock()
	defer lw.s.writeMu.Unlock()
	return lw.s.nc.Write(p)
}

func (s *session) dispatch(msg clientMessage) {
	switch msg.Op {
	case "subscribe":
		s.handleSubscribe(msg.Topic)
	case "unsubscribe":
		s.handleUnsubscribe(msg.Topic)
	case "publish":
		s.handlePublish(msg.Topic, msg.Data)
	default:
		s.sendError(fmt.Sprintf("unknown op %q", msg.Op))
	}
}

func (s *session) handleSubscribe(topic string) {
	if topic == "" {
		s.sendError("subscribe: topic required")
		return
	}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	if _, exists := s.subs[topic]; exists {
		s.mu.Unlock()
		s.send(serverMessage{Op: "subscribed", Topic: topic})
		return
	}
	sub, err := s.b.p.NewSubscriber(topic, s.b.opts.qos())
	if err != nil {
		s.mu.Unlock()
		s.sendError(fmt.Sprintf("subscribe %q: %v", topic, err))
		return
	}
	s.subs[topic] = sub
	s.mu.Unlock()

	go s.forwardSamples(topic, sub)
	s.send(serverMessage{Op: "subscribed", Topic: topic})
}

func (s *session) handleUnsubscribe(topic string) {
	s.mu.Lock()
	sub, ok := s.subs[topic]
	if ok {
		delete(s.subs, topic)
	}
	s.mu.Unlock()
	if ok {
		_ = sub.Close()
	}
	s.send(serverMessage{Op: "unsubscribed", Topic: topic})
}

func (s *session) handlePublish(topic, data string) {
	if topic == "" {
		s.sendError("publish: topic required")
		return
	}
	payload, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		s.sendError(fmt.Sprintf("publish %q: invalid base64 data: %v", topic, err))
		return
	}

	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	pub, ok := s.pubs[topic]
	if !ok {
		var err error
		pub, err = s.b.p.NewPublisher(topic, s.b.opts.qos())
		if err != nil {
			s.mu.Unlock()
			s.sendError(fmt.Sprintf("publish %q: %v", topic, err))
			return
		}
		s.pubs[topic] = pub
	}
	s.mu.Unlock()

	if err := pub.WriteCtx(context.Background(), payload); err != nil {
		s.sendError(fmt.Sprintf("publish %q: %v", topic, err))
	}
}

// forwardSamples pushes every sample received on sub as a "sample" message
// until sub's channel closes or the session ends.
func (s *session) forwardSamples(topic string, sub dds.Subscriber) {
	for {
		select {
		case sample, ok := <-sub.C():
			if !ok {
				return
			}
			s.send(serverMessage{
				Op:    "sample",
				Topic: topic,
				Data:  base64.StdEncoding.EncodeToString(sample.Payload),
			})
		case <-s.done:
			return
		}
	}
}

func (s *session) sendError(text string) {
	s.send(serverMessage{Op: "error", Message: text})
}

// send JSON-encodes msg and writes it as a single WebSocket TEXT frame,
// serialised against every other writer of this connection. Errors are
// intentionally swallowed here — a write failure means the connection is
// going away, and run's own read loop (or a subsequent write attempt) will
// discover that and tear the session down; there is nothing more useful to
// do with the error at a send call site deep inside sample forwarding.
func (s *session) send(msg serverMessage) {
	b, err := json.Marshal(msg)
	if err != nil {
		return
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	_ = writeMessage(s.nc, b)
}

// close tears down every subscriber/publisher this session created and
// closes the underlying connection. Safe to call more than once (e.g. once
// from run's own defer and once from Bridge.Close iterating active
// sessions).
func (s *session) close() {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	subs := make([]dds.Subscriber, 0, len(s.subs))
	for _, sub := range s.subs {
		subs = append(subs, sub)
	}
	pubs := make([]dds.Publisher, 0, len(s.pubs))
	for _, pub := range s.pubs {
		pubs = append(pubs, pub)
	}
	s.subs = nil
	s.pubs = nil
	s.mu.Unlock()

	close(s.done)
	for _, sub := range subs {
		_ = sub.Close()
	}
	for _, pub := range pubs {
		_ = pub.Close()
	}
	_ = s.nc.Close()
}
