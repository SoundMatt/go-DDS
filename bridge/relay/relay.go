// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Package relay provides a TURN-style relay server for go-DDS NAT traversal
// (ROADMAP.md, Milestone 15 "Cloud-Native Runtime", "NAT Traversal / Cloud
// Gateway").
//
// Two RTPS participants that both sit behind restrictive NATs or firewalls
// generally cannot open a direct UDP/TCP connection to each other — the
// existing RTPS-over-TCP and RTPS-over-DTLS transports (Milestone 14) both
// still require one side to accept an inbound connection at a reachable
// address. The relay in this package solves the case where neither side is
// reachable at all: every participant makes a single outbound TLS
// connection to a relay server running somewhere reachable by both (a
// public cloud VM, for instance — hence "cloud gateway"), registers under a
// stable ID, and the relay forwards opaque length-prefixed frames between
// any two registered IDs. This is the same role a TURN server plays for
// WebRTC media (RFC 5766), simplified to an application-layer ID-addressed
// relay rather than a raw UDP allocation, since go-DDS participants already
// speak a connection-oriented, length-prefixed RTPS-over-TCP wire format
// (see the root module's rtps.WithTCPAddr) that an ID-addressed relay can
// forward byte-for-byte without modification.
//
// # Wire protocol
//
// A client connects (optionally over TLS — see [Options.TLS]) and sends
// exactly one control frame to register:
//
//	REGISTER  = frameHeader{type: 0x01} + idLen(2) + id
//
// Thereafter it may send any number of data frames addressed to another
// registered ID:
//
//	SEND      = frameHeader{type: 0x10} + idLen(2) + targetID + payload
//
// The server forwards each SEND as a DELIVER frame to the target's
// connection, substituting the sender's ID so the recipient knows who it
// came from:
//
//	DELIVER   = frameHeader{type: 0x11} + idLen(2) + sourceID + payload
//
// If the target ID is not currently registered, the server replies to the
// sender with an ERROR frame instead of silently dropping the frame:
//
//	ERROR     = frameHeader{type: 0x30} + message
//
// Every frame (including REGISTER/SEND/DELIVER/ERROR) is wrapped in a
// 4-byte big-endian length prefix around the frame body, exactly like the
// existing bridge/wan and rtps RTPS-over-TCP framing — see [maxFrameBytes]
// for the size cap. This package only ever reads the 1-byte frame type and
// the (short, size-capped) ID fields to decide where to forward a frame; it
// never parses, inspects, or decrypts the payload itself, so a relay
// operator that does not also control the participants' DDS-Security keys
// (see the root module's rtps.WithSecurity) never has access to plaintext
// DDS payload data — end-to-end payload confidentiality is preserved
// through the relay hop, satisfying this milestone's "the relay must never
// decrypt DDS payload" requirement. [Options.TLS] additionally secures the
// two transport-level hops (client-to-relay) themselves.
//
// The root module's rtps package (rtps.WithRelayAddr) implements the
// client side of this same protocol independently, without importing this
// package — the same "independent length-prefixed framing on each side"
// precedent already set by bridge/wan and rtps/transport_tcp.go, and
// required here specifically so the root module does not gain a dependency
// on this submodule (ROADMAP.md, "Architecture Initiative", #71: submodules
// depend on root, never the reverse).
//
// See also [Discover] for STUN-based public-address discovery, used for
// cloud↔edge peer pairing alongside (not instead of) the relay: a
// participant that can determine its own server-reflexive address via STUN
// can advertise it for direct connection attempts, falling back to this
// relay when direct connectivity truly is not possible — the standard
// STUN/TURN usage pattern this milestone's title reflects.
package relay

//fusa:req REQ-RELAY-001
//fusa:req REQ-RELAY-002
//fusa:req REQ-RELAY-003
//fusa:req REQ-RELAY-004

import (
	"context"
	"crypto/tls"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
)

// maxFrameBytes bounds a single relay frame (registration ID, or SEND/DELIVER
// payload) to guard against a malicious or malformed peer forcing an
// unbounded allocation. Set to the same 16 MiB ceiling bridge/wan uses.
const maxFrameBytes uint32 = 16 * 1024 * 1024

// maxIDBytes caps a registration or target ID so a hostile peer cannot force
// a large allocation before the server even knows who it is.
const maxIDBytes uint16 = 4096

// Frame type tags. See the package doc comment for the wire format.
const (
	frameRegister byte = 0x01
	frameSend     byte = 0x10
	frameDeliver  byte = 0x11
	frameError    byte = 0x30
)

// ErrFrameTooLarge is returned when an incoming frame exceeds maxFrameBytes.
var ErrFrameTooLarge = errors.New("relay: frame too large")

// ErrUnknownFrameType is returned when a frame's type tag is not one this
// package understands.
var ErrUnknownFrameType = errors.New("relay: unknown frame type")

// ErrNoSuchPeer is the message text carried in an ERROR frame sent back to a
// client whose SEND named a target ID that is not currently registered.
const ErrNoSuchPeer = "relay: no such peer registered"

// Options configures a relay [Server].
type Options struct {
	// TLS, when non-nil, wraps every accepted connection in TLS (must carry
	// a certificate). Nil leaves the listener as plain TCP — acceptable for
	// local development and testing, but production deployments should
	// always set this: the relay channel must be TLS-secured per this
	// milestone's requirements.
	TLS *tls.Config
}

// clientConn is one registered client's connection, guarded by its own
// write mutex so concurrent SEND/DELIVER forwards never interleave frames.
type clientConn struct {
	id   string
	conn net.Conn
	wmu  sync.Mutex
}

func (c *clientConn) writeFrame(typ byte, idField, payload []byte) error {
	c.wmu.Lock()
	defer c.wmu.Unlock()
	return writeFrame(c.conn, typ, idField, payload)
}

// Server is a TURN-style relay server. Create one with [Serve].
//
// Server is safe for concurrent use from multiple goroutines (every accepted
// connection runs its own read loop).
type Server struct {
	ln net.Listener

	mu      sync.RWMutex
	clients map[string]*clientConn

	done chan struct{}
	once sync.Once
	wg   sync.WaitGroup
}

// Serve starts a relay server listening on addr (TLS-wrapped when
// opts.TLS is non-nil) and returns immediately; the accept loop runs in a
// background goroutine. Use [Server.Addr] to discover the actual bound
// address when addr uses port 0.
func Serve(addr string, opts Options) (*Server, error) {
	ln, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("relay: listen %s: %w", addr, err)
	}
	if opts.TLS != nil {
		cfg := opts.TLS
		if cfg.MinVersion == 0 {
			clone := cfg.Clone()
			clone.MinVersion = tls.VersionTLS13
			cfg = clone
		}
		ln = tls.NewListener(ln, cfg)
	}
	s := &Server{
		ln:      ln,
		clients: make(map[string]*clientConn),
		done:    make(chan struct{}),
	}
	s.wg.Add(1)
	go s.acceptLoop()
	return s, nil
}

// Addr returns the TCP address the server is listening on.
func (s *Server) Addr() string {
	return s.ln.Addr().String()
}

// Peers returns the IDs currently registered with the relay. Intended for
// diagnostics/ops (e.g. an operator health page), not for routing decisions.
func (s *Server) Peers() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	ids := make([]string, 0, len(s.clients))
	for id := range s.clients {
		ids = append(ids, id)
	}
	return ids
}

// Close stops accepting new connections, closes every registered client
// connection, and waits for all goroutines to exit. Safe to call multiple
// times.
func (s *Server) Close() error {
	s.once.Do(func() {
		close(s.done)
		_ = s.ln.Close()
		s.mu.Lock()
		for _, c := range s.clients {
			_ = c.conn.Close()
		}
		s.clients = make(map[string]*clientConn)
		s.mu.Unlock()
	})
	s.wg.Wait()
	return nil
}

func (s *Server) acceptLoop() {
	defer s.wg.Done()
	for {
		conn, err := s.ln.Accept()
		if err != nil {
			select {
			case <-s.done:
				return
			default:
				continue
			}
		}
		s.wg.Add(1)
		go s.handleConn(conn)
	}
}

// handleConn reads the mandatory REGISTER frame, publishes the client into
// the registry, and then relays SEND frames until the connection closes.
func (s *Server) handleConn(conn net.Conn) {
	defer s.wg.Done()
	defer func() { _ = conn.Close() }()

	typ, idField, _, err := readFrame(conn)
	if err != nil || typ != frameRegister || len(idField) == 0 {
		return // no valid registration; drop the connection
	}
	id := string(idField)

	c := &clientConn{id: id, conn: conn}
	s.register(c)
	defer s.unregister(c)

	for {
		typ, targetID, payload, err := readFrame(conn)
		if err != nil {
			return
		}
		if typ != frameSend || len(targetID) == 0 {
			continue // ignore anything that isn't a well-formed SEND
		}
		s.forward(c, string(targetID), payload)
	}
}

// register publishes c into the client table. A reconnecting client with
// the same ID replaces (and closes) any prior connection for that ID —
// the same "last registration wins" semantics bridge/wan and the RTPS-over-
// TCP transport use for their own per-peer connection caches.
func (s *Server) register(c *clientConn) {
	s.mu.Lock()
	old, existed := s.clients[c.id]
	s.clients[c.id] = c
	s.mu.Unlock()
	if existed {
		_ = old.conn.Close()
	}
}

// unregister removes c from the client table, but only if it is still the
// current registration for its ID (a stale connection that already lost a
// register() race must not evict the newer one on its way out).
func (s *Server) unregister(c *clientConn) {
	s.mu.Lock()
	if cur, ok := s.clients[c.id]; ok && cur == c {
		delete(s.clients, c.id)
	}
	s.mu.Unlock()
}

// forward relays payload from sender to targetID, or sends sender an ERROR
// frame if targetID is not currently registered.
func (s *Server) forward(sender *clientConn, targetID string, payload []byte) {
	s.mu.RLock()
	target, ok := s.clients[targetID]
	s.mu.RUnlock()
	if !ok {
		_ = sender.writeFrame(frameError, nil, []byte(ErrNoSuchPeer))
		return
	}
	_ = target.writeFrame(frameDeliver, []byte(sender.id), payload)
}

// ── frame encoding ────────────────────────────────────────────────────────────

// writeFrame writes [4-byte BE total length][1-byte type][2-byte BE idField
// length][idField][payload] to w.
func writeFrame(w io.Writer, typ byte, idField, payload []byte) error {
	if len(idField) > int(maxIDBytes) {
		return fmt.Errorf("relay: id field too large: %d bytes", len(idField))
	}
	body := make([]byte, 1+2+len(idField)+len(payload))
	body[0] = typ
	binary.BigEndian.PutUint16(body[1:3], uint16(len(idField)))
	copy(body[3:], idField)
	copy(body[3+len(idField):], payload)
	if uint32(len(body)) > maxFrameBytes {
		return fmt.Errorf("%w: %d bytes", ErrFrameTooLarge, len(body))
	}
	var hdr [4]byte
	binary.BigEndian.PutUint32(hdr[:], uint32(len(body)))
	if _, err := w.Write(hdr[:]); err != nil {
		return err
	}
	_, err := w.Write(body)
	return err
}

// readFrame reads one length-prefixed frame from r and splits it into its
// type tag, id field, and remaining payload.
func readFrame(r io.Reader) (typ byte, idField, payload []byte, err error) {
	var hdr [4]byte
	if _, err = io.ReadFull(r, hdr[:]); err != nil {
		return 0, nil, nil, err
	}
	n := binary.BigEndian.Uint32(hdr[:])
	if n == 0 || n > maxFrameBytes {
		return 0, nil, nil, fmt.Errorf("%w: %d bytes", ErrFrameTooLarge, n)
	}
	body := make([]byte, n)
	if _, err = io.ReadFull(r, body); err != nil {
		return 0, nil, nil, err
	}
	if len(body) < 3 {
		return 0, nil, nil, fmt.Errorf("relay: short frame: %d bytes", len(body))
	}
	typ = body[0]
	idLen := binary.BigEndian.Uint16(body[1:3])
	if int(idLen) > len(body)-3 {
		return 0, nil, nil, fmt.Errorf("relay: id field length %d exceeds frame", idLen)
	}
	idField = body[3 : 3+int(idLen)]
	payload = body[3+int(idLen):]
	switch typ {
	case frameRegister, frameSend, frameDeliver, frameError:
	default:
		return 0, nil, nil, ErrUnknownFrameType
	}
	return typ, idField, payload, nil
}
