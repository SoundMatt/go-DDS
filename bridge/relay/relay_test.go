// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package relay

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/binary"
	"encoding/pem"
	"math/big"
	"net"
	"testing"
	"time"
)

// testClient is a minimal hand-rolled relay client used only to exercise
// [Server] from both sides without depending on the root module's rtps
// package (which independently implements the client side of this same
// protocol — see relay.go's package doc comment).
type testClient struct {
	t    *testing.T
	conn net.Conn
}

func dialTestClient(t *testing.T, addr, id string, tlsConfig *tls.Config) *testClient {
	t.Helper()
	var conn net.Conn
	var err error
	if tlsConfig != nil {
		conn, err = (&tls.Dialer{Config: tlsConfig}).DialContext(context.Background(), "tcp", addr)
	} else {
		conn, err = (&net.Dialer{}).DialContext(context.Background(), "tcp", addr)
	}
	if err != nil {
		t.Fatalf("dial %s: %v", addr, err)
	}
	c := &testClient{t: t, conn: conn}
	if err := writeFrame(conn, frameRegister, []byte(id), nil); err != nil {
		t.Fatalf("register %s: %v", id, err)
	}
	return c
}

func (c *testClient) send(targetID string, payload []byte) {
	c.t.Helper()
	if err := writeFrame(c.conn, frameSend, []byte(targetID), payload); err != nil {
		c.t.Fatalf("send: %v", err)
	}
}

func (c *testClient) recv() (typ byte, fromID string, payload []byte, err error) {
	c.t.Helper()
	_ = c.conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	t, id, p, err := readFrame(c.conn)
	return t, string(id), p, err
}

func (c *testClient) close() { _ = c.conn.Close() }

func TestServer_ForwardsFrameBetweenTwoPeers(t *testing.T) {
	srv, err := Serve("127.0.0.1:0", Options{})
	if err != nil {
		t.Fatalf("Serve: %v", err)
	}
	defer func() { _ = srv.Close() }()

	alice := dialTestClient(t, srv.Addr(), "alice", nil)
	defer alice.close()
	bob := dialTestClient(t, srv.Addr(), "bob", nil)
	defer bob.close()

	// Give both registrations time to land before racing the forward.
	time.Sleep(50 * time.Millisecond)

	want := []byte("rtps-frame-opaque-bytes")
	alice.send("bob", want)

	typ, from, payload, err := bob.recv()
	if err != nil {
		t.Fatalf("bob.recv: %v", err)
	}
	if typ != frameDeliver {
		t.Errorf("frame type = %#x, want frameDeliver", typ)
	}
	if from != "alice" {
		t.Errorf("from = %q, want %q", from, "alice")
	}
	if !bytes.Equal(payload, want) {
		t.Errorf("payload = %q, want %q", payload, want)
	}
}

func TestServer_BidirectionalForwarding(t *testing.T) {
	srv, err := Serve("127.0.0.1:0", Options{})
	if err != nil {
		t.Fatalf("Serve: %v", err)
	}
	defer func() { _ = srv.Close() }()

	alice := dialTestClient(t, srv.Addr(), "alice", nil)
	defer alice.close()
	bob := dialTestClient(t, srv.Addr(), "bob", nil)
	defer bob.close()
	time.Sleep(50 * time.Millisecond)

	alice.send("bob", []byte("ping"))
	if _, _, payload, err := bob.recv(); err != nil || string(payload) != "ping" {
		t.Fatalf("bob.recv: payload=%q err=%v", payload, err)
	}

	bob.send("alice", []byte("pong"))
	if _, _, payload, err := alice.recv(); err != nil || string(payload) != "pong" {
		t.Fatalf("alice.recv: payload=%q err=%v", payload, err)
	}
}

func TestServer_UnknownTargetReturnsError(t *testing.T) {
	srv, err := Serve("127.0.0.1:0", Options{})
	if err != nil {
		t.Fatalf("Serve: %v", err)
	}
	defer func() { _ = srv.Close() }()

	alice := dialTestClient(t, srv.Addr(), "alice", nil)
	defer alice.close()
	time.Sleep(20 * time.Millisecond)

	alice.send("nobody", []byte("hello?"))

	typ, _, payload, err := alice.recv()
	if err != nil {
		t.Fatalf("recv: %v", err)
	}
	if typ != frameError {
		t.Errorf("frame type = %#x, want frameError", typ)
	}
	if string(payload) != ErrNoSuchPeer {
		t.Errorf("payload = %q, want %q", payload, ErrNoSuchPeer)
	}
}

func TestServer_ReregisterReplacesOldConnection(t *testing.T) {
	srv, err := Serve("127.0.0.1:0", Options{})
	if err != nil {
		t.Fatalf("Serve: %v", err)
	}
	defer func() { _ = srv.Close() }()

	first := dialTestClient(t, srv.Addr(), "dup", nil)
	time.Sleep(20 * time.Millisecond)
	second := dialTestClient(t, srv.Addr(), "dup", nil)
	defer second.close()
	time.Sleep(20 * time.Millisecond)

	// The first connection must have been closed by the server.
	_ = first.conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, 1)
	if _, err := first.conn.Read(buf); err == nil {
		t.Error("expected the superseded connection to be closed")
	}

	// The second (current) registration must still be reachable.
	sender := dialTestClient(t, srv.Addr(), "sender", nil)
	defer sender.close()
	time.Sleep(20 * time.Millisecond)
	sender.send("dup", []byte("hi"))
	if _, from, payload, err := second.recv(); err != nil || from != "sender" || string(payload) != "hi" {
		t.Fatalf("second.recv: from=%q payload=%q err=%v", from, payload, err)
	}
}

func TestServer_Peers(t *testing.T) {
	srv, err := Serve("127.0.0.1:0", Options{})
	if err != nil {
		t.Fatalf("Serve: %v", err)
	}
	defer func() { _ = srv.Close() }()

	a := dialTestClient(t, srv.Addr(), "a", nil)
	defer a.close()
	b := dialTestClient(t, srv.Addr(), "b", nil)
	defer b.close()
	time.Sleep(50 * time.Millisecond)

	peers := srv.Peers()
	if len(peers) != 2 {
		t.Fatalf("Peers() = %v, want 2 entries", peers)
	}
	seen := map[string]bool{}
	for _, id := range peers {
		seen[id] = true
	}
	if !seen["a"] || !seen["b"] {
		t.Errorf("Peers() = %v, want [a b]", peers)
	}
}

func TestServer_TLS(t *testing.T) {
	certPEM, keyPEM := generateTestCertKey(t)
	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		t.Fatalf("X509KeyPair: %v", err)
	}
	pool := x509.NewCertPool()
	pool.AppendCertsFromPEM(certPEM)

	srv, err := Serve("127.0.0.1:0", Options{TLS: &tls.Config{Certificates: []tls.Certificate{cert}}})
	if err != nil {
		t.Fatalf("Serve: %v", err)
	}
	defer func() { _ = srv.Close() }()

	clientCfg := &tls.Config{RootCAs: pool, ServerName: "go-dds-relay-test"}
	alice := dialTestClient(t, srv.Addr(), "alice", clientCfg)
	defer alice.close()
	bob := dialTestClient(t, srv.Addr(), "bob", clientCfg)
	defer bob.close()
	time.Sleep(50 * time.Millisecond)

	alice.send("bob", []byte("over tls"))
	if _, _, payload, err := bob.recv(); err != nil || string(payload) != "over tls" {
		t.Fatalf("bob.recv: payload=%q err=%v", payload, err)
	}
}

func TestServer_CloseInterruptsClients(t *testing.T) {
	srv, err := Serve("127.0.0.1:0", Options{})
	if err != nil {
		t.Fatalf("Serve: %v", err)
	}
	alice := dialTestClient(t, srv.Addr(), "alice", nil)
	defer alice.close()
	time.Sleep(20 * time.Millisecond)

	if err := srv.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	_ = alice.conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, 1)
	if _, err := alice.conn.Read(buf); err == nil {
		t.Error("expected client connection to be closed after Server.Close")
	}
}

func TestReadFrame_TooLarge(t *testing.T) {
	var buf bytes.Buffer
	var hdr [4]byte
	binary.BigEndian.PutUint32(hdr[:], maxFrameBytes+1)
	buf.Write(hdr[:])
	if _, _, _, err := readFrame(&buf); err == nil {
		t.Fatal("expected ErrFrameTooLarge")
	}
}

func TestWriteFrame_IDTooLarge(t *testing.T) {
	var buf bytes.Buffer
	big := make([]byte, int(maxIDBytes)+1)
	if err := writeFrame(&buf, frameSend, big, nil); err == nil {
		t.Fatal("expected an error for an oversized id field")
	}
}

func TestReadFrame_UnknownType(t *testing.T) {
	var buf bytes.Buffer
	if err := writeFrame(&buf, 0x99, []byte("x"), nil); err != nil {
		t.Fatalf("writeFrame: %v", err)
	}
	if _, _, _, err := readFrame(&buf); err != ErrUnknownFrameType {
		t.Fatalf("readFrame err = %v, want ErrUnknownFrameType", err)
	}
}

func TestServer_MalformedRegistrationDropsConnection(t *testing.T) {
	srv, err := Serve("127.0.0.1:0", Options{})
	if err != nil {
		t.Fatalf("Serve: %v", err)
	}
	defer func() { _ = srv.Close() }()

	conn, err := (&net.Dialer{}).DialContext(context.Background(), "tcp", srv.Addr())
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() { _ = conn.Close() }()
	// Send a SEND frame first instead of REGISTER.
	if err := writeFrame(conn, frameSend, []byte("bob"), []byte("x")); err != nil {
		t.Fatalf("writeFrame: %v", err)
	}
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, 1)
	if _, err := conn.Read(buf); err == nil {
		t.Error("expected the server to drop a connection that skips registration")
	}
}

// generateTestCertKey returns an ephemeral, in-memory self-signed ECDSA
// certificate/key pair (PEM-encoded), valid only for this test process.
func generateTestCertKey(t *testing.T) (certPEM, keyPEM []byte) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "go-dds-relay-test"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		DNSNames:     []string{"go-dds-relay-test"},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("CreateCertificate: %v", err)
	}
	certPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatalf("MarshalECPrivateKey: %v", err)
	}
	keyPEM = pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	return certPEM, keyPEM
}
