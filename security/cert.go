// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package security

//fusa:req REQ-SEC-014
//fusa:req REQ-SEC-024
//fusa:req REQ-SEC-025

import (
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/binary"
	"encoding/pem"
	"errors"
	"fmt"
)

// CertPlugin provides asymmetric signing and verification using X.509
// certificates and ECDSA private keys.
//
// Seal appends the signer's DER-encoded certificate and an ECDSA signature
// over SHA-256(plaintext) to the payload, allowing any peer holding a
// certificate issued by the trusted CA to verify the message.
//
// Wire format (length fields trail their data so parsing from the end is O(1)):
//
//	| plaintext | certDER | certLen uint32 BE | sigDER | sigLen uint32 BE |
//
// This provides integrity and peer authentication but NOT confidentiality.
// Combine with [AESGCMPlugin] when confidentiality is required.
type CertPlugin struct {
	cert   *x509.Certificate
	key    *ecdsa.PrivateKey
	caPool *x509.CertPool
}

// NewCertPlugin creates a CertPlugin from PEM-encoded certificate, private
// key, and CA certificate(s). The private key must be ECDSA.
//
// certPEM  — PEM block containing the signer's leaf certificate (CERTIFICATE)
// keyPEM   — PEM block containing the signer's ECDSA private key (EC PRIVATE KEY)
// caPEM    — PEM block(s) containing the trusted CA certificate(s)
func NewCertPlugin(certPEM, keyPEM, caPEM []byte) (*CertPlugin, error) {
	cert, err := parseCert(certPEM)
	if err != nil {
		return nil, err
	}
	key, err := parseECKey(keyPEM)
	if err != nil {
		return nil, err
	}
	// Verify that the key matches the certificate's public key.
	certPub, ok := cert.PublicKey.(*ecdsa.PublicKey)
	if !ok {
		return nil, errors.New("security: certificate public key is not ECDSA")
	}
	if !certPub.Equal(key.Public()) {
		return nil, errors.New("security: certificate and private key do not match")
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caPEM) {
		return nil, errors.New("security: no valid CA certificates found in caPEM")
	}
	return &CertPlugin{cert: cert, key: key, caPool: pool}, nil
}

// Seal signs plaintext with the plugin's ECDSA private key and appends the
// signer's certificate and the signature to the returned payload.
func (p *CertPlugin) Seal(plaintext []byte) ([]byte, error) {
	digest := sha256.Sum256(plaintext)
	sig, err := ecdsa.SignASN1(rand.Reader, p.key, digest[:])
	if err != nil {
		return nil, err
	}
	certDER := p.cert.Raw

	// | plaintext | certDER | certLen(4) | sigDER | sigLen(4) |
	// Length fields trail their data so Open can parse from the end in O(1).
	totalLen := len(plaintext) + len(certDER) + 4 + len(sig) + 4
	out := make([]byte, totalLen)
	n := copy(out, plaintext)
	n += copy(out[n:], certDER)
	binary.BigEndian.PutUint32(out[n:], uint32(len(certDER)))
	n += 4
	n += copy(out[n:], sig)
	binary.BigEndian.PutUint32(out[n:], uint32(len(sig)))
	return out, nil
}

// Open verifies the signature appended by Seal and returns the original
// plaintext. Returns an error if the certificate is not trusted by the CA
// pool, if the signature is invalid, or if the payload is malformed.
func (p *CertPlugin) Open(data []byte) ([]byte, error) {
	// Parse from the end.
	// Layout: | plaintext | certDER | certLen(4) | sigDER | sigLen(4) |
	if len(data) < 8 {
		return nil, errors.New("security: cert payload too short")
	}

	// Last 4 bytes → sigLen.
	sigLen := int(binary.BigEndian.Uint32(data[len(data)-4:]))
	if sigLen < 0 || len(data) < 4+sigLen+4 {
		return nil, errors.New("security: cert payload too short for sig+certLen")
	}
	sigEnd := len(data) - 4
	sigStart := sigEnd - sigLen
	sig := data[sigStart:sigEnd]

	// 4 bytes before sig → certLen.
	certLenEnd := sigStart
	certLenStart := certLenEnd - 4
	certLen := int(binary.BigEndian.Uint32(data[certLenStart:certLenEnd]))
	certEnd := certLenStart
	certStart := certEnd - certLen
	if certStart < 0 {
		return nil, errors.New("security: cert payload too short for cert")
	}
	certDER := data[certStart:certEnd]
	plaintext := data[:certStart]

	// Parse and validate the signer's certificate against the CA pool.
	signerCert, err := x509.ParseCertificate(certDER)
	if err != nil {
		return nil, errors.New("security: cannot parse signer certificate: " + err.Error())
	}
	opts := x509.VerifyOptions{Roots: p.caPool}
	if _, err := signerCert.Verify(opts); err != nil {
		return nil, errors.New("security: signer certificate not trusted: " + err.Error())
	}

	// Verify the signature.
	pub, ok := signerCert.PublicKey.(*ecdsa.PublicKey)
	if !ok {
		return nil, errors.New("security: signer certificate has non-ECDSA public key")
	}
	digest := sha256.Sum256(plaintext)
	if !ecdsa.VerifyASN1(pub, digest[:], sig) {
		return nil, errors.New("security: ECDSA signature verification failed")
	}
	return plaintext, nil
}

// ── helpers ───────────────────────────────────────────────────────────────────

func parseCert(pemBytes []byte) (*x509.Certificate, error) {
	block, remaining := pem.Decode(pemBytes)
	_ = remaining
	if block == nil {
		return nil, errors.New("security: no PEM block found in certPEM")
	}
	return x509.ParseCertificate(block.Bytes)
}

func parseECKey(pemBytes []byte) (*ecdsa.PrivateKey, error) {
	block, remaining := pem.Decode(pemBytes)
	_ = remaining
	if block == nil {
		return nil, errors.New("security: no PEM block found in keyPEM")
	}
	key, err := x509.ParseECPrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("security: cannot parse EC private key: %w", err)
	}
	return key, nil
}
