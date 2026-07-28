// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package rtps

//fusa:req REQ-TRANS-010
//fusa:req REQ-TRANS-011

// RTPS-over-Relay transport (Milestone 15, "NAT Traversal / Cloud Gateway").
//
// RTPS-over-TCP and RTPS-over-DTLS (Milestone 14) both still require one
// side of a connection to accept an inbound connection at a reachable
// address, which is exactly what most NATs and firewalls prevent. WithRelay
// solves the case where neither side is reachable at all: the participant
// makes a single outbound TLS connection to a relay server (bridge/relay,
// in the sibling bridge module — see its package doc comment for the wire
// protocol and design rationale) and registers under its own GUID prefix,
// hex-encoded, as its relay ID. Once registered, any other participant that
// knows this one's relay ID (learned via SPDP over the relay — see
// WithRelayPeers and pidRelayID) can reach it by asking the relay server to
// forward frames, with no direct connectivity required in either direction.
//
// This file implements only the client side of the bridge/relay wire
// protocol, independently of that package — the same "independent
// length-prefixed framing on each side" precedent transport_tcp.go and
// bridge/wan already set, and required here specifically so this root
// module does not gain a dependency on the bridge submodule (ROADMAP.md,
// "Architecture Initiative", #71: submodules depend on root, never the
// reverse).
//
// The relay never sees DDS payload plaintext beyond what any other
// transport already exposes at the RTPS-message level: it forwards whatever
// bytes wrapInRTPSMessage produced, exactly like RTPS-over-TCP/DTLS do, so
// payload-level confidentiality still comes entirely from WithSecurity (DDS
// payload encryption) — the relay hop adds no additional exposure.

import (
	"context"
	"crypto/tls"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"sync"
	"time"
)

// relayMaxFrameBytes bounds a single relay frame, matching
// bridge/relay.maxFrameBytes.
const relayMaxFrameBytes uint32 = 16 * 1024 * 1024

// relayMaxIDBytes caps an ID field, matching bridge/relay.maxIDBytes.
const relayMaxIDBytes uint16 = 4096

// relayDialTimeout bounds how long the initial connection to the relay
// server may take.
const relayDialTimeout = 5 * time.Second

// Relay frame type tags — must match bridge/relay's frameRegister/
// frameSend/frameDeliver/frameError exactly; the two packages implement the
// same protocol independently (see this file's package doc comment).
const (
	relayFrameRegister byte = 0x01
	relayFrameSend     byte = 0x10
	relayFrameDeliver  byte = 0x11
	relayFrameError    byte = 0x30
)

// relayPacket is a message delivered to us via the relay, together with the
// relay ID of whichever peer sent it.
type relayPacket struct {
	data   []byte
	fromID string
}

// relaySocket is a single persistent client connection to a relay server.
// Unlike tcpSocket/dtlsSocket, which each cache one connection per peer,
// relaySocket has exactly one underlying connection (to the relay itself)
// multiplexing traffic to/from every other participant registered there.
type relaySocket struct {
	id   string // this participant's own relay registration ID
	conn net.Conn
	wmu  sync.Mutex // serialises writes to conn

	recv chan relayPacket
	done chan struct{}
	wg   sync.WaitGroup

	closeOnce sync.Once
}

// newRelaySocket dials addr (TLS-wrapped when tlsConfig is non-nil),
// registers as id, and starts a background read loop delivering inbound
// frames on the returned socket's recv channel.
func newRelaySocket(addr, id string, tlsConfig *tls.Config) (*relaySocket, error) {
	if len(id) > int(relayMaxIDBytes) {
		return nil, fmt.Errorf("rtps: relay id too large: %d bytes", len(id))
	}
	cfg := tlsConfig
	if cfg != nil && cfg.MinVersion == 0 {
		clone := cfg.Clone()
		clone.MinVersion = tls.VersionTLS13
		cfg = clone
	}

	ctx, cancel := context.WithTimeout(context.Background(), relayDialTimeout)
	defer cancel()
	dialer := &net.Dialer{}
	var conn net.Conn
	var err error
	if cfg != nil {
		conn, err = (&tls.Dialer{NetDialer: dialer, Config: cfg}).DialContext(ctx, "tcp", addr)
	} else {
		conn, err = dialer.DialContext(ctx, "tcp", addr)
	}
	if err != nil {
		return nil, fmt.Errorf("rtps: relay dial %s: %w", addr, err)
	}

	if err := relayWriteFrame(conn, relayFrameRegister, []byte(id), nil); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("rtps: relay register: %w", err)
	}

	s := &relaySocket{
		id:   id,
		conn: conn,
		recv: make(chan relayPacket, 256),
		done: make(chan struct{}),
	}
	s.wg.Add(1)
	go s.readLoop()
	return s, nil
}

// send frames data as a SEND frame addressed to targetID.
func (s *relaySocket) send(targetID string, data []byte) error {
	s.wmu.Lock()
	defer s.wmu.Unlock()
	return relayWriteFrame(s.conn, relayFrameSend, []byte(targetID), data)
}

// close shuts down the relay connection and waits for the read loop to
// exit. Safe to call multiple times.
func (s *relaySocket) close() {
	s.closeOnce.Do(func() {
		close(s.done)
		_ = s.conn.Close()
	})
	s.wg.Wait()
}

// readLoop decodes DELIVER frames from the relay connection and pushes them
// onto recv; ERROR frames are dropped (best-effort logging is the caller's
// concern — matching bridge/wan and transport_tcp.go, which likewise drop
// on any framing error rather than surfacing it per-message).
func (s *relaySocket) readLoop() {
	defer s.wg.Done()
	defer close(s.recv)
	for {
		typ, idField, payload, err := relayReadFrame(s.conn)
		if err != nil {
			return
		}
		if typ != relayFrameDeliver || len(idField) == 0 {
			continue // ERROR frame or anything unexpected; ignore
		}
		select {
		case s.recv <- relayPacket{data: payload, fromID: string(idField)}:
		case <-s.done:
			return
		default: // slow consumer; drop
		}
	}
}

// relayIDFromGuidPrefix returns this transport's canonical relay
// registration ID for a participant's GUID prefix: the lowercase hex
// encoding of its 12 bytes. Both WithRelayAddr (registration) and the
// per-message dispatch loop (deriving a sender's GuidPrefix back from the
// relay's DELIVER frame, see dispatchRelayPacket) use this same mapping.
func relayIDFromGuidPrefix(prefix GuidPrefix) string {
	return hex.EncodeToString(prefix[:])
}

// guidPrefixFromRelayID reverses relayIDFromGuidPrefix. Returns false if id
// is not a valid hex encoding of exactly 12 bytes (e.g. a relay peer with a
// custom, non-GuidPrefix-derived ID — no GuidPrefix can be recovered, so
// per-message sender identification silently falls back to whatever the
// RTPS message header itself carries).
func guidPrefixFromRelayID(id string) (GuidPrefix, bool) {
	var prefix GuidPrefix
	b, err := hex.DecodeString(id)
	if err != nil || len(b) != len(prefix) {
		return GuidPrefix{}, false
	}
	copy(prefix[:], b)
	return prefix, true
}

// ── frame encoding (matches bridge/relay's wire format) ─────────────────────────

func relayWriteFrame(w io.Writer, typ byte, idField, payload []byte) error {
	if len(idField) > int(relayMaxIDBytes) {
		return fmt.Errorf("rtps: relay id field too large: %d bytes", len(idField))
	}
	body := make([]byte, 1+2+len(idField)+len(payload))
	body[0] = typ
	binary.BigEndian.PutUint16(body[1:3], uint16(len(idField)))
	copy(body[3:], idField)
	copy(body[3+len(idField):], payload)
	if uint32(len(body)) > relayMaxFrameBytes {
		return fmt.Errorf("rtps: relay frame too large: %d bytes", len(body))
	}
	var hdr [4]byte
	binary.BigEndian.PutUint32(hdr[:], uint32(len(body)))
	if _, err := w.Write(hdr[:]); err != nil {
		return err
	}
	_, err := w.Write(body)
	return err
}

func relayReadFrame(r io.Reader) (typ byte, idField, payload []byte, err error) {
	var hdr [4]byte
	if _, err = io.ReadFull(r, hdr[:]); err != nil {
		return 0, nil, nil, err
	}
	n := binary.BigEndian.Uint32(hdr[:])
	if n == 0 || n > relayMaxFrameBytes {
		return 0, nil, nil, fmt.Errorf("rtps: relay frame too large: %d bytes", n)
	}
	body := make([]byte, n)
	if _, err = io.ReadFull(r, body); err != nil {
		return 0, nil, nil, err
	}
	if len(body) < 3 {
		return 0, nil, nil, fmt.Errorf("rtps: relay short frame: %d bytes", len(body))
	}
	typ = body[0]
	idLen := binary.BigEndian.Uint16(body[1:3])
	if int(idLen) > len(body)-3 {
		return 0, nil, nil, fmt.Errorf("rtps: relay id field length %d exceeds frame", idLen)
	}
	idField = body[3 : 3+int(idLen)]
	payload = body[3+int(idLen):]
	return typ, idField, payload, nil
}
