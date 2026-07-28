// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package rtps

//fusa:req REQ-TRANS-007
//fusa:req REQ-TRANS-008
//fusa:req REQ-TRANS-009

// RTPS-over-DTLS transport (Milestone 14, ROADMAP.md "DTLS (Encrypted
// UDP)"). It wraps the same RTPS message bytes wrapInRTPSMessage produces
// for the plain UDP transport in a DTLS-secured datagram session, so a
// message boundary is preserved 1:1 with a DTLS record — unlike
// RTPS-over-TCP (transport_tcp.go), no length-prefix framing is needed:
// DTLS, like UDP, is datagram-oriented rather than a byte stream.
//
// Go's standard library crypto/tls implements TLS (stream) only — it has no
// DTLS support, and DTLS 1.3 (RFC 9147) has no production-ready
// implementation available anywhere in the Go ecosystem as of this writing
// (including github.com/pion/dtls, the most mature Go DTLS library, whose
// v3 README lists DTLS 1.3 under "Planned Features"). This transport
// therefore uses DTLS 1.2 (RFC 6347) via github.com/pion/dtls/v3 — the
// exception to go-DDS's usual "stdlib only" transport policy (see
// transport_tcp.go's TLS 1.3 comment), because no stdlib alternative exists.
// WithDTLS still takes a stdlib *tls.Config, exactly like WithTCPTLSConfig,
// so callers and CertPlugin.TLSCertificate/CAPool (security/cert.go) work
// identically across both transports; dtlsServerOptions/dtlsClientOptions
// translate it into the pion/dtls functional options this file drives
// internally (the package's recommended API — a bare *dtls.Config is
// deprecated as of pion/dtls v3).

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/pion/dtls/v3"
)

// dtlsDialTimeout bounds how long an outbound DTLS handshake may block.
const dtlsDialTimeout = 5 * time.Second

// dtlsPacket is a received RTPS-over-DTLS message together with the
// sender's address (as a "host:port" string, like tcpPacket.from — a DTLS
// association, like a TCP connection, has no per-datagram net.UDPAddr the
// way a raw UDP socket does).
type dtlsPacket struct {
	data []byte
	from string
}

// dtlsSocket is the DTLS analogue of udpSocket/tcpSocket: it accepts inbound
// DTLS associations on a listen address and dials outbound associations to
// peers on demand, caching one per peer address for reuse.
type dtlsSocket struct {
	ln net.Listener // DTLS-wrapped UDP listener (dtls.ListenWithOptions)

	tlsConfig  *tls.Config // original config, kept for introspection/tests
	clientOpts []dtls.ClientOption

	// effectiveClientAuth is cfg.ClientAuth after dtlsServerOptions' default
	// upgrade (REQ-TRANS-007) — the actual policy the listener enforces,
	// exposed for tests since it isn't otherwise readable back from the
	// functional ServerOption slice passed to dtls.ListenWithOptions.
	effectiveClientAuth tls.ClientAuthType

	port int

	recv chan dtlsPacket
	done chan struct{}

	mu    sync.Mutex
	conns map[string]net.Conn    // peer addr -> cached outbound/inbound DTLS conn
	wmu   map[string]*sync.Mutex // per-peer lock: serialises writes and dials
}

// newDTLSSocket starts a DTLS 1.2 listener on addr, authenticated with
// tlsConfig (see dtlsServerOptions/dtlsClientOptions for the field mapping),
// and returns a socket ready to accept inbound associations and dial
// outbound ones.
func newDTLSSocket(addr string, tlsConfig *tls.Config) (*dtlsSocket, error) {
	laddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return nil, fmt.Errorf("rtps: DTLS resolve %s: %w", addr, err)
	}
	serverOpts, effectiveClientAuth := dtlsServerOptions(tlsConfig)
	ln, err := dtls.ListenWithOptions("udp", laddr, serverOpts...)
	if err != nil {
		return nil, fmt.Errorf("rtps: DTLS listen %s: %w", addr, err)
	}
	port := 0
	if udpAddr, ok := ln.Addr().(*net.UDPAddr); ok {
		port = udpAddr.Port
	}
	s := &dtlsSocket{
		ln:                  ln,
		tlsConfig:           tlsConfig,
		clientOpts:          dtlsClientOptions(tlsConfig),
		effectiveClientAuth: effectiveClientAuth,
		port:                port,
		recv:                make(chan dtlsPacket, 256),
		done:                make(chan struct{}),
		conns:               make(map[string]net.Conn),
		wmu:                 make(map[string]*sync.Mutex),
	}
	go s.acceptLoop()
	return s, nil
}

// dtlsServerOptions translates a stdlib *tls.Config into the pion/dtls
// functional ServerOptions this transport drives internally (dtls.Config
// itself is deprecated in favour of the options-based API as of pion/dtls
// v3), so callers — and CertPlugin.TLSCertificate/CAPool — only ever deal
// with stdlib types. A nil input yields no options (insecure, no cert),
// matching how a nil *tls.Config behaves for RTPS-over-TCP.
//
// When the caller left ClientAuth at its zero value (tls.NoClientCert)
// while also supplying a ClientCAs pool, ClientAuth is upgraded to
// tls.RequireAndVerifyClientCert (returned as the second value): supplying
// a CA pool to authenticate peers against is a clear signal the caller
// wants mutual certificate authentication, and NoClientCert would silently
// accept any unauthenticated peer despite that pool being configured.
func dtlsServerOptions(cfg *tls.Config) ([]dtls.ServerOption, tls.ClientAuthType) {
	if cfg == nil {
		cfg = &tls.Config{}
	}
	var opts []dtls.ServerOption
	if len(cfg.Certificates) > 0 {
		opts = append(opts, dtls.WithCertificates(cfg.Certificates...))
	}
	if cfg.ClientCAs != nil {
		opts = append(opts, dtls.WithClientCAs(cfg.ClientCAs))
	}
	if cfg.RootCAs != nil {
		opts = append(opts, dtls.WithRootCAs(cfg.RootCAs))
	}
	clientAuth := cfg.ClientAuth
	if clientAuth == tls.NoClientCert && cfg.ClientCAs != nil {
		clientAuth = tls.RequireAndVerifyClientCert
	}
	opts = append(opts, dtls.WithClientAuth(dtls.ClientAuthType(clientAuth)))
	if cfg.InsecureSkipVerify {
		opts = append(opts, dtls.WithInsecureSkipVerify(true))
	}
	if cfg.ServerName != "" {
		opts = append(opts, dtls.WithServerName(cfg.ServerName))
	}
	return opts, clientAuth
}

// dtlsClientOptions is dtlsServerOptions' client-side counterpart: it has no
// ClientAuth/ClientCAs equivalent (those are server-only concepts), so it is
// a straight subset covering the fields WithRootCAs verifies the server
// against, plus the client's own Certificates when the server requests one.
func dtlsClientOptions(cfg *tls.Config) []dtls.ClientOption {
	if cfg == nil {
		cfg = &tls.Config{}
	}
	var opts []dtls.ClientOption
	if len(cfg.Certificates) > 0 {
		opts = append(opts, dtls.WithCertificates(cfg.Certificates...))
	}
	if cfg.RootCAs != nil {
		opts = append(opts, dtls.WithRootCAs(cfg.RootCAs))
	}
	if cfg.InsecureSkipVerify {
		opts = append(opts, dtls.WithInsecureSkipVerify(true))
	}
	if cfg.ServerName != "" {
		opts = append(opts, dtls.WithServerName(cfg.ServerName))
	}
	return opts
}

// acceptLoop accepts inbound DTLS associations and spawns a read loop for
// each. Like tls.Listener, the DTLS handshake for an accepted connection is
// lazy: it runs on the first Read/Write, performed here by readConn.
func (s *dtlsSocket) acceptLoop() {
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

// readConn reads DTLS records from conn until it errors or closes, pushing
// each decrypted record — one RTPS message, since DTLS preserves datagram
// boundaries — onto recv.
func (s *dtlsSocket) readConn(conn net.Conn, from string) {
	defer func() {
		_ = conn.Close()
		s.dropConn(from)
	}()
	buf := make([]byte, maxUDPSize)
	for {
		n, err := conn.Read(buf)
		if err != nil {
			return
		}
		data := make([]byte, n)
		copy(data, buf[:n])
		select {
		case s.recv <- dtlsPacket{data: data, from: from}:
		case <-s.done:
			return
		default: // slow consumer; drop
		}
	}
}

// send writes data as a single DTLS record to addr, dialling (or reusing a
// cached association to) addr as needed. Writes to the same peer are
// serialised so concurrent senders never interleave, and a failed write
// drops the cached connection so the next send redials.
func (s *dtlsSocket) send(addr string, data []byte) error {
	if len(data) > maxUDPSize {
		return fmt.Errorf("rtps: DTLS record too large: %d bytes", len(data))
	}
	wmu := s.peerLock(addr)
	wmu.Lock()
	defer wmu.Unlock()

	conn, err := s.connLocked(addr)
	if err != nil {
		return err
	}
	if _, err := conn.Write(data); err != nil {
		s.dropConn(addr)
		return fmt.Errorf("rtps: DTLS write %s: %w", addr, err)
	}
	return nil
}

// peerLock returns (creating on first use) the per-peer mutex for addr.
func (s *dtlsSocket) peerLock(addr string) *sync.Mutex {
	s.mu.Lock()
	defer s.mu.Unlock()
	wmu, ok := s.wmu[addr]
	if !ok {
		wmu = &sync.Mutex{}
		s.wmu[addr] = wmu
	}
	return wmu
}

// connLocked returns the cached DTLS connection for addr, dialling and
// handshaking a fresh one — bounded by dtlsDialTimeout — if none is cached.
// Caller must hold the lock returned by peerLock(addr), which also
// serialises concurrent dials to the same peer.
func (s *dtlsSocket) connLocked(addr string) (net.Conn, error) {
	s.mu.Lock()
	conn, ok := s.conns[addr]
	s.mu.Unlock()
	if ok {
		return conn, nil
	}

	raddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return nil, fmt.Errorf("rtps: DTLS resolve %s: %w", addr, err)
	}
	pconn, err := net.ListenUDP("udp", nil)
	if err != nil {
		return nil, fmt.Errorf("rtps: DTLS dial %s: %w", addr, err)
	}
	dconn, err := dtls.ClientWithOptions(pconn, raddr, s.clientOpts...)
	if err != nil {
		_ = pconn.Close()
		return nil, fmt.Errorf("rtps: DTLS client %s: %w", addr, err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), dtlsDialTimeout)
	defer cancel()
	if err := dconn.HandshakeContext(ctx); err != nil {
		_ = dconn.Close()
		return nil, fmt.Errorf("rtps: DTLS handshake %s: %w", addr, err)
	}

	s.mu.Lock()
	s.conns[addr] = dconn
	s.mu.Unlock()
	go s.readConn(dconn, addr)
	return dconn, nil
}

// dropConn closes and evicts the cached connection to addr, if any.
func (s *dtlsSocket) dropConn(addr string) {
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
func (s *dtlsSocket) close() {
	close(s.done)
	_ = s.ln.Close()
	s.mu.Lock()
	for _, conn := range s.conns {
		_ = conn.Close()
	}
	s.conns = nil
	s.mu.Unlock()
}
