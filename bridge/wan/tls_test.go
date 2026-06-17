// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Tests for WAN bridge TLS encryption and shared-token authentication.

package wan_test

//fusa:test REQ-BRIDGE-013
//fusa:test REQ-BRIDGE-014

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"testing"
	"time"

	dds "github.com/SoundMatt/go-DDS"
	"github.com/SoundMatt/go-DDS/bridge/wan"
)

// newTLSPair builds a server tls.Config (with a self-signed cert valid for
// 127.0.0.1) and a client tls.Config that trusts it — no InsecureSkipVerify.
func newTLSPair(t *testing.T) (server, client *tls.Config) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "wan-test"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1")},
		IsCA:                  true,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("CreateCertificate: %v", err)
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatalf("MarshalECPrivateKey: %v", err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})

	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		t.Fatalf("X509KeyPair: %v", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(certPEM) {
		t.Fatal("AppendCertsFromPEM failed")
	}
	return &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12},
		&tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}
}

func TestWANBridge_TLS_RoundTrip(t *testing.T) {
	serverTLS, clientTLS := newTLSPair(t)
	src := newPart(t)
	dst := newPart(t)
	topic := uniqueTopic("wan/tls")

	sub, err := dst.NewSubscriber(topic, dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewSubscriber: %v", err)
	}
	defer sub.Close()

	srv, err := wan.Serve(dst, "127.0.0.1:0", wan.Options{TLS: serverTLS})
	if err != nil {
		t.Fatalf("Serve: %v", err)
	}
	defer srv.Close()

	cli, err := wan.Connect(src, srv.Addr(), wan.Options{Topics: []string{topic}, TLS: clientTLS})
	if err != nil {
		t.Fatalf("Connect (TLS): %v", err)
	}
	defer cli.Close()

	pub, err := src.NewPublisher(topic, dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
	defer pub.Close()
	if err := pub.Write([]byte("encrypted")); err != nil {
		t.Fatalf("Write: %v", err)
	}

	select {
	case s := <-sub.C():
		if string(s.Payload) != "encrypted" {
			t.Errorf("payload: got %q, want encrypted", s.Payload)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout: sample not forwarded over TLS")
	}
}

func TestWANBridge_TokenAuth_Accepts(t *testing.T) {
	src := newPart(t)
	dst := newPart(t)
	topic := uniqueTopic("wan/auth/ok")

	sub, err := dst.NewSubscriber(topic, dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewSubscriber: %v", err)
	}
	defer sub.Close()

	srv, err := wan.Serve(dst, "127.0.0.1:0", wan.Options{Token: "s3cret"})
	if err != nil {
		t.Fatalf("Serve: %v", err)
	}
	defer srv.Close()

	cli, err := wan.Connect(src, srv.Addr(), wan.Options{Topics: []string{topic}, Token: "s3cret"})
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer cli.Close()

	pub, err := src.NewPublisher(topic, dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
	defer pub.Close()
	if err := pub.Write([]byte("authed")); err != nil {
		t.Fatalf("Write: %v", err)
	}

	select {
	case s := <-sub.C():
		if string(s.Payload) != "authed" {
			t.Errorf("payload: got %q, want authed", s.Payload)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout: authenticated sample not forwarded")
	}
}

func TestWANBridge_TokenAuth_Rejects(t *testing.T) {
	src := newPart(t)
	dst := newPart(t)
	topic := uniqueTopic("wan/auth/bad")

	sub, err := dst.NewSubscriber(topic, dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewSubscriber: %v", err)
	}
	defer sub.Close()

	srv, err := wan.Serve(dst, "127.0.0.1:0", wan.Options{Token: "correct-token"})
	if err != nil {
		t.Fatalf("Serve: %v", err)
	}
	defer srv.Close()

	// Client presents the wrong token — the server must drop the connection.
	cli, err := wan.Connect(src, srv.Addr(), wan.Options{Topics: []string{topic}, Token: "wrong-token"})
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer cli.Close()

	pub, err := src.NewPublisher(topic, dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
	defer pub.Close()
	if err := pub.Write([]byte("should-not-arrive")); err != nil {
		t.Fatalf("Write: %v", err)
	}

	select {
	case s := <-sub.C():
		t.Errorf("unauthorized client's sample was forwarded: %q", s.Payload)
	case <-time.After(800 * time.Millisecond):
		// Expected: nothing delivered because the server rejected the token.
	}
}
