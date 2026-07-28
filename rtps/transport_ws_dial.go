// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

//go:build !(js && wasm)

package rtps

// The default RTPS-over-WebSocket client-dial backend (Milestone 16,
// ROADMAP.md "WebAssembly Target") — used on every platform except
// GOOS=js GOARCH=wasm (a real browser tab; see transport_ws_browser.go),
// including GOOS=wasip1 GOARCH=wasm (a Wasm build running under a WASI
// runtime such as Wasmtime, Fastly Compute, or a Cloudflare Worker), all of
// which get a real net.Conn from the standard library's net package and so
// need to perform the RFC 6455 opening handshake themselves exactly as any
// other platform does — see this file's dial for that handshake, split out
// of transport_ws.go verbatim (Milestone 16 "WebSocket Transport") so the
// two per-GOOS backends can live in their own build-tag-gated files instead
// of one intertwined with a runtime GOOS check.

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"
)

// dial performs the RFC 6455 client-side opening handshake against addr —
// dialling a fresh TCP/TLS connection, sending an HTTP Upgrade request with
// a fresh random Sec-WebSocket-Key, and verifying the server's response
// carries the matching Sec-WebSocket-Accept — bounded by wsDialTimeout,
// exactly as tcpSocket.connLocked/quicSocket.connLocked bound their own
// dials via context.WithTimeout.
func (s *wsSocket) dial(addr string) (wsConnIface, error) {
	ctx, cancel := context.WithTimeout(context.Background(), wsDialTimeout)
	defer cancel()

	var nc net.Conn
	var err error
	if s.tlsConfig != nil {
		nc, err = (&tls.Dialer{Config: s.tlsConfig}).DialContext(ctx, "tcp", addr)
	} else {
		nc, err = (&net.Dialer{}).DialContext(ctx, "tcp", addr)
	}
	if err != nil {
		return nil, fmt.Errorf("rtps: WS dial %s: %w", addr, err)
	}
	// The handshake round trip itself (request write + response read) is
	// bounded separately via a connection deadline, since it happens after
	// DialContext has already returned and ctx's cancellation no longer
	// applies to nc's I/O.
	_ = nc.SetDeadline(time.Now().Add(wsDialTimeout))

	keyBytes := make([]byte, 16)
	if _, keyErr := rand.Read(keyBytes); keyErr != nil {
		_ = nc.Close()
		return nil, fmt.Errorf("rtps: WS handshake key %s: %w", addr, keyErr)
	}
	secKey := base64.StdEncoding.EncodeToString(keyBytes)

	req := "GET / HTTP/1.1\r\n" +
		"Host: " + addr + "\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Key: " + secKey + "\r\n" +
		"Sec-WebSocket-Version: 13\r\n\r\n"
	if _, writeErr := io.WriteString(nc, req); writeErr != nil {
		_ = nc.Close()
		return nil, fmt.Errorf("rtps: WS handshake write %s: %w", addr, writeErr)
	}

	br := bufio.NewReader(nc)
	resp, err := http.ReadResponse(br, nil)
	if err != nil {
		_ = nc.Close()
		return nil, fmt.Errorf("rtps: WS handshake read %s: %w", addr, err)
	}
	// A "101 Switching Protocols" response carries no body by definition
	// (RFC 7230 §3.3.3); net/http's own ReadResponse already sets Body to
	// http.NoBody for any 1xx status, so this Close is a documented no-op
	// that touches neither br nor nc — just satisfies the "always close a
	// response body" contract explicitly rather than relying on that
	// internal behaviour silently.
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusSwitchingProtocols {
		_ = nc.Close()
		return nil, fmt.Errorf("rtps: WS handshake %s: unexpected status %d", addr, resp.StatusCode)
	}
	if got := resp.Header.Get("Sec-WebSocket-Accept"); got != wsAcceptKey(secKey) {
		_ = nc.Close()
		return nil, fmt.Errorf("rtps: WS handshake %s: invalid Sec-WebSocket-Accept", addr)
	}
	_ = nc.SetDeadline(time.Time{})

	return &wsConn{nc: nc, br: br, isClient: true}, nil
}
