// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package rtps

//fusa:req REQ-TRANS-015
//fusa:req REQ-TRANS-016
//fusa:req REQ-TRANS-017
//fusa:req REQ-TRANS-018
//fusa:req REQ-TRANS-019

// RTPS-over-WebSocket transport (Milestone 16, ROADMAP.md "WebSocket
// Transport") — the sub-phase that lets a browser tab or Wasm participant
// join a DDS domain directly, the way transport_tcp.go/transport_quic.go
// already let a TCP- or QUIC-capable peer do so, with no separate protocol
// bridge required (see bridge/ws for an *additional*, optional gateway that
// speaks a simpler JSON pub/sub protocol instead of full RTPS, for clients
// that would rather not implement SPDP/SEDP at all).
//
// wsSocket is the WebSocket analogue of tcpSocket: it accepts inbound
// connections on a listen address (performing the RFC 6455 opening
// handshake itself, using only net/http's request/response parsers — no
// external dependency, the same "no external dependency" choice
// transport_tcp.go documents for its own TLS wrapping) and dials outbound
// connections to peers on demand, caching one per peer address for reuse,
// exactly like tcpSocket. Unlike tcpSocket, no additional 4-byte
// length-prefix framing is layered on top: a WebSocket connection is
// already message-oriented (RFC 6455 §5.2's FIN bit delimits one logical
// message, however many frames it is fragmented across), so one RTPS
// message maps directly to one WebSocket message.
//
// Every WebSocket message carries its payload one of two ways — the "JSON
// and binary (base64-CDR) framing modes" ROADMAP.md calls for:
//
//   - Binary framing (the default; see wsFramingBinary): a WebSocket BINARY
//     frame whose payload is the raw RTPS message bytes, unmodified. This is
//     the efficient path — no encoding overhead — and what two go-DDS
//     participants use when talking to each other over WithWSAddr/
//     WithWSPeers.
//   - JSON framing (wsFramingJSON, selected via WithWSFraming): a WebSocket
//     TEXT frame containing {"data":"<base64 CDR>"}. This exists for
//     JavaScript/TypeScript environments (browsers, Wasm runtimes without
//     convenient binary-frame plumbing, or a hand-written client that would
//     rather work with a JSON-native library) where handling text frames is
//     more convenient than binary ones — see js/dds-client, which supports
//     both.
//
// A socket's own framing field only controls what it *sends*; every inbound
// message is decoded by its actual opcode (BINARY or TEXT) regardless of the
// receiving socket's configured framing, so a go-DDS peer configured for
// binary framing can still receive a JSON-framed message from a browser
// client, and vice versa — the two framing modes freely interoperate on the
// wire.
//
// RFC 6455 requires every frame a client sends to be masked with a random
// 32-bit key (and forbids a server from masking its own frames); wsConn
// tracks which role (isClient) this connection is playing so send/receive
// mask correctly regardless of which side of a given connection this
// process is on — the accept side (this socket's listener) is always the
// server role, and the dial side (connLocked) is always the client role,
// exactly mirroring how quicSocket's accept/dial sides map onto QUIC's own
// server/client TLS roles.
//
// Two further additions support the "WebAssembly Target" sub-phase
// (ROADMAP.md Milestone 16), which needs this transport to work as the
// *only* transport for a peer that can never accept inbound connections:
//
//   - Listener-less ("dial-only") sockets: newWSSocket(addr, ...) now
//     accepts addr == "" and simply skips the net.Listen/acceptLoop step —
//     s.port stays 0 and s.ln stays nil. participant.go creates a wsSocket
//     whenever either WithWSAddr or WithWSPeers is supplied (previously
//     WithWSPeers alone was a no-op — see its doc comment), so a browser or
//     edge-function participant that can only ever dial out — never bind a
//     publicly reachable listener — still gets a working WS transport for
//     outbound SPDP/SEDP/user-data unicast. spdp.go skips advertising a
//     pidWSLocator when s.port == 0: a dial-only participant has no address
//     for peers to dial back into, so it would be actively misleading to
//     claim one.
//   - A per-GOOS dial backend: every platform except GOOS=js GOARCH=wasm
//     dials out with a real net.Conn and performs the RFC 6455 handshake
//     itself (transport_ws_dial.go, unchanged from before this sub-phase).
//     GOOS=js GOARCH=wasm — a real browser tab — has no such thing: the Go
//     net package's js/wasm port is "fake networking... intended to allow
//     tests of other packages to pass" (see $GOROOT/src/net/fd_js.go) and
//     never reaches a real remote host, so a build targeting an actual
//     browser dials out via syscall/js against the browser's own, already
//     RFC-6455-compliant WebSocket object instead (transport_ws_browser.go)
//     — see wsConnIface, which both backends implement identically from
//     wsSocket's point of view.
import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/sha1"
	"crypto/tls"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// maxWSFrameSize bounds a single RTPS-over-WebSocket message (after any
// JSON/base64 decoding), the same protection maxTCPFrameSize/
// maxQUICFrameSize give their transports against a corrupt or hostile peer
// claiming an unbounded payload length.
const maxWSFrameSize = 1 << 20 // 1 MiB

// wsDialTimeout bounds how long an outbound WebSocket connection attempt —
// including the TCP/TLS dial and the opening handshake round trip — may
// block.
const wsDialTimeout = 5 * time.Second

// wsHandshakeTimeout bounds how long this socket waits for an inbound
// peer's opening handshake request to arrive before giving up on that
// connection, so a client that connects but never sends a valid HTTP
// Upgrade request cannot tie up a goroutine (and a file descriptor)
// forever.
const wsHandshakeTimeout = 5 * time.Second

// wsMagicGUID is the fixed GUID RFC 6455 §1.3 defines for computing
// Sec-WebSocket-Accept from a client's Sec-WebSocket-Key.
const wsMagicGUID = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"

// WebSocket opcodes (RFC 6455 §5.2).
const (
	wsOpContinuation = 0x0
	wsOpText         = 0x1
	wsOpBinary       = 0x2
	wsOpClose        = 0x8
	wsOpPing         = 0x9
	wsOpPong         = 0xA
)

// wsFraming selects how a wsSocket encodes outbound RTPS messages onto the
// WebSocket wire — see this file's doc comment. It never affects how
// inbound messages are decoded (that is always determined by the received
// frame's own opcode).
type wsFraming int

const (
	// wsFramingBinary sends each RTPS message as a single WebSocket BINARY
	// frame containing the raw message bytes. The default, and the
	// efficient choice for go-DDS-to-go-DDS traffic.
	wsFramingBinary wsFraming = iota
	// wsFramingJSON sends each RTPS message as a single WebSocket TEXT
	// frame containing {"data":"<base64 CDR>"} — see wsJSONMessage.
	wsFramingJSON
)

// wsJSONMessage is the JSON envelope used by wsFramingJSON — see this
// file's doc comment.
type wsJSONMessage struct {
	Data string `json:"data"`
}

// wsPacket is a received RTPS-over-WebSocket message together with the
// sender's address. Like tcpPacket/quicPacket, from is a "host:port"
// string.
type wsPacket struct {
	data []byte
	from string
}

// wsConnIface abstracts one WebSocket connection to a peer, spanning the
// two backends this transport can use to obtain one — see this file's doc
// comment. wsSocket (readLoop, connLocked, dropConn, close) only ever talks
// to a wsConnIface, never to a concrete backend type, so none of that
// shared code needs to know or care which backend produced the connection
// it is holding.
type wsConnIface interface {
	// readMessage reads one complete WebSocket message, exactly as
	// (*wsConn).readMessage does — see that method's doc comment.
	readMessage() (opcode byte, payload []byte, err error)
	// writeMessage encodes and sends one RTPS message per framing, exactly
	// as (*wsConn).writeMessage does — see that method's doc comment.
	writeMessage(framing wsFraming, data []byte) error
	// close releases the connection and any resources (goroutines, JS
	// callbacks) it holds. Idempotent.
	close() error
}

// wsConn is the default wsConnIface backend — used on every platform except
// GOOS=js GOARCH=wasm — wrapping a real net.Conn with the buffered reader
// frame decoding needs and the role (isClient) that determines masking
// behaviour on both send and receive. wmu serialises writes — both
// application sends (wsSocket.send) and control-frame replies this
// connection's own read loop issues (a Pong reply to an inbound Ping) — so
// two goroutines can never interleave bytes of two different frames on the
// wire.
type wsConn struct {
	nc       net.Conn
	br       *bufio.Reader
	isClient bool // true: this process dialled out (must mask frames it sends)
	wmu      sync.Mutex
}

var _ wsConnIface = (*wsConn)(nil)

// close implements wsConnIface.
func (c *wsConn) close() error { return c.nc.Close() }

// wsSocket is the WebSocket analogue of tcpSocket: it accepts inbound
// WebSocket connections on a listen address and dials outbound connections
// to peers on demand, caching one per peer address for reuse. addr == ""
// (see newWSSocket) puts it in dial-only mode: no listener, ln stays nil
// and port stays 0 — the mode a browser or edge-function participant that
// can never accept inbound connections uses (see this file's doc comment).
type wsSocket struct {
	ln        net.Listener
	tlsConfig *tls.Config // non-nil = wss:// (TLS-wrapped); nil = plain ws://
	port      int
	framing   wsFraming

	recv chan wsPacket
	done chan struct{}

	mu    sync.Mutex
	conns map[string]wsConnIface // peer addr -> cached outbound/inbound connection
	wmu   map[string]*sync.Mutex // per-peer lock: serialises dials
}

// newWSSocket returns a socket ready to dial outbound WebSocket
// connections and, if addr is non-empty, to also accept inbound ones on
// addr (TLS-wrapped — wss:// — when tlsConfig is non-nil). addr == "" is
// dial-only mode: no net.Listen call is made at all and s.port stays 0 —
// see this file's doc comment on the "WebAssembly Target" sub-phase this
// exists for. When tlsConfig is supplied with MinVersion unset, TLS 1.3 is
// enforced, matching newTCPSocket.
func newWSSocket(addr string, tlsConfig *tls.Config, framing wsFraming) (*wsSocket, error) {
	cfg := tlsConfig
	if cfg != nil && cfg.MinVersion == 0 {
		clone := cfg.Clone()
		clone.MinVersion = tls.VersionTLS13
		cfg = clone
	}
	s := &wsSocket{
		tlsConfig: cfg,
		framing:   framing,
		recv:      make(chan wsPacket, 256),
		done:      make(chan struct{}),
		conns:     make(map[string]wsConnIface),
		wmu:       make(map[string]*sync.Mutex),
	}
	if addr == "" {
		return s, nil
	}
	var ln net.Listener
	var err error
	if cfg != nil {
		ln, err = tls.Listen("tcp", addr, cfg)
	} else {
		ln, err = (&net.ListenConfig{}).Listen(context.Background(), "tcp", addr)
	}
	if err != nil {
		return nil, fmt.Errorf("rtps: WS listen %s: %w", addr, err)
	}
	s.ln = ln
	if tcpAddr, ok := ln.Addr().(*net.TCPAddr); ok {
		s.port = tcpAddr.Port
	}
	go s.acceptLoop()
	return s, nil
}

// acceptLoop accepts inbound TCP/TLS connections and performs the
// server-side WebSocket handshake on each before starting its read loop.
func (s *wsSocket) acceptLoop() {
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
		go s.handleAccept(conn)
	}
}

// handleAccept performs the RFC 6455 server-side opening handshake on a
// freshly accepted connection — reading the client's HTTP Upgrade request
// (via net/http's own request parser, so no hand-rolled HTTP parsing) and
// writing back a "101 Switching Protocols" response with the computed
// Sec-WebSocket-Accept — then hands the connection to readLoop. Any
// handshake failure (malformed request, missing/invalid Upgrade headers, a
// write error) simply closes the connection: a WebSocket peer that cannot
// complete the handshake was never a viable RTPS transport connection to
// begin with, exactly as a TLS handshake failure is fatal for tcpSocket.
func (s *wsSocket) handleAccept(nc net.Conn) {
	_ = nc.SetDeadline(time.Now().Add(wsHandshakeTimeout))
	br := bufio.NewReader(nc)
	req, err := http.ReadRequest(br)
	if err != nil {
		_ = nc.Close()
		return
	}
	key := req.Header.Get("Sec-WebSocket-Key")
	if key == "" || !headerContainsToken(req.Header.Get("Upgrade"), "websocket") ||
		!headerContainsToken(req.Header.Get("Connection"), "upgrade") {
		_ = nc.Close()
		return
	}
	resp := "HTTP/1.1 101 Switching Protocols\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Accept: " + wsAcceptKey(key) + "\r\n\r\n"
	if _, err := io.WriteString(nc, resp); err != nil {
		_ = nc.Close()
		return
	}
	_ = nc.SetDeadline(time.Time{})

	from := nc.RemoteAddr().String()
	wc := &wsConn{nc: nc, br: br, isClient: false}
	s.mu.Lock()
	s.conns[from] = wc
	s.mu.Unlock()
	s.readLoop(wc, from)
}

// headerContainsToken reports whether header — a comma-separated list per
// RFC 7230 §7 (Upgrade/Connection headers permit this even though
// WebSocket clients typically send a single token) — contains token,
// case-insensitively.
func headerContainsToken(header, token string) bool {
	for _, part := range strings.Split(header, ",") {
		if strings.EqualFold(strings.TrimSpace(part), token) {
			return true
		}
	}
	return false
}

// wsAcceptKey computes the Sec-WebSocket-Accept value for a client's
// Sec-WebSocket-Key, per RFC 6455 §1.3.
func wsAcceptKey(key string) string {
	h := sha1.New() //nolint:gosec // RFC 6455 mandates SHA-1 here; not a security use
	_, _ = io.WriteString(h, key+wsMagicGUID)
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}

// dial obtains a connection to addr, ready to send/receive RTPS messages —
// see transport_ws_dial.go (every platform except GOOS=js GOARCH=wasm,
// which performs the RFC 6455 client-side opening handshake itself over a
// real net.Conn) and transport_ws_browser.go (GOOS=js GOARCH=wasm, which
// asks the browser's own WebSocket object to do it) for the two platform
// backends — see this file's doc comment.

// wsBrowserURL builds the ws:// or wss:// URL a browser's native WebSocket
// constructor expects from addr (host:port, exactly the form every other
// WS-transport API in this package — WithWSAddr, WithWSPeers — already
// uses) and whether TLS was requested. Kept platform-neutral (unlike the
// rest of transport_ws_browser.go, which needs syscall/js and so only
// compiles for GOOS=js GOARCH=wasm) purely so it can be unit-tested on
// every platform without a browser or WASI runtime available.
func wsBrowserURL(addr string, tlsEnabled bool) string {
	scheme := "ws"
	if tlsEnabled {
		scheme = "wss"
	}
	return scheme + "://" + addr + "/"
}

// readLoop decodes WebSocket messages from wc until it errors, closes, or
// receives a Close frame, pushing each successfully decoded RTPS message
// onto recv. Mirrors tcpSocket.readConn/quicSocket.readFramedLoop's
// teardown-on-exit behaviour.
func (s *wsSocket) readLoop(wc wsConnIface, from string) {
	defer func() {
		_ = wc.close()
		s.dropConn(from)
	}()
	for {
		opcode, payload, err := wc.readMessage()
		if err != nil {
			return
		}
		if opcode == wsOpClose {
			return
		}
		data, ok := decodeWSPayload(opcode, payload)
		if !ok {
			// Malformed or unexpected-opcode message from a hostile or
			// buggy peer; drop the connection rather than the message
			// alone, the same policy readFramedLoop takes for a bad length
			// prefix.
			return
		}
		select {
		case s.recv <- wsPacket{data: data, from: from}:
		case <-s.done:
			return
		default: // slow consumer; drop
		}
	}
}

// decodeWSPayload extracts the RTPS message bytes from a decoded WebSocket
// message, dispatching on its opcode: BINARY carries the raw bytes
// directly (wsFramingBinary); TEXT carries a wsJSONMessage envelope
// (wsFramingJSON) that must be JSON- and then base64-decoded. Any other
// opcode (Ping/Pong/Continuation are already handled inside
// wsConn.readMessage and never reach here) or malformed envelope reports
// ok=false.
func decodeWSPayload(opcode byte, payload []byte) (data []byte, ok bool) {
	switch opcode {
	case wsOpBinary:
		return payload, true
	case wsOpText:
		var msg wsJSONMessage
		if err := json.Unmarshal(payload, &msg); err != nil {
			return nil, false
		}
		decoded, err := base64.StdEncoding.DecodeString(msg.Data)
		if err != nil {
			return nil, false
		}
		return decoded, true
	default:
		return nil, false
	}
}

// readMessage reads one complete WebSocket message from c, reassembling any
// continuation frames (RFC 6455 §5.4) and transparently answering Ping
// frames with a Pong (discarding Pong frames, which this transport never
// sends unsolicited) without returning them to the caller. Returns
// (wsOpClose, payload, nil) when a Close frame is received.
func (c *wsConn) readMessage() (opcode byte, payload []byte, err error) {
	var buf []byte
	msgOp := byte(0xFF) // unset until the first non-continuation data frame
	for {
		fin, op, frame, ferr := readWSFrame(c.br)
		if ferr != nil {
			return 0, nil, ferr
		}
		switch op {
		case wsOpPing:
			if werr := c.writeControlFrame(wsOpPong, frame); werr != nil {
				return 0, nil, werr
			}
			continue
		case wsOpPong:
			continue
		case wsOpClose:
			return wsOpClose, frame, nil
		case wsOpContinuation:
			buf = append(buf, frame...)
		default: // wsOpText / wsOpBinary: starts a new (possibly fragmented) message
			msgOp = op
			buf = append(buf, frame...)
		}
		if len(buf) > maxWSFrameSize {
			return 0, nil, fmt.Errorf("rtps: WS message too large: > %d bytes", maxWSFrameSize)
		}
		if fin {
			return msgOp, buf, nil
		}
	}
}

// writeMessage encodes data per framing and writes it to c as a single
// unfragmented WebSocket message (FIN=1), masked iff c.isClient — see this
// file's doc comment on the client/server masking rule. wmu serialises this
// against any concurrent writer of c (including readMessage's own Pong
// replies).
func (c *wsConn) writeMessage(framing wsFraming, data []byte) error {
	c.wmu.Lock()
	defer c.wmu.Unlock()
	if framing == wsFramingJSON {
		payload, err := json.Marshal(wsJSONMessage{Data: base64.StdEncoding.EncodeToString(data)})
		if err != nil {
			return err
		}
		return writeWSFrame(c.nc, wsOpText, payload, c.isClient)
	}
	return writeWSFrame(c.nc, wsOpBinary, data, c.isClient)
}

// writeControlFrame writes a control frame (Pong) to c, serialised the same
// way writeMessage is.
func (c *wsConn) writeControlFrame(opcode byte, payload []byte) error {
	c.wmu.Lock()
	defer c.wmu.Unlock()
	return writeWSFrame(c.nc, opcode, payload, c.isClient)
}

// readWSFrame reads and decodes exactly one WebSocket frame header plus
// payload from r (RFC 6455 §5.2), unmasking the payload first if the frame
// carries the MASK bit. It rejects any claimed payload length exceeding
// maxWSFrameSize before attempting to allocate a buffer for it, the same
// hostile-peer protection maxTCPFrameSize/maxQUICFrameSize give their own
// transports.
func readWSFrame(r *bufio.Reader) (fin bool, opcode byte, payload []byte, err error) {
	var hdr [2]byte
	if _, err = io.ReadFull(r, hdr[:]); err != nil {
		return false, 0, nil, err
	}
	fin = hdr[0]&0x80 != 0
	opcode = hdr[0] & 0x0F
	masked := hdr[1]&0x80 != 0
	plen := uint64(hdr[1] & 0x7F)

	switch plen {
	case 126:
		var ext [2]byte
		if _, err = io.ReadFull(r, ext[:]); err != nil {
			return false, 0, nil, err
		}
		plen = uint64(binary.BigEndian.Uint16(ext[:]))
	case 127:
		var ext [8]byte
		if _, err = io.ReadFull(r, ext[:]); err != nil {
			return false, 0, nil, err
		}
		plen = binary.BigEndian.Uint64(ext[:])
	}
	if plen > maxWSFrameSize {
		return false, 0, nil, fmt.Errorf("rtps: WS frame too large: %d bytes", plen)
	}

	var maskKey [4]byte
	if masked {
		if _, err = io.ReadFull(r, maskKey[:]); err != nil {
			return false, 0, nil, err
		}
	}

	payload = make([]byte, plen)
	if _, err = io.ReadFull(r, payload); err != nil {
		return false, 0, nil, err
	}
	if masked {
		for i := range payload {
			payload[i] ^= maskKey[i%4]
		}
	}
	return fin, opcode, payload, nil
}

// writeWSFrame writes a single, unfragmented (FIN=1) WebSocket frame
// carrying opcode/payload to w, masked with a fresh random key iff mask is
// true (RFC 6455 §5.1 forbids a server from masking and requires a client
// to always mask).
func writeWSFrame(w io.Writer, opcode byte, payload []byte, mask bool) error {
	n := len(payload)
	var hdr [10]byte
	hdr[0] = 0x80 | (opcode & 0x0F) // FIN=1, RSV1-3=0
	maskBit := byte(0)
	if mask {
		maskBit = 0x80
	}
	var hdrLen int
	switch {
	case n < 126:
		hdr[1] = byte(n) | maskBit
		hdrLen = 2
	case n <= 0xFFFF:
		hdr[1] = 126 | maskBit
		binary.BigEndian.PutUint16(hdr[2:4], uint16(n))
		hdrLen = 4
	default:
		hdr[1] = 127 | maskBit
		binary.BigEndian.PutUint64(hdr[2:10], uint64(n))
		hdrLen = 10
	}

	out := make([]byte, 0, hdrLen+4+n)
	out = append(out, hdr[:hdrLen]...)
	if mask {
		var maskKey [4]byte
		if _, err := rand.Read(maskKey[:]); err != nil {
			return err
		}
		out = append(out, maskKey[:]...)
		start := len(out)
		out = append(out, payload...)
		for i := 0; i < n; i++ {
			out[start+i] ^= maskKey[i%4]
		}
	} else {
		out = append(out, payload...)
	}
	_, err := w.Write(out)
	return err
}

// send encodes data per s.framing and writes it to addr as a single
// WebSocket message, dialling (or reusing a cached connection to) addr as
// needed. Writes to the same peer are serialised so concurrent senders
// never interleave frames, and a failed write drops the cached connection
// so the next send redials — the same policy tcpSocket.send/
// quicSocket.send use. Unlike quicSocket.send, there is no reliable/
// best-effort distinction: a WebSocket connection, like a TCP connection,
// is a single ordered reliable stream of messages.
func (s *wsSocket) send(addr string, data []byte) error {
	if len(data) > maxWSFrameSize {
		return fmt.Errorf("rtps: WS message too large: %d bytes", len(data))
	}
	wmu := s.peerLock(addr)
	wmu.Lock()
	defer wmu.Unlock()

	wc, err := s.connLocked(addr)
	if err != nil {
		return err
	}
	if err := wc.writeMessage(s.framing, data); err != nil {
		s.dropConn(addr)
		return fmt.Errorf("rtps: WS write %s: %w", addr, err)
	}
	return nil
}

// peerLock returns (creating on first use) the per-peer mutex for addr.
func (s *wsSocket) peerLock(addr string) *sync.Mutex {
	s.mu.Lock()
	defer s.mu.Unlock()
	wmu, ok := s.wmu[addr]
	if !ok {
		wmu = &sync.Mutex{}
		s.wmu[addr] = wmu
	}
	return wmu
}

// connLocked returns the cached connection for addr, dialling a fresh one
// (performing the full RFC 6455 opening handshake) if none is cached.
// Caller must hold the lock returned by peerLock(addr), which also
// serialises concurrent dials to the same peer.
func (s *wsSocket) connLocked(addr string) (wsConnIface, error) {
	s.mu.Lock()
	wc, ok := s.conns[addr]
	s.mu.Unlock()
	if ok {
		return wc, nil
	}

	wc, err := s.dial(addr)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	s.conns[addr] = wc
	s.mu.Unlock()
	go s.readLoop(wc, addr)
	return wc, nil
}

// cachedConnForIP returns the "host:port" key of a cached connection whose
// host matches ip, or "" if none is cached. This is participant.
// wsLocatorForIP's fallback for reaching a peer that advertised no
// pidWSLocator at all — a dial-only participant (ROADMAP.md "WebAssembly
// Target": a browser tab or edge function with no listener of its own; see
// newWSSocket and WithWSPeers' doc comment) is only ever reachable this
// way, over the exact connection it dialled in on, since it has no
// separately-dialable address for anyone to advertise or connect to in the
// first place. Safe to call from any goroutine.
func (s *wsSocket) cachedConnForIP(ip net.IP) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	for addr := range s.conns {
		host, _, err := net.SplitHostPort(addr)
		if err != nil {
			continue
		}
		if connIP := net.ParseIP(host); connIP != nil && connIP.Equal(ip) {
			return addr
		}
	}
	return ""
}

// dropConn closes and evicts the cached connection to addr, if any.
func (s *wsSocket) dropConn(addr string) {
	s.mu.Lock()
	wc, ok := s.conns[addr]
	if ok {
		delete(s.conns, addr)
	}
	s.mu.Unlock()
	if ok {
		_ = wc.close()
	}
}

// close shuts down the listener (if any — dial-only sockets, see
// newWSSocket, have none) and every cached connection.
func (s *wsSocket) close() {
	close(s.done)
	if s.ln != nil {
		_ = s.ln.Close()
	}
	s.mu.Lock()
	for _, wc := range s.conns {
		_ = wc.close()
	}
	s.conns = nil
	s.mu.Unlock()
}
