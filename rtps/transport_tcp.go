// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package rtps

//fusa:req REQ-TRANS-004
//fusa:req REQ-TRANS-005
//fusa:req REQ-TRANS-006

// RTPS-over-TCP transport (Milestone 14, ROADMAP.md "TCP/TLS (RTPS over
// TCP)"). It carries the exact same RTPS message bytes wrapInRTPSMessage
// produces for UDP, framed with a 4-byte big-endian length prefix so message
// boundaries survive TCP's byte-stream semantics — UDP datagrams are
// naturally boundary-preserving, TCP is not.
//
// TLS 1.3 wrapping uses only crypto/tls from the standard library — no
// external dependency — via WithTCPTLSConfig.

import (
	"context"
	"crypto/tls"
	"encoding/binary"
	"fmt"
	"net"
	"sync"
	"time"
)

// maxTCPFrameSize bounds a single RTPS-over-TCP frame to guard against a
// corrupt or hostile peer claiming an unbounded length prefix. It is set well
// above maxUDPSize since TCP has no datagram-size ceiling of its own.
const maxTCPFrameSize = 1 << 20 // 1 MiB

// tcpDialTimeout bounds how long an outbound connection attempt may block.
const tcpDialTimeout = 5 * time.Second

// tcpPacket is a received RTPS-over-TCP message together with the sender's
// address. Unlike udpPacket, from is a "host:port" string: a TCP connection
// has no equivalent of net.UDPAddr's per-datagram sender metadata.
type tcpPacket struct {
	data []byte
	from string
}

// tcpSocket is the TCP analogue of udpSocket: it accepts inbound connections
// on a listen address and dials outbound connections to peers on demand,
// caching one connection per peer address for reuse. Every message, inbound
// or outbound, is framed with a 4-byte big-endian length prefix around a
// single RTPS message.
type tcpSocket struct {
	ln        net.Listener
	tlsConfig *tls.Config // nil = plaintext TCP
	port      int

	recv chan tcpPacket
	done chan struct{}

	mu    sync.Mutex
	conns map[string]net.Conn    // peer addr -> cached outbound/inbound connection
	wmu   map[string]*sync.Mutex // per-peer lock: serialises writes and dials
}

// newTCPSocket starts listening on addr (TLS-wrapped when tlsConfig is
// non-nil) and returns a socket ready to accept inbound connections and dial
// outbound ones. When tlsConfig is supplied with MinVersion unset, TLS 1.3 is
// enforced.
func newTCPSocket(addr string, tlsConfig *tls.Config) (*tcpSocket, error) {
	cfg := tlsConfig
	if cfg != nil && cfg.MinVersion == 0 {
		clone := cfg.Clone()
		clone.MinVersion = tls.VersionTLS13
		cfg = clone
	}
	var ln net.Listener
	var err error
	if cfg != nil {
		ln, err = tls.Listen("tcp", addr, cfg)
	} else {
		ln, err = (&net.ListenConfig{}).Listen(context.Background(), "tcp", addr)
	}
	if err != nil {
		return nil, fmt.Errorf("rtps: TCP listen %s: %w", addr, err)
	}
	port := 0
	if tcpAddr, ok := ln.Addr().(*net.TCPAddr); ok {
		port = tcpAddr.Port
	}
	s := &tcpSocket{
		ln:        ln,
		tlsConfig: cfg,
		port:      port,
		recv:      make(chan tcpPacket, 256),
		done:      make(chan struct{}),
		conns:     make(map[string]net.Conn),
		wmu:       make(map[string]*sync.Mutex),
	}
	go s.acceptLoop()
	return s, nil
}

// acceptLoop accepts inbound connections and spawns a read loop for each.
func (s *tcpSocket) acceptLoop() {
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
		go s.readConn(conn, conn.RemoteAddr().String())
	}
}

// readConn frame-decodes messages from conn until it errors or closes,
// pushing each onto recv.
func (s *tcpSocket) readConn(conn net.Conn, from string) {
	defer func() { _ = conn.Close() }()
	lenBuf := make([]byte, 4)
	for {
		if err := readFull(conn, lenBuf); err != nil {
			return
		}
		n := binary.BigEndian.Uint32(lenBuf)
		if n == 0 || n > maxTCPFrameSize {
			return // malformed or hostile frame; drop the connection
		}
		data := make([]byte, n)
		if err := readFull(conn, data); err != nil {
			return
		}
		select {
		case s.recv <- tcpPacket{data: data, from: from}:
		case <-s.done:
			return
		default: // slow consumer; drop
		}
	}
}

// readFull reads exactly len(buf) bytes from conn, like io.ReadFull.
func readFull(conn net.Conn, buf []byte) error {
	for total := 0; total < len(buf); {
		n, err := conn.Read(buf[total:])
		total += n
		if err != nil {
			return err
		}
	}
	return nil
}

// send frames data with a 4-byte length prefix and writes it to addr,
// dialling (or reusing a cached connection to) addr as needed. Writes to the
// same peer are serialised so concurrent senders never interleave frames, and
// a failed write drops the cached connection so the next send redials.
func (s *tcpSocket) send(addr string, data []byte) error {
	if len(data) > maxTCPFrameSize {
		return fmt.Errorf("rtps: TCP frame too large: %d bytes", len(data))
	}
	wmu := s.peerLock(addr)
	wmu.Lock()
	defer wmu.Unlock()

	conn, err := s.connLocked(addr)
	if err != nil {
		return err
	}
	frame := make([]byte, 4+len(data))
	binary.BigEndian.PutUint32(frame[:4], uint32(len(data)))
	copy(frame[4:], data)
	if _, err := conn.Write(frame); err != nil {
		s.dropConn(addr)
		return fmt.Errorf("rtps: TCP write %s: %w", addr, err)
	}
	return nil
}

// peerLock returns (creating on first use) the per-peer mutex for addr.
func (s *tcpSocket) peerLock(addr string) *sync.Mutex {
	s.mu.Lock()
	defer s.mu.Unlock()
	wmu, ok := s.wmu[addr]
	if !ok {
		wmu = &sync.Mutex{}
		s.wmu[addr] = wmu
	}
	return wmu
}

// connLocked returns the cached connection for addr, dialling a fresh one if
// none is cached. Caller must hold the lock returned by peerLock(addr), which
// also serialises concurrent dials to the same peer.
func (s *tcpSocket) connLocked(addr string) (net.Conn, error) {
	s.mu.Lock()
	conn, ok := s.conns[addr]
	s.mu.Unlock()
	if ok {
		return conn, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), tcpDialTimeout)
	defer cancel()
	var newConn net.Conn
	var err error
	if s.tlsConfig != nil {
		newConn, err = (&tls.Dialer{Config: s.tlsConfig}).DialContext(ctx, "tcp", addr)
	} else {
		newConn, err = (&net.Dialer{}).DialContext(ctx, "tcp", addr)
	}
	if err != nil {
		return nil, fmt.Errorf("rtps: TCP dial %s: %w", addr, err)
	}

	s.mu.Lock()
	s.conns[addr] = newConn
	s.mu.Unlock()
	go s.readConn(newConn, addr)
	return newConn, nil
}

// dropConn closes and evicts the cached connection to addr, if any.
func (s *tcpSocket) dropConn(addr string) {
	s.mu.Lock()
	conn, ok := s.conns[addr]
	if ok {
		delete(s.conns, addr)
	}
	s.mu.Unlock()
	if ok {
		_ = conn.Close()
	}
}

// close shuts down the listener and every cached connection.
func (s *tcpSocket) close() {
	close(s.done)
	_ = s.ln.Close()
	s.mu.Lock()
	for _, conn := range s.conns {
		_ = conn.Close()
	}
	s.conns = nil
	s.mu.Unlock()
}
