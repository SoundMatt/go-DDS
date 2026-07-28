// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Internal tests for the RTPS-over-DTLS transport (Milestone 14). Package
// rtps (internal) for direct access to dtlsSocket, participant fields, and
// the dispatch helpers. generateTestTLSConfig (self-signed ECDSA cert +
// matching CA pool) is shared with transport_tcp_test.go — DTLS reuses the
// same stdlib *tls.Config shape as RTPS-over-TCP (see WithDTLS).

package rtps

//fusa:test REQ-TRANS-007
//fusa:test REQ-TRANS-008
//fusa:test REQ-TRANS-009
//fusa:test REQ-SEC-026

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"runtime"
	"testing"
	"time"

	dds "github.com/SoundMatt/go-DDS"
	"github.com/SoundMatt/go-DDS/security"
)

// testCertKeyCA generates an ephemeral, in-memory CA and a leaf certificate
// signed by it (ECDSA P-256, valid only for this test process), PEM-encoded
// exactly as security.NewCertPlugin expects: a CERTIFICATE block for
// certPEM, an EC PRIVATE KEY block for keyPEM, and a CERTIFICATE block for
// the CA in caPEM.
func testCertKeyCA(t *testing.T) (certPEM, keyPEM, caPEM []byte) {
	t.Helper()

	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey (CA): %v", err)
	}
	caTmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "go-dds-certplugin-test-ca"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTmpl, caTmpl, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("CreateCertificate (CA): %v", err)
	}
	caCert, err := x509.ParseCertificate(caDER)
	if err != nil {
		t.Fatalf("ParseCertificate (CA): %v", err)
	}
	caPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER})

	leafKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey (leaf): %v", err)
	}
	leafTmpl := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: "go-dds-certplugin-test"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		DNSNames:     []string{"go-dds-certplugin-test"},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
	}
	leafDER, err := x509.CreateCertificate(rand.Reader, leafTmpl, caCert, &leafKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("CreateCertificate (leaf): %v", err)
	}
	certPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: leafDER})

	keyDER, err := x509.MarshalECPrivateKey(leafKey)
	if err != nil {
		t.Fatalf("MarshalECPrivateKey: %v", err)
	}
	keyPEM = pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})

	return certPEM, keyPEM, caPEM
}

// ── dtlsSocket primitives ────────────────────────────────────────────────────

func newTestDTLSSocket(t *testing.T, tlsConfig *tls.Config) *dtlsSocket {
	t.Helper()
	s, err := newDTLSSocket("127.0.0.1:0", tlsConfig)
	if err != nil {
		t.Skipf("newDTLSSocket: %v — UDP loopback unavailable", err)
	}
	t.Cleanup(s.close)
	return s
}

func TestDTLSSocket_SendReceive(t *testing.T) {
	cfg := generateTestTLSConfig(t)
	server := newTestDTLSSocket(t, cfg)
	client := newTestDTLSSocket(t, cfg)

	addr := fmt.Sprintf("127.0.0.1:%d", server.port)
	want := []byte("rtps-over-dtls: hello")
	if err := client.send(addr, want); err != nil {
		t.Fatalf("send: %v", err)
	}

	select {
	case pkt := <-server.recv:
		if !bytes.Equal(pkt.data, want) {
			t.Errorf("payload: got %q, want %q", pkt.data, want)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for DTLS record")
	}
}

func TestDTLSSocket_SendReceive_MultipleRecords(t *testing.T) {
	cfg := generateTestTLSConfig(t)
	server := newTestDTLSSocket(t, cfg)
	client := newTestDTLSSocket(t, cfg)
	addr := fmt.Sprintf("127.0.0.1:%d", server.port)

	const n = 20
	for i := 0; i < n; i++ {
		msg := []byte(fmt.Sprintf("record-%02d", i))
		if err := client.send(addr, msg); err != nil {
			t.Fatalf("send %d: %v", i, err)
		}
	}
	for i := 0; i < n; i++ {
		select {
		case pkt := <-server.recv:
			want := fmt.Sprintf("record-%02d", i)
			if string(pkt.data) != want {
				t.Errorf("record %d: got %q, want %q", i, pkt.data, want)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("timeout waiting for record %d", i)
		}
	}
}

// TestDTLSSocket_MutualAuthEnforcedByDefault proves REQ-TRANS-007: supplying
// a ClientCAs pool with ClientAuth left unset upgrades enforcement to
// RequireAndVerifyClientCert, so a client with no certificate at all is
// rejected during the handshake rather than silently accepted.
func TestDTLSSocket_MutualAuthEnforcedByDefault(t *testing.T) {
	cfg := generateTestTLSConfig(t)
	// Server config: has Certificates (to serve) and ClientCAs (so it can
	// verify a client cert) but ClientAuth left at its zero value —
	// dtlsConfigFromTLS must upgrade this to RequireAndVerifyClientCert.
	serverCfg := &tls.Config{
		Certificates: cfg.Certificates,
		ClientCAs:    cfg.RootCAs,
	}
	server := newTestDTLSSocket(t, serverCfg)

	if server.effectiveClientAuth != tls.RequireAndVerifyClientCert {
		t.Fatalf("effectiveClientAuth: got %v, want RequireAndVerifyClientCert", server.effectiveClientAuth)
	}

	// A client with no certificate at all must fail the handshake against a
	// server that requires and verifies one.
	noCertClientCfg := &tls.Config{RootCAs: cfg.RootCAs, ServerName: "go-dds-tcp-test"}
	client := newTestDTLSSocket(t, noCertClientCfg)
	addr := fmt.Sprintf("127.0.0.1:%d", server.port)
	if err := client.send(addr, []byte("should not be accepted")); err == nil {
		t.Fatal("expected handshake failure for a client with no certificate, got nil error")
	}
}

func TestDTLSSocket_Send_RecordTooLarge(t *testing.T) {
	cfg := generateTestTLSConfig(t)
	client := newTestDTLSSocket(t, cfg)
	big := make([]byte, maxUDPSize+1)
	if err := client.send("127.0.0.1:1", big); err == nil {
		t.Fatal("expected error for oversized record, got nil")
	}
}

// TestDTLSSocket_Send_HandshakeTimeout proves REQ-TRANS-009: dialling a
// DTLS peer that never responds fails within a bounded time rather than
// hanging forever.
func TestDTLSSocket_Send_HandshakeTimeout(t *testing.T) {
	cfg := generateTestTLSConfig(t)
	client := newTestDTLSSocket(t, cfg)

	// Bind a plain UDP socket that never speaks DTLS, so the handshake gets
	// no response and must time out rather than hang.
	deadEnd, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1")})
	if err != nil {
		t.Skipf("ListenUDP: %v", err)
	}
	defer deadEnd.Close()
	addr := deadEnd.LocalAddr().String()

	done := make(chan error, 1)
	go func() { done <- client.send(addr, []byte("nobody home")) }()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected handshake timeout error, got nil")
		}
	case <-time.After(dtlsDialTimeout + 5*time.Second):
		t.Fatal("DTLS dial did not time out within dtlsDialTimeout + margin")
	}
}

// ── security.CertPlugin TLS material export (REQ-SEC-026) ───────────────────

func TestCertPlugin_TLSCertificate_CAPool(t *testing.T) {
	certPEM, keyPEM, caPEM := testCertKeyCA(t)
	plugin, err := security.NewCertPlugin(certPEM, keyPEM, caPEM)
	if err != nil {
		t.Fatalf("NewCertPlugin: %v", err)
	}

	tlsCert := plugin.TLSCertificate()
	if len(tlsCert.Certificate) != 1 || len(tlsCert.Certificate[0]) == 0 {
		t.Fatal("TLSCertificate: empty DER chain")
	}
	if tlsCert.PrivateKey == nil {
		t.Fatal("TLSCertificate: nil PrivateKey")
	}
	if tlsCert.Leaf == nil {
		t.Fatal("TLSCertificate: nil Leaf")
	}

	pool := plugin.CAPool()
	if pool == nil {
		t.Fatal("CAPool: nil")
	}

	// The exported material must actually work as a *tls.Config for the
	// DTLS transport — that is the entire point of REQ-SEC-026.
	dtlsCfg := &tls.Config{
		Certificates: []tls.Certificate{tlsCert},
		RootCAs:      pool,
		ClientCAs:    pool,
		ServerName:   "go-dds-certplugin-test",
	}
	server := newTestDTLSSocket(t, dtlsCfg)
	client := newTestDTLSSocket(t, dtlsCfg)
	addr := fmt.Sprintf("127.0.0.1:%d", server.port)
	want := []byte("certplugin-identity-over-dtls")
	if err := client.send(addr, want); err != nil {
		t.Fatalf("send: %v", err)
	}
	select {
	case pkt := <-server.recv:
		if !bytes.Equal(pkt.data, want) {
			t.Errorf("payload: got %q, want %q", pkt.data, want)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for DTLS record using CertPlugin identity")
	}
}

// ── participant wiring ───────────────────────────────────────────────────────

func TestParticipant_DTLSLocatorForIP(t *testing.T) {
	p := &participant{dtlsPeers: []string{"127.0.0.1:17600", "10.0.0.9:17601"}}

	if got := p.dtlsLocatorForIP(net.ParseIP("127.0.0.1")); got != "127.0.0.1:17600" {
		t.Errorf("dtlsLocatorForIP: got %q, want %q", got, "127.0.0.1:17600")
	}
	if got := p.dtlsLocatorForIP(net.ParseIP("192.168.1.1")); got != "" {
		t.Errorf("dtlsLocatorForIP(unknown): got %q, want empty", got)
	}
}

func TestDTLS_Disabled_ByDefault(t *testing.T) {
	p := testPart(t)
	if p.dtlsSock != nil {
		t.Fatal("dtlsSock should be nil without WithDTLSAddr")
	}
	if got := p.dtlsLocatorForIP(net.ParseIP("127.0.0.1")); got != "" {
		t.Errorf("dtlsLocatorForIP without WithDTLSPeers: got %q, want empty", got)
	}
}

// TestDTLS_WithDTLS_StartsCleanly confirms a participant can be built with
// the DTLS transport configured and shuts down cleanly, without requiring a
// live peer.
func TestDTLS_WithDTLS_StartsCleanly(t *testing.T) {
	cfg := generateTestTLSConfig(t)
	p, err := newParticipant(dds.Domain(95),
		WithDTLSAddr("127.0.0.1:17610"),
		WithDTLS(cfg),
	)
	if err != nil {
		t.Skipf("newParticipant: %v — UDP loopback unavailable", err)
	}
	defer func() { _ = p.Close() }()

	if p.dtlsSock == nil {
		t.Fatal("dtlsSock is nil")
	}
	if p.dtlsSock.tlsConfig == nil || len(p.dtlsSock.tlsConfig.Certificates) == 0 {
		t.Fatal("dtlsSock.tlsConfig has no certificates; DTLS was not applied")
	}
}

// testMulticastLocalIPv4 returns the IPv4 address of the same interface
// firstMulticastInterface (transport.go) picks for SPDP — i.e. the address a
// peer will observe as this host's source address — falling back to
// 127.0.0.1 if none is found.
func testMulticastLocalIPv4(t *testing.T) string {
	t.Helper()
	iface, err := firstMulticastInterface()
	if err != nil || iface == nil {
		return "127.0.0.1"
	}
	addrs, err := iface.Addrs()
	if err != nil {
		return "127.0.0.1"
	}
	for _, a := range addrs {
		if ipnet, ok := a.(*net.IPNet); ok {
			if ip4 := ipnet.IP.To4(); ip4 != nil {
				return ip4.String()
			}
		}
	}
	return "127.0.0.1"
}

// TestDTLS_TwoParticipants_SameHost proves end-to-end delivery over the
// RTPS-over-DTLS transport between two real participants: SPDP discovery
// happens over ordinary UDP multicast (unrelated to this milestone; SPDP's
// mcastSock is unconditional regardless of WithNoMulticast — only the
// user-data multicast socket is gated by it), but WithNoMulticast forces
// every DATA write over unicast rather than the user-data multicast
// fast-path, so it is intercepted by sendUnicast and encrypted over DTLS
// because each participant's WithDTLSPeers names the other's DTLS address.
// Gated like the sibling same-host two-participant tests (rtps_test.go,
// access_test.go): skipped under -short (used by CI's test-rtps job) and on
// platforms where UDP multicast loopback is unreliable.
func TestDTLS_TwoParticipants_SameHost(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping two-participant test in -short mode (requires network)")
	}
	if runtime.GOOS != "linux" {
		t.Skipf("UDP multicast loopback unreliable on %s; skipping cross-participant DTLS test", runtime.GOOS)
	}

	cfg := generateTestTLSConfig(t)
	const domain = 94
	const p1Port = 17620
	const p2Port = 17621

	// SPDP's multicast socket joins the first non-loopback multicast-capable
	// interface (see firstMulticastInterface in transport.go), so peers
	// observe each other's source address on that interface — not
	// necessarily 127.0.0.1 (e.g. inside a container, the bridge interface
	// address). Bind the DTLS listeners on all interfaces (0.0.0.0) and
	// advertise that same real address via WithDTLSPeers so dtlsLocatorForIP
	// actually matches the peer address sendUnicast observes.
	localIP := testMulticastLocalIPv4(t)
	p1DTLSAddr := net.JoinHostPort(localIP, fmt.Sprintf("%d", p1Port))
	p2DTLSAddr := net.JoinHostPort(localIP, fmt.Sprintf("%d", p2Port))

	p1, err := newParticipant(dds.Domain(domain),
		WithDTLSAddr(fmt.Sprintf("0.0.0.0:%d", p1Port)), WithDTLS(cfg), WithDTLSPeers(p2DTLSAddr),
		WithNoMulticast(),
	)
	if err != nil {
		t.Skipf("newParticipant(p1): %v", err)
	}
	defer func() { _ = p1.Close() }()

	p2, err := newParticipant(dds.Domain(domain),
		WithDTLSAddr(fmt.Sprintf("0.0.0.0:%d", p2Port)), WithDTLS(cfg), WithDTLSPeers(p1DTLSAddr),
		WithNoMulticast(),
	)
	if err != nil {
		t.Skipf("newParticipant(p2): %v", err)
	}
	defer func() { _ = p2.Close() }()

	sub, err := p2.NewSubscriber("dtls/cross-participant", dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewSubscriber: %v", err)
	}
	defer sub.Close()

	pub, err := p1.NewPublisher("dtls/cross-participant", dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
	defer pub.Close()

	// Allow SPDP + SEDP to complete (within the 2 s announce period).
	time.Sleep(2200 * time.Millisecond)

	want := []byte(`{"transport":"rtps-over-dtls","ok":true}`)
	if err := pub.Write(want); err != nil {
		t.Fatalf("Write: %v", err)
	}

	select {
	case s := <-sub.C():
		if !bytes.Equal(s.Payload, want) {
			t.Errorf("payload: got %q, want %q", s.Payload, want)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timeout: sample not delivered over RTPS-over-DTLS")
	}

	// Confirm the mechanism under test actually ran: at least one
	// participant must have a live DTLS connection to the other.
	p1.dtlsSock.mu.Lock()
	p1Conns := len(p1.dtlsSock.conns)
	p1.dtlsSock.mu.Unlock()
	p2.dtlsSock.mu.Lock()
	p2Conns := len(p2.dtlsSock.conns)
	p2.dtlsSock.mu.Unlock()
	if p1Conns == 0 && p2Conns == 0 {
		t.Error("expected at least one cached DTLS connection on p1 or p2; none found")
	}
}
