// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package rtps

//fusa:req REQ-TRANS-012
//fusa:req REQ-TRANS-013
//fusa:req REQ-TRANS-014

// RTPS-over-QUIC transport (Milestone 16, ROADMAP.md "QUIC Transport"). Each
// peer address gets one QUIC connection carrying two independent channels:
//
//   - A single reliable, ordered bidirectional stream, framed with a 4-byte
//     big-endian length prefix exactly like transport_tcp.go's RTPS-over-TCP
//     framing. SPDP and SEDP discovery — and anything else the caller marks
//     reliable, see send's reliable parameter — always use this stream.
//   - Unreliable QUIC DATAGRAM frames (RFC 9221) for best-effort user DATA.
//     RTPS's own reliability protocol (HEARTBEAT/ACKNACK retransmission,
//     reliable.go) already tolerates transport-level loss — that is how the
//     existing plain-UDP transport achieves Reliable QoS despite UDP itself
//     being unreliable — so a best-effort sample that doesn't need that
//     protocol is free to ride the cheaper, head-of-line-blocking-free
//     datagram path instead of the stream. A message too large for a single
//     datagram at the current path MTU falls back to the reliable stream
//     rather than being dropped (see send).
//
// Because control traffic and best-effort data travel on genuinely separate
// QUIC streams/frames rather than being serialised through one byte stream
// (as RTPS-over-TCP necessarily does), a slow or lossy run of best-effort
// samples can never head-of-line-block a concurrent SPDP/SEDP exchange or a
// reliable retransmission on the same session — this is the "multi-stream
// RTPS session" property ROADMAP.md calls out, and the reason QUIC is used
// here at all rather than just another TCP-shaped transport.
//
// QUIC mandates TLS 1.3 end-to-end, so — unlike RTPS-over-TCP, where
// WithTCPTLSConfig is optional — a *tls.Config is always in effect; when the
// caller's config leaves ALPN/MinVersion/ClientSessionCache unset,
// quicTLSConfig fills in go-DDS's defaults (see its doc comment for the
// 0-RTT session cache rationale).
//
// Interoperability scoping note: FastDDS's QUIC transport extension
// (ROADMAP.md "Interoperable with FastDDS QUIC extension (draft spec)") is,
// as of this writing, an evolving draft with no published fixed ALPN token
// or wire-framing spec to conform to byte-for-byte — the same kind of gap
// transport_dtls.go documents for DTLS 1.3. This transport's framing is a
// straightforward, defensible reading of "RTPS messages over QUIC streams
// and datagrams," not a claim of byte-for-byte conformance to that draft;
// operators who need to match a specific FastDDS build's ALPN can override
// it directly via WithQUIC's tlsCfg.NextProtos. See interop/doc.go for
// go-DDS's live-peer wire-compatibility testing pattern, which a future
// round can extend to a FastDDS QUIC peer once the draft stabilises.
import (
	"context"
	"crypto/tls"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"github.com/quic-go/quic-go"
)

// maxQUICFrameSize bounds a single RTPS-over-QUIC stream frame, the same
// protection maxTCPFrameSize gives the TCP transport against a corrupt or
// hostile peer claiming an unbounded length prefix.
const maxQUICFrameSize = 1 << 20 // 1 MiB

// quicDialTimeout bounds how long an outbound QUIC connection attempt (and
// its initial control-stream open) may block.
const quicDialTimeout = 5 * time.Second

// quicALPN is the default ALPN protocol negotiated for RTPS-over-QUIC when
// the caller's tlsCfg.NextProtos is left unset — see this file's doc comment
// for the FastDDS-draft interoperability caveat.
const quicALPN = "godds-rtps/1"

// quicSessionCacheSize bounds the default client-side TLS session-ticket
// cache installed by quicTLSConfig — see its doc comment.
const quicSessionCacheSize = 64

// quicPacket is a received RTPS-over-QUIC message together with the sender's
// address. Like tcpPacket/dtlsPacket, from is a "host:port" string.
type quicPacket struct {
	data []byte
	from string
}

// quicPeerConn is one outbound QUIC connection this socket dialled to a
// peer, plus the reliable control stream opened on it. wmu serialises
// writes to stream (a *quic.Stream, like a TCP connection, is not safe for
// concurrent writers) and datagram sends together, so the two channels for
// one peer are never reordered relative to each other by concurrent callers.
type quicPeerConn struct {
	conn   *quic.Conn
	stream *quic.Stream
	wmu    sync.Mutex
}

// quicSocket is the QUIC analogue of tcpSocket/dtlsSocket: it accepts
// inbound QUIC connections on a listen address and dials outbound
// connections to peers on demand, caching one per peer address for reuse.
type quicSocket struct {
	ln              *quic.EarlyListener // early accept: lets 0-RTT/0.5-RTT data flow before the handshake fully confirms
	clientTLSConfig *tls.Config         // see quicTLSConfig(cfg, true)
	quicConfig      *quic.Config
	port            int

	recv chan quicPacket
	done chan struct{}

	mu    sync.Mutex
	peers map[string]*quicPeerConn // peer addr -> cached outbound connection
	wmu   map[string]*sync.Mutex   // per-peer lock: serialises dials
}

// newQUICSocket starts a QUIC listener on addr, authenticated with
// tlsConfig (see quicTLSConfig for how defaults are filled in), and returns
// a socket ready to accept inbound connections and dial outbound ones.
func newQUICSocket(addr string, tlsConfig *tls.Config) (*quicSocket, error) {
	serverCfg := quicTLSConfig(tlsConfig, false)
	clientCfg := quicTLSConfig(tlsConfig, true)
	qConf := &quic.Config{
		// Allow0RTT (server-side only) lets a returning client's early data
		// — including its first control-stream frame — be accepted before
		// the handshake is confirmed: the 0-RTT reconnection ROADMAP.md
		// calls for.
		Allow0RTT:       true,
		EnableDatagrams: true,
	}
	ln, err := quic.ListenAddrEarly(addr, serverCfg, qConf)
	if err != nil {
		return nil, fmt.Errorf("rtps: QUIC listen %s: %w", addr, err)
	}
	port := 0
	if udpAddr, ok := ln.Addr().(*net.UDPAddr); ok {
		port = udpAddr.Port
	}
	s := &quicSocket{
		ln:              ln,
		clientTLSConfig: clientCfg,
		quicConfig:      qConf,
		port:            port,
		recv:            make(chan quicPacket, 256),
		done:            make(chan struct{}),
		peers:           make(map[string]*quicPeerConn),
		wmu:             make(map[string]*sync.Mutex),
	}
	go s.acceptLoop()
	return s, nil
}

// quicTLSConfig fills in go-DDS's defaults for whatever the caller left
// unset in cfg (WithQUIC), for either the server or client role, without
// mutating cfg itself (a fresh Clone is returned, or a fresh empty Config
// when cfg is nil):
//   - NextProtos (ALPN): quic-go requires a non-empty value (QUIC has no
//     stdlib-assigned default the way plain TLS-over-TCP does); defaults to
//     quicALPN.
//   - MinVersion: QUIC already mandates TLS 1.3 at the protocol level, so
//     this is mostly documentation, but it is set explicitly for the same
//     reason WithTCPTLSConfig/WithDTLS do: an unset MinVersion should never
//     silently mean "whatever the peer negotiates."
//   - ClientSessionCache (client role only): 0-RTT reconnection (ROADMAP.md)
//     works by resuming a session ticket issued during a prior handshake;
//     without somewhere to store that ticket, DialAddrEarly can never
//     actually attempt 0-RTT, even to a peer this process already
//     successfully connected to. quicSessionCacheSize peers' worth of
//     tickets is ample for a single participant's peer set. A persisted
//     cache surviving process restart is a future enhancement (this session
//     cache, like DTLS 1.2 in transport_dtls.go, is a documented scoping
//     choice, not a claim of the whole feature space).
func quicTLSConfig(cfg *tls.Config, isClient bool) *tls.Config {
	var out *tls.Config
	if cfg != nil {
		out = cfg.Clone()
	} else {
		out = &tls.Config{}
	}
	if len(out.NextProtos) == 0 {
		out.NextProtos = []string{quicALPN}
	}
	if out.MinVersion == 0 {
		out.MinVersion = tls.VersionTLS13
	}
	if isClient && out.ClientSessionCache == nil {
		out.ClientSessionCache = tls.NewLRUClientSessionCache(quicSessionCacheSize)
	}
	return out
}

// acceptLoop accepts inbound QUIC connections and spawns the control-stream
// and datagram read loops for each.
func (s *quicSocket) acceptLoop() {
	for {
		conn, err := s.ln.Accept(context.Background())
		if err != nil {
			select {
			case <-s.done:
				return
			default:
				continue
			}
		}
		from := conn.RemoteAddr().String()
		go s.acceptControlStream(conn, from)
		go s.readDatagrams(conn, from)
	}
}

// acceptControlStream waits for the inbound peer's self-initiated control
// stream (opened on its side by connLocked's OpenStreamSync) and then reads
// framed messages from it until the connection closes.
func (s *quicSocket) acceptControlStream(conn *quic.Conn, from string) {
	stream, err := conn.AcceptStream(context.Background())
	if err != nil {
		return
	}
	s.readFramedLoop(stream, conn, from)
}

// readFramedLoop frame-decodes length-prefixed messages from stream until it
// errors, closes, or receives a malformed/hostile length prefix, pushing
// each well-formed message onto recv. Shared by both the accept-side
// (acceptControlStream) and dial-side (connLocked) control streams. On any
// exit it tears down the whole QUIC connection — not just this stream — and
// evicts it from the dial cache if it was in one (a harmless no-op
// otherwise, e.g. for an accept-side connection this socket never dialled):
// a broken or hostile control stream invalidates the connection for RTPS
// purposes even though the datagram channel might still technically work.
func (s *quicSocket) readFramedLoop(stream *quic.Stream, conn *quic.Conn, from string) {
	defer func() {
		_ = conn.CloseWithError(0, "")
		s.dropConn(from)
	}()
	lenBuf := make([]byte, 4)
	for {
		if err := quicReadFull(stream, lenBuf); err != nil {
			return
		}
		n := binary.BigEndian.Uint32(lenBuf)
		if n == 0 || n > maxQUICFrameSize {
			return // malformed or hostile frame; drop the connection
		}
		data := make([]byte, n)
		if err := quicReadFull(stream, data); err != nil {
			return
		}
		select {
		case s.recv <- quicPacket{data: data, from: from}:
		case <-s.done:
			return
		default: // slow consumer; drop
		}
	}
}

// readDatagrams loops ReceiveDatagram for conn until it errors or closes,
// pushing each unreliable best-effort datagram onto recv whole — like a DTLS
// record, one QUIC datagram maps 1:1 to one RTPS message, no framing needed.
// Mirrors readFramedLoop's connection teardown/cache-eviction on exit.
func (s *quicSocket) readDatagrams(conn *quic.Conn, from string) {
	defer func() {
		_ = conn.CloseWithError(0, "")
		s.dropConn(from)
	}()
	for {
		data, err := conn.ReceiveDatagram(context.Background())
		if err != nil {
			return
		}
		cp := make([]byte, len(data))
		copy(cp, data)
		select {
		case s.recv <- quicPacket{data: cp, from: from}:
		case <-s.done:
			return
		default: // slow consumer; drop
		}
	}
}

// quicReadFull reads exactly len(buf) bytes from r, like io.ReadFull.
func quicReadFull(r io.Reader, buf []byte) error {
	_, err := io.ReadFull(r, buf)
	return err
}

// send delivers data to addr, dialling (or reusing a cached connection to)
// addr as needed. When reliable is false (best-effort user DATA — see this
// file's doc comment), it first tries an unreliable QUIC datagram; if the
// payload doesn't fit in one at the current path MTU (*quic.
// DatagramTooLargeError), it falls back to the reliable stream below rather
// than dropping the message. When reliable is true (SPDP/SEDP discovery,
// HEARTBEAT/ACKNACK, or any reliable-QoS writer's DATA — see
// participant.sendUnicast's call sites), it always uses the reliable framed
// stream. Writes to the same peer are serialised so concurrent senders never
// interleave stream frames, and a failed stream write drops the cached
// connection so the next send redials.
func (s *quicSocket) send(addr string, data []byte, reliable bool) error {
	if len(data) > maxQUICFrameSize {
		return fmt.Errorf("rtps: QUIC frame too large: %d bytes", len(data))
	}
	wmu := s.peerLock(addr)
	wmu.Lock()
	defer wmu.Unlock()

	pc, err := s.connLocked(addr)
	if err != nil {
		return err
	}

	if !reliable {
		pc.wmu.Lock()
		dgErr := pc.conn.SendDatagram(data)
		pc.wmu.Unlock()
		if dgErr == nil {
			return nil
		}
		var tooLarge *quic.DatagramTooLargeError
		if !errors.As(dgErr, &tooLarge) {
			// A real send failure (peer gone, datagrams unsupported by the
			// peer, ...): drop the cached connection so the next send
			// redials, and report failure so participant.sendUnicast can
			// fall back to another transport.
			s.dropConn(addr)
			return fmt.Errorf("rtps: QUIC datagram %s: %w", addr, dgErr)
		}
		// DatagramTooLargeError: fall through to the reliable stream — the
		// datagram path is a best-effort optimisation, not the only way to
		// deliver this message.
	}

	frame := make([]byte, 4+len(data))
	binary.BigEndian.PutUint32(frame[:4], uint32(len(data)))
	copy(frame[4:], data)
	pc.wmu.Lock()
	_, werr := pc.stream.Write(frame)
	pc.wmu.Unlock()
	if werr != nil {
		s.dropConn(addr)
		return fmt.Errorf("rtps: QUIC stream write %s: %w", addr, werr)
	}
	return nil
}

// peerLock returns (creating on first use) the per-peer dial-serialisation
// mutex for addr.
func (s *quicSocket) peerLock(addr string) *sync.Mutex {
	s.mu.Lock()
	defer s.mu.Unlock()
	wmu, ok := s.wmu[addr]
	if !ok {
		wmu = &sync.Mutex{}
		s.wmu[addr] = wmu
	}
	return wmu
}

// connLocked returns the cached connection for addr, dialling a fresh one —
// including opening its control stream — if none is cached. Caller must
// hold the lock returned by peerLock(addr), which also serialises
// concurrent dials to the same peer. DialAddrEarly, combined with the
// client session cache quicTLSConfig installs, is what lets a redial to a
// previously-contacted addr attempt 0-RTT resumption instead of a full
// handshake.
func (s *quicSocket) connLocked(addr string) (*quicPeerConn, error) {
	s.mu.Lock()
	pc, ok := s.peers[addr]
	s.mu.Unlock()
	if ok {
		return pc, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), quicDialTimeout)
	defer cancel()
	conn, err := quic.DialAddrEarly(ctx, addr, s.clientTLSConfig, s.quicConfig)
	if err != nil {
		return nil, fmt.Errorf("rtps: QUIC dial %s: %w", addr, err)
	}
	stream, err := conn.OpenStreamSync(ctx)
	if err != nil {
		_ = conn.CloseWithError(0, "control stream open failed")
		return nil, fmt.Errorf("rtps: QUIC open stream %s: %w", addr, err)
	}

	pc = &quicPeerConn{conn: conn, stream: stream}
	s.mu.Lock()
	s.peers[addr] = pc
	s.mu.Unlock()
	go s.readFramedLoop(stream, conn, addr)
	go s.readDatagrams(conn, addr)
	return pc, nil
}

// dropConn closes and evicts the cached connection to addr, if any.
func (s *quicSocket) dropConn(addr string) {
	s.mu.Lock()
	pc, ok := s.peers[addr]
	if ok {
		delete(s.peers, addr)
	}
	s.mu.Unlock()
	if ok {
		_ = pc.stream.Close()
		_ = pc.conn.CloseWithError(0, "")
	}
}

// close shuts down the listener and every cached connection.
func (s *quicSocket) close() {
	close(s.done)
	_ = s.ln.Close()
	s.mu.Lock()
	for _, pc := range s.peers {
		_ = pc.stream.Close()
		_ = pc.conn.CloseWithError(0, "")
	}
	s.peers = nil
	s.mu.Unlock()
}
