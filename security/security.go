// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Package security provides pluggable transport-security for go-DDS.
//
// Security is applied at the packet level in the RTPS transport: every
// outbound payload is passed through Plugin.Seal before transmission, and
// every inbound payload through Plugin.Open before delivery to the
// application. The mock transport passes payloads through the plugin at the
// broker level so tests can use the same Plugin implementations without a
// live network.
//
// Two built-in plugins are provided:
//
//   - [NullPlugin] — identity transform; no confidentiality, no integrity.
//     Use during development and for interop with non-secured peers.
//   - [HMACPlugin] — appends an HMAC-SHA-256 tag to each payload. Provides
//     integrity and authentication without confidentiality. Fast; zero
//     payload expansion overhead beyond the 32-byte tag.
//   - [AESGCMPlugin] — encrypts with AES-256-GCM (AEAD). Provides full
//     confidentiality, integrity, and authenticity. Payload expands by 12
//     bytes (nonce) + 16 bytes (GCM tag) = 28 bytes.
package security

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"io"
)

// Plugin is implemented by any type that can seal (encrypt / sign) and open
// (decrypt / verify) a DDS payload. Seal and Open must be inverses:
//
//	plaintext == must(Open(must(Seal(plaintext))))
//
// Implementations must be safe for concurrent use from multiple goroutines.
type Plugin interface {
	// Seal transforms plaintext into a protected form ready for transmission.
	// The returned slice may share memory with plaintext or be newly allocated.
	Seal(plaintext []byte) ([]byte, error)

	// Open reverses Seal, returning the original plaintext. Returns an error
	// if the payload is invalid, tampered, or cannot be decrypted.
	Open(ciphertext []byte) ([]byte, error)
}

// ── NullPlugin ────────────────────────────────────────────────────────────────

// NullPlugin is the identity transform: Seal and Open return the input
// unchanged. Use it when no security is required (e.g. development, testing,
// or within a trusted private network).
type NullPlugin struct{}

func (NullPlugin) Seal(p []byte) ([]byte, error) { return p, nil }
func (NullPlugin) Open(p []byte) ([]byte, error) { return p, nil }

// ── HMACPlugin ────────────────────────────────────────────────────────────────

// HMACPlugin appends an HMAC-SHA-256 authentication tag to each payload.
// It provides integrity and peer authentication but NOT confidentiality —
// the payload travels in plaintext. Use when eavesdropping is not a concern
// but tampering or spoofing must be detected.
//
// Wire format: | plaintext... | HMAC[32] |
type HMACPlugin struct {
	key []byte
}

// NewHMACPlugin creates an HMACPlugin keyed with key. The key should be at
// least 32 bytes of random data; use [NewRandomKey] to generate one.
func NewHMACPlugin(key []byte) *HMACPlugin {
	k := make([]byte, len(key))
	copy(k, key)
	return &HMACPlugin{key: k}
}

const hmacSize = 32

func (p *HMACPlugin) Seal(plaintext []byte) ([]byte, error) {
	mac := hmac.New(sha256.New, p.key)
	mac.Write(plaintext)
	out := make([]byte, len(plaintext)+hmacSize)
	copy(out, plaintext)
	mac.Sum(out[len(plaintext):len(plaintext)])
	return out, nil
}

func (p *HMACPlugin) Open(data []byte) ([]byte, error) {
	if len(data) < hmacSize {
		return nil, errors.New("security: HMAC payload too short")
	}
	plaintext := data[:len(data)-hmacSize]
	tag := data[len(data)-hmacSize:]
	mac := hmac.New(sha256.New, p.key)
	mac.Write(plaintext)
	expected := mac.Sum(nil)
	if !hmac.Equal(tag, expected) {
		return nil, errors.New("security: HMAC verification failed")
	}
	return plaintext, nil
}

// ── AESGCMPlugin ──────────────────────────────────────────────────────────────

// AESGCMPlugin encrypts payloads with AES-256-GCM (authenticated encryption).
// It provides confidentiality, integrity, and authenticity. Each Seal call
// generates a fresh 12-byte random nonce prepended to the ciphertext.
//
// Wire format: | nonce[12] | ciphertext... | GCM-tag[16] |
//
// Payload overhead: 28 bytes per sample.
type AESGCMPlugin struct {
	aead cipher.AEAD
}

// NewAESGCMPlugin creates an AESGCMPlugin. key must be exactly 32 bytes
// (AES-256); use [NewRandomKey] to generate one.
func NewAESGCMPlugin(key []byte) (*AESGCMPlugin, error) {
	if len(key) != 32 {
		return nil, errors.New("security: AES-GCM key must be 32 bytes (AES-256)")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &AESGCMPlugin{aead: aead}, nil
}

func (p *AESGCMPlugin) Seal(plaintext []byte) ([]byte, error) {
	nonce := make([]byte, p.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	out := p.aead.Seal(nonce, nonce, plaintext, nil)
	return out, nil
}

func (p *AESGCMPlugin) Open(data []byte) ([]byte, error) {
	ns := p.aead.NonceSize()
	if len(data) < ns+p.aead.Overhead() {
		return nil, errors.New("security: AES-GCM payload too short")
	}
	nonce, ct := data[:ns], data[ns:]
	return p.aead.Open(nil, nonce, ct, nil)
}

// ── Key utilities ─────────────────────────────────────────────────────────────

// NewRandomKey returns n cryptographically random bytes suitable for use as a
// plugin key. Panics if the OS random source fails (this should never happen
// on any supported platform).
func NewRandomKey(n int) []byte {
	k := make([]byte, n)
	if _, err := io.ReadFull(rand.Reader, k); err != nil {
		panic("security: random key generation failed: " + err.Error())
	}
	return k
}
