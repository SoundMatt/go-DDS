// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package security_test

//fusa:test REQ-SEC-014
//fusa:test REQ-SEC-024
//fusa:test REQ-SEC-025

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/binary"
	"encoding/pem"
	"math/big"
	"testing"
	"time"

	"github.com/SoundMatt/go-DDS/security"
)

// genCA creates a self-signed CA certificate and returns (caPEM, caKey).
func genCA(t *testing.T) (caPEM []byte, caKey *ecdsa.PrivateKey, caCert *x509.Certificate) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("genCA: generate key: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "test-ca"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, key.Public(), key)
	if err != nil {
		t.Fatalf("genCA: create cert: %v", err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("genCA: parse cert: %v", err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), key, cert
}

// genLeaf creates a leaf certificate signed by the given CA.
func genLeaf(t *testing.T, caKey *ecdsa.PrivateKey, caCert *x509.Certificate) (certPEM, keyPEM []byte) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("genLeaf: generate key: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: "test-leaf"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, caCert, key.Public(), caKey)
	if err != nil {
		t.Fatalf("genLeaf: create cert: %v", err)
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatalf("genLeaf: marshal key: %v", err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
		pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
}

func newCertPlugin(t *testing.T) *security.CertPlugin {
	t.Helper()
	caPEM, caKey, caCert := genCA(t)
	certPEM, keyPEM := genLeaf(t, caKey, caCert)
	p, err := security.NewCertPlugin(certPEM, keyPEM, caPEM)
	if err != nil {
		t.Fatalf("NewCertPlugin: %v", err)
	}
	return p
}

func TestCertPlugin_RoundTrip(t *testing.T) {
	p := newCertPlugin(t)
	cases := [][]byte{
		{},
		[]byte("hello world"),
		[]byte(`{"speed":120,"unit":"kmh"}`),
		make([]byte, 512),
	}
	for _, c := range cases {
		roundTrip(t, p, "CertPlugin", c)
	}
}

func TestCertPlugin_TamperedPayload(t *testing.T) {
	p := newCertPlugin(t)
	sealed, _ := p.Seal([]byte("sensitive data"))
	sealed[0] ^= 0xFF
	if _, err := p.Open(sealed); err == nil {
		t.Error("expected error on tampered plaintext")
	}
}

func TestCertPlugin_TamperedSignature(t *testing.T) {
	p := newCertPlugin(t)
	sealed, _ := p.Seal([]byte("data"))
	// Corrupt the last byte (part of the signature).
	sealed[len(sealed)-1] ^= 0xFF
	if _, err := p.Open(sealed); err == nil {
		t.Error("expected error on corrupted signature")
	}
}

func TestCertPlugin_UntrustedCert(t *testing.T) {
	// Build two independent CA/leaf pairs; try to open a message from leafA
	// using a plugin that only trusts caB.
	caAPEM, caAKey, caACert := genCA(t)
	certAPEM, keyAPEM := genLeaf(t, caAKey, caACert)

	caBPEM, caBKey, caBCert := genCA(t)
	certBPEM, keyBPEM := genLeaf(t, caBKey, caBCert)

	pluginA, err := security.NewCertPlugin(certAPEM, keyAPEM, caAPEM)
	if err != nil {
		t.Fatalf("pluginA: %v", err)
	}
	pluginB, err := security.NewCertPlugin(certBPEM, keyBPEM, caBPEM)
	if err != nil {
		t.Fatalf("pluginB: %v", err)
	}

	sealed, _ := pluginA.Seal([]byte("from A"))
	if _, err := pluginB.Open(sealed); err == nil {
		t.Error("expected error: cert from CA-A should not be trusted by CA-B")
	}
}

func TestCertPlugin_Open_TooShort(t *testing.T) {
	p := newCertPlugin(t)
	if _, err := p.Open([]byte("short")); err == nil {
		t.Error("expected error on payload shorter than minimum")
	}
}

func TestCertPlugin_New_BadCertPEM(t *testing.T) {
	_, err := security.NewCertPlugin([]byte("not-pem"), []byte("x"), []byte("y"))
	if err == nil {
		t.Error("expected error for invalid certPEM")
	}
}

func TestCertPlugin_New_BadKeyPEM(t *testing.T) {
	caPEM, caKey, caCert := genCA(t)
	certPEM, _ := genLeaf(t, caKey, caCert)
	_, err := security.NewCertPlugin(certPEM, []byte("not-pem"), caPEM)
	if err == nil {
		t.Error("expected error for invalid keyPEM")
	}
}

func TestCertPlugin_New_MismatchedKey(t *testing.T) {
	caPEM, caKey, caCert := genCA(t)
	certPEM, _ := genLeaf(t, caKey, caCert)
	// Generate a different leaf key (does not match certPEM's public key).
	_, keyPEM2 := genLeaf(t, caKey, caCert)
	_, err := security.NewCertPlugin(certPEM, keyPEM2, caPEM)
	if err == nil {
		t.Error("expected error for mismatched cert/key pair")
	}
}

func TestCertPlugin_New_BadCAPEM(t *testing.T) {
	caPEM, caKey, caCert := genCA(t)
	certPEM, keyPEM := genLeaf(t, caKey, caCert)
	_ = caPEM
	_, err := security.NewCertPlugin(certPEM, keyPEM, []byte("not-a-ca"))
	if err == nil {
		t.Error("expected error for invalid CA PEM")
	}
}

func TestCertPlugin_SealProducesDistinctOutputs(t *testing.T) {
	// ECDSA signing uses a fresh random nonce, so two Seal calls on identical
	// plaintext must produce different sealed bytes.
	p := newCertPlugin(t)
	a, _ := p.Seal([]byte("same"))
	b, _ := p.Seal([]byte("same"))
	if bytes.Equal(a, b) {
		t.Error("two Seal calls on identical plaintext produced identical output (nonce reuse?)")
	}
}

// TestCertPlugin_New_NonECDSACert exercises the branch in NewCertPlugin that
// rejects a certificate whose public key is not *ecdsa.PublicKey.
func TestCertPlugin_New_NonECDSACert(t *testing.T) {
	caPEM, _, _ := genCA(t)

	// Create a self-signed RSA certificate.
	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("rsa.GenerateKey: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(99),
		Subject:      pkix.Name{CommonName: "rsa-leaf"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, rsaKey.Public(), rsaKey)
	if err != nil {
		t.Fatalf("CreateCertificate: %v", err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})

	// Provide a valid EC key so parsing succeeds up to the public-key type check.
	ecKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("ecdsa.GenerateKey: %v", err)
	}
	keyDER, err := x509.MarshalECPrivateKey(ecKey)
	if err != nil {
		t.Fatalf("MarshalECPrivateKey: %v", err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})

	_, err = security.NewCertPlugin(certPEM, keyPEM, caPEM)
	if err == nil {
		t.Error("expected error for RSA certificate (non-ECDSA public key)")
	}
}

// TestCertPlugin_Open_CraftedLargeCertLen exercises the certStart < 0 branch
// in Open by supplying a payload whose certLen field claims more bytes than
// are available before the certLen field itself.
func TestCertPlugin_Open_CraftedLargeCertLen(t *testing.T) {
	p := newCertPlugin(t)

	// Layout: | 2-byte dummy | certLen(4) = 0x00FFFFFF | sigLen(4) = 0 |
	// sigLen = 0  → sigStart = 6 ≥ 0 (passes prior check)
	// certLen = 16_777_215 → certStart = 2 - 16_777_215 < 0 (triggers error)
	data := []byte{
		0x00, 0x00, // dummy "plaintext"
		0x00, 0xFF, 0xFF, 0xFF, // certLen = 16,777,215 (too large)
		0x00, 0x00, 0x00, 0x00, // sigLen = 0
	}
	_, err := p.Open(data)
	if err == nil {
		t.Error("expected error for crafted payload with huge certLen")
	}
}

// TestCertPlugin_Open_NonECDSASignerCert exercises the !ok branch where the
// signer's certificate has an RSA public key rather than ECDSA.
func TestCertPlugin_Open_NonECDSASignerCert(t *testing.T) {
	// Build a plugin that trusts CA A.
	caPEM, caKey, caCert := genCA(t)
	certPEM, keyPEM := genLeaf(t, caKey, caCert)
	p, err := security.NewCertPlugin(certPEM, keyPEM, caPEM)
	if err != nil {
		t.Fatalf("NewCertPlugin: %v", err)
	}

	// Create an RSA leaf cert signed by the same CA (so Verify passes).
	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("rsa.GenerateKey: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(99),
		Subject:      pkix.Name{CommonName: "rsa-attacker"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}
	rsaDER, err := x509.CreateCertificate(rand.Reader, tmpl, caCert, rsaKey.Public(), caKey)
	if err != nil {
		t.Fatalf("CreateCertificate RSA leaf: %v", err)
	}

	// Manually construct a wire-format payload embedding the RSA cert DER.
	// The sig bytes are arbitrary — we fail before signature verification.
	plaintext := []byte("test")
	sig := []byte("not-a-real-sig")
	certLen := len(rsaDER)
	sigLen := len(sig)
	totalLen := len(plaintext) + certLen + 4 + sigLen + 4
	payload := make([]byte, totalLen)
	n := copy(payload, plaintext)
	n += copy(payload[n:], rsaDER)
	binary.BigEndian.PutUint32(payload[n:], uint32(certLen))
	n += 4
	n += copy(payload[n:], sig)
	binary.BigEndian.PutUint32(payload[n:], uint32(sigLen))

	_, err = p.Open(payload)
	if err == nil {
		t.Error("expected error for non-ECDSA signer certificate in Open")
	}
}

// TestCertPlugin_ParseECKey_InvalidDER exercises the parseECKey error branch
// where the PEM block exists but contains invalid EC key DER.
func TestCertPlugin_ParseECKey_InvalidDER(t *testing.T) {
	caPEM, caKey, caCert := genCA(t)
	certPEM, _ := genLeaf(t, caKey, caCert)

	// Valid PEM type but garbage DER content.
	badKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "EC PRIVATE KEY",
		Bytes: []byte("not valid DER content"),
	})

	_, err := security.NewCertPlugin(certPEM, badKeyPEM, caPEM)
	if err == nil {
		t.Error("expected error for valid PEM with invalid EC key DER")
	}
}
