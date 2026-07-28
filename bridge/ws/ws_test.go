// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Internal tests (package ws, not ws_test) so testClient can drive the raw
// RFC 6455 wire protocol directly via this package's own frame codec
// (wsframe.go) rather than depending on an external WebSocket client
// library go-DDS/bridge does not otherwise need.

package ws

//fusa:test REQ-BRIDGE-015
//fusa:test REQ-BRIDGE-016

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	dds "github.com/SoundMatt/go-DDS"
	"github.com/SoundMatt/go-DDS/mock"
)

func newTestBridge(t *testing.T, opts Options) (*Bridge, *httptest.Server) {
	t.Helper()
	p, err := mock.New(dds.Domain(0))
	if err != nil {
		t.Fatalf("mock.New: %v", err)
	}
	b := New(p, opts)
	srv := httptest.NewServer(b)
	t.Cleanup(func() {
		srv.Close()
		_ = b.Close()
		_ = p.Close()
	})
	return b, srv
}

// testClient is a minimal WebSocket client built directly on this package's
// own frame codec, used to drive Bridge.ServeHTTP as an external client
// would.
type testClient struct {
	t    *testing.T
	conn net.Conn
	br   *bufio.Reader
}

func dialWS(t *testing.T, srvURL, authToken string) *testClient {
	t.Helper()
	host, target := splitTestURL(t, srvURL)
	conn, err := (&net.Dialer{}).DialContext(context.Background(), "tcp", host)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	keyBytes := make([]byte, 16)
	if _, keyErr := rand.Read(keyBytes); keyErr != nil {
		t.Fatalf("rand.Read: %v", keyErr)
	}
	secKey := base64.StdEncoding.EncodeToString(keyBytes)
	req := "GET " + target + " HTTP/1.1\r\n" +
		"Host: " + host + "\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Key: " + secKey + "\r\n" +
		"Sec-WebSocket-Version: 13\r\n"
	if authToken != "" {
		req += "Authorization: Bearer " + authToken + "\r\n"
	}
	req += "\r\n"
	if _, writeErr := conn.Write([]byte(req)); writeErr != nil {
		t.Fatalf("write handshake: %v", writeErr)
	}
	br := bufio.NewReader(conn)
	resp, err := http.ReadResponse(br, nil)
	if err != nil {
		t.Fatalf("ReadResponse: %v", err)
	}
	// A "101 Switching Protocols" response carries no body by definition
	// (RFC 7230 §3.3.3); net/http sets Body to http.NoBody for any 1xx
	// status, so this Close is a documented no-op, not a real I/O op on br/conn.
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusSwitchingProtocols {
		conn.Close()
		t.Fatalf("handshake status: got %d, want %d", resp.StatusCode, http.StatusSwitchingProtocols)
	}
	if got := resp.Header.Get("Sec-WebSocket-Accept"); got != acceptKey(secKey) {
		t.Fatalf("Sec-WebSocket-Accept: got %q, want %q", got, acceptKey(secKey))
	}
	return &testClient{t: t, conn: conn, br: br}
}

// dialWSExpectRejected performs the TCP dial and raw HTTP request but
// returns the *http.Response instead of asserting a 101 — for tests that
// expect the handshake itself to be rejected (e.g. a bad/missing auth
// token). Callers must close the returned Response's Body (e.g. `defer
// resp.Body.Close()`).
func dialWSExpectRejected(t *testing.T, srvURL, authToken string) *http.Response {
	t.Helper()
	host, target := splitTestURL(t, srvURL)
	conn, err := (&net.Dialer{}).DialContext(context.Background(), "tcp", host)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	keyBytes := make([]byte, 16)
	_, _ = rand.Read(keyBytes)
	secKey := base64.StdEncoding.EncodeToString(keyBytes)
	req := "GET " + target + " HTTP/1.1\r\n" +
		"Host: " + host + "\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Key: " + secKey + "\r\n" +
		"Sec-WebSocket-Version: 13\r\n"
	if authToken != "" {
		req += "Authorization: Bearer " + authToken + "\r\n"
	}
	req += "\r\n"
	if _, writeErr := conn.Write([]byte(req)); writeErr != nil {
		t.Fatalf("write handshake: %v", writeErr)
	}
	resp, err := http.ReadResponse(bufio.NewReader(conn), nil)
	if err != nil {
		t.Fatalf("ReadResponse: %v", err)
	}
	return resp
}

// splitTestURL splits an httptest.Server URL (optionally with a query
// string appended by the caller, e.g. "http://127.0.0.1:PORT?token=x") into
// the bare host:port to dial and the HTTP request-target (path + query) to
// send on the request line.
func splitTestURL(t *testing.T, srvURL string) (host, target string) {
	t.Helper()
	u, err := url.Parse(srvURL)
	if err != nil {
		t.Fatalf("url.Parse(%q): %v", srvURL, err)
	}
	target = u.Path
	if target == "" {
		target = "/"
	}
	if u.RawQuery != "" {
		target += "?" + u.RawQuery
	}
	return u.Host, target
}

func (c *testClient) sendJSON(v any) {
	c.t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		c.t.Fatalf("Marshal: %v", err)
	}
	// A real client masks every frame it sends (RFC 6455 §5.1); this
	// package's frame codec is server-role-only (see wsframe.go's doc
	// comment) and doesn't require the MASK bit on read, so an unmasked
	// test frame decodes identically — see readFrame.
	if err := writeFrame(c.conn, opText, b); err != nil {
		c.t.Fatalf("writeFrame: %v", err)
	}
}

func (c *testClient) recvJSON(v any) {
	c.t.Helper()
	_ = c.conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	op, payload, err := readMessage(c.br, c.conn)
	if err != nil {
		c.t.Fatalf("readMessage: %v", err)
	}
	if op != opText && op != opBinary {
		c.t.Fatalf("unexpected opcode %d", op)
	}
	if err := json.Unmarshal(payload, v); err != nil {
		c.t.Fatalf("Unmarshal %q: %v", payload, err)
	}
}

// recvJSONTimeout is like recvJSON but reports ok=false on a read timeout
// instead of failing the test — for asserting that nothing arrives.
func (c *testClient) recvJSONTimeout(v any, d time.Duration) (ok bool) {
	c.t.Helper()
	_ = c.conn.SetReadDeadline(time.Now().Add(d))
	op, payload, err := readMessage(c.br, c.conn)
	if err != nil {
		return false
	}
	if op != opText && op != opBinary {
		return false
	}
	if err := json.Unmarshal(payload, v); err != nil {
		c.t.Fatalf("Unmarshal %q: %v", payload, err)
	}
	return true
}

func (c *testClient) close() { _ = c.conn.Close() }

// ── subscribe / publish round trip ────────────────────────────────────────────

func TestBridge_SubscribePublish_RoundTrip(t *testing.T) {
	_, srv := newTestBridge(t, Options{})

	sub := dialWS(t, srv.URL, "")
	defer sub.close()
	sub.sendJSON(clientMessage{Op: "subscribe", Topic: "ws/roundtrip"})
	var ack serverMessage
	sub.recvJSON(&ack)
	if ack.Op != "subscribed" || ack.Topic != "ws/roundtrip" {
		t.Fatalf("subscribe ack: got %+v", ack)
	}

	pub := dialWS(t, srv.URL, "")
	defer pub.close()
	payload := base64.StdEncoding.EncodeToString([]byte("hello-ws-bridge"))
	pub.sendJSON(clientMessage{Op: "publish", Topic: "ws/roundtrip", Data: payload})

	var sample serverMessage
	sub.recvJSON(&sample)
	if sample.Op != "sample" || sample.Topic != "ws/roundtrip" {
		t.Fatalf("sample message: got %+v", sample)
	}
	got, err := base64.StdEncoding.DecodeString(sample.Data)
	if err != nil {
		t.Fatalf("decode sample data: %v", err)
	}
	if string(got) != "hello-ws-bridge" {
		t.Errorf("payload: got %q, want %q", got, "hello-ws-bridge")
	}
}

func TestBridge_MultipleSubscribers_BothReceive(t *testing.T) {
	_, srv := newTestBridge(t, Options{})

	subA := dialWS(t, srv.URL, "")
	defer subA.close()
	subA.sendJSON(clientMessage{Op: "subscribe", Topic: "ws/fanout"})
	var ackA serverMessage
	subA.recvJSON(&ackA)

	subB := dialWS(t, srv.URL, "")
	defer subB.close()
	subB.sendJSON(clientMessage{Op: "subscribe", Topic: "ws/fanout"})
	var ackB serverMessage
	subB.recvJSON(&ackB)

	pub := dialWS(t, srv.URL, "")
	defer pub.close()
	payload := base64.StdEncoding.EncodeToString([]byte("fanout-message"))
	pub.sendJSON(clientMessage{Op: "publish", Topic: "ws/fanout", Data: payload})

	var sampleA, sampleB serverMessage
	subA.recvJSON(&sampleA)
	subB.recvJSON(&sampleB)
	if sampleA.Data != payload {
		t.Errorf("subscriber A: got %q, want %q", sampleA.Data, payload)
	}
	if sampleB.Data != payload {
		t.Errorf("subscriber B: got %q, want %q", sampleB.Data, payload)
	}
}

func TestBridge_Unsubscribe_StopsDelivery(t *testing.T) {
	_, srv := newTestBridge(t, Options{})

	sub := dialWS(t, srv.URL, "")
	defer sub.close()
	sub.sendJSON(clientMessage{Op: "subscribe", Topic: "ws/unsub"})
	var ack serverMessage
	sub.recvJSON(&ack)

	sub.sendJSON(clientMessage{Op: "unsubscribe", Topic: "ws/unsub"})
	var unsubAck serverMessage
	sub.recvJSON(&unsubAck)
	if unsubAck.Op != "unsubscribed" || unsubAck.Topic != "ws/unsub" {
		t.Fatalf("unsubscribe ack: got %+v", unsubAck)
	}

	pub := dialWS(t, srv.URL, "")
	defer pub.close()
	pub.sendJSON(clientMessage{Op: "publish", Topic: "ws/unsub", Data: base64.StdEncoding.EncodeToString([]byte("should-not-arrive"))})

	var stray serverMessage
	if sub.recvJSONTimeout(&stray, 300*time.Millisecond) {
		t.Fatalf("expected no message after unsubscribe, got %+v", stray)
	}
}

func TestBridge_Publish_InvalidBase64_ReturnsError(t *testing.T) {
	_, srv := newTestBridge(t, Options{})
	c := dialWS(t, srv.URL, "")
	defer c.close()

	c.sendJSON(clientMessage{Op: "publish", Topic: "ws/bad", Data: "not-valid-base64!!!"})
	var resp serverMessage
	c.recvJSON(&resp)
	if resp.Op != "error" {
		t.Fatalf("expected an error message, got %+v", resp)
	}
}

func TestBridge_Subscribe_EmptyTopic_ReturnsError(t *testing.T) {
	_, srv := newTestBridge(t, Options{})
	c := dialWS(t, srv.URL, "")
	defer c.close()

	c.sendJSON(clientMessage{Op: "subscribe", Topic: ""})
	var resp serverMessage
	c.recvJSON(&resp)
	if resp.Op != "error" {
		t.Fatalf("expected an error message, got %+v", resp)
	}
}

func TestBridge_UnknownOp_ReturnsError(t *testing.T) {
	_, srv := newTestBridge(t, Options{})
	c := dialWS(t, srv.URL, "")
	defer c.close()

	c.sendJSON(clientMessage{Op: "frobnicate", Topic: "x"})
	var resp serverMessage
	c.recvJSON(&resp)
	if resp.Op != "error" {
		t.Fatalf("expected an error message, got %+v", resp)
	}
}

// ── auth ─────────────────────────────────────────────────────────────────────

func TestBridge_AuthToken_RejectsMissingToken(t *testing.T) {
	_, srv := newTestBridge(t, Options{AuthToken: "secret-token"})
	resp := dialWSExpectRejected(t, srv.URL, "")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status: got %d, want %d", resp.StatusCode, http.StatusUnauthorized)
	}
}

func TestBridge_AuthToken_RejectsWrongToken(t *testing.T) {
	_, srv := newTestBridge(t, Options{AuthToken: "secret-token"})
	resp := dialWSExpectRejected(t, srv.URL, "wrong-token")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status: got %d, want %d", resp.StatusCode, http.StatusUnauthorized)
	}
}

func TestBridge_AuthToken_AcceptsCorrectToken(t *testing.T) {
	_, srv := newTestBridge(t, Options{AuthToken: "secret-token"})
	c := dialWS(t, srv.URL, "secret-token")
	defer c.close()
	c.sendJSON(clientMessage{Op: "subscribe", Topic: "ws/authed"})
	var ack serverMessage
	c.recvJSON(&ack)
	if ack.Op != "subscribed" {
		t.Fatalf("subscribe ack: got %+v", ack)
	}
}

// TestBridge_AuthToken_AcceptsQueryParam proves the browser-compatible
// fallback (see Bridge.authorize's doc comment): a page's own JavaScript
// cannot set an Authorization header on a WebSocket handshake, so a
// "?token=" query parameter must work too.
func TestBridge_AuthToken_AcceptsQueryParam(t *testing.T) {
	_, srv := newTestBridge(t, Options{AuthToken: "secret-token"})
	c := dialWS(t, srv.URL+"?token=secret-token", "")
	defer c.close()
	c.sendJSON(clientMessage{Op: "subscribe", Topic: "ws/authed-query"})
	var ack serverMessage
	c.recvJSON(&ack)
	if ack.Op != "subscribed" {
		t.Fatalf("subscribe ack: got %+v", ack)
	}
}

func TestBridge_AuthToken_RejectsWrongQueryParam(t *testing.T) {
	_, srv := newTestBridge(t, Options{AuthToken: "secret-token"})
	resp := dialWSExpectRejected(t, srv.URL+"?token=nope", "")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status: got %d, want %d", resp.StatusCode, http.StatusUnauthorized)
	}
}

// ── lifecycle ────────────────────────────────────────────────────────────────

func TestBridge_Close_ClosesActiveSessions(t *testing.T) {
	b, srv := newTestBridge(t, Options{})
	c := dialWS(t, srv.URL, "")
	defer c.close()
	c.sendJSON(clientMessage{Op: "subscribe", Topic: "ws/closer"})
	var ack serverMessage
	c.recvJSON(&ack)

	if err := b.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	_ = c.conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, 1)
	if _, err := c.conn.Read(buf); err == nil {
		t.Error("expected the connection to be closed after Bridge.Close, but Read succeeded")
	}
}
