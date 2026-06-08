// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package security

//fusa:req REQ-SEC-001
//fusa:req REQ-SEC-004
//fusa:req REQ-SEC-005
//fusa:req REQ-SEC-006
//fusa:req REQ-SEC-007
//fusa:req REQ-SEC-015
//fusa:req REQ-SEC-016
//fusa:req REQ-SEC-017
//fusa:req REQ-SEC-018
//fusa:req REQ-SEOOC-003

import (
	"crypto/hmac"
	"crypto/sha256"
	"sync"
)

// DiscoveryPlugin signs and verifies SPDP participant-discovery announcements.
// Use with rtps.WithDiscoverySecurity to authenticate the discovery layer.
//
// Only go-DDS participants configured with compatible plugins accept each
// other's discovery announcements; unauthenticated or wrongly-signed peers
// are silently discarded at the SPDP layer.
type DiscoveryPlugin interface {
	// SignDiscovery returns an authentication tag for guidPrefix (12 bytes).
	// The tag is embedded in outbound SPDP announcements as PID 0x8001.
	SignDiscovery(guidPrefix []byte) []byte
	// VerifyDiscovery returns true when tag is a valid authentication token
	// for the given guidPrefix. A nil or empty tag must return false.
	VerifyDiscovery(guidPrefix, tag []byte) bool
}

const discoveryContext = "go-dds-discovery-v1"

// HMACDiscoveryPlugin authenticates SPDP announcements using HMAC-SHA-256.
// All participants in a discovery group must share the same key.
//
// Usage:
//
//	plugin := security.NewHMACDiscoveryPlugin([]byte("shared-secret"))
//	p, err := rtps.New(0, rtps.WithDiscoverySecurity(plugin))
type HMACDiscoveryPlugin struct {
	mu  sync.RWMutex
	key []byte
}

// NewHMACDiscoveryPlugin returns an HMACDiscoveryPlugin keyed with key.
// The key is copied; the caller may discard or reuse the slice.
func NewHMACDiscoveryPlugin(key []byte) *HMACDiscoveryPlugin {
	k := make([]byte, len(key))
	copy(k, key)
	return &HMACDiscoveryPlugin{key: k}
}

// Rekey atomically replaces the HMAC key. Ongoing sign/verify operations
// complete before the key is swapped; subsequent operations use the new key.
// The new key is copied; the caller may discard or reuse the slice.
func (h *HMACDiscoveryPlugin) Rekey(newKey []byte) {
	k := make([]byte, len(newKey))
	copy(k, newKey)
	h.mu.Lock()
	h.key = k
	h.mu.Unlock()
}

func (h *HMACDiscoveryPlugin) sign(guidPrefix []byte) []byte {
	h.mu.RLock()
	key := h.key
	h.mu.RUnlock()
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(discoveryContext))
	_, _ = mac.Write(guidPrefix)
	return mac.Sum(nil)
}

// SignDiscovery returns an HMAC-SHA-256 tag for guidPrefix.
func (h *HMACDiscoveryPlugin) SignDiscovery(guidPrefix []byte) []byte {
	return h.sign(guidPrefix)
}

// VerifyDiscovery returns true when tag matches the expected HMAC for guidPrefix.
func (h *HMACDiscoveryPlugin) VerifyDiscovery(guidPrefix, tag []byte) bool {
	if len(tag) == 0 {
		return false
	}
	return hmac.Equal(h.sign(guidPrefix), tag)
}

const endpointContext = "go-dds-endpoint-v1"

// SignEndpoint returns an HMAC-SHA-256 tag for the endpoint identified by
// guidPrefix and topic. Implements rtps.EndpointPlugin.
func (h *HMACDiscoveryPlugin) SignEndpoint(guidPrefix []byte, topic string) []byte {
	h.mu.RLock()
	key := h.key
	h.mu.RUnlock()
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(endpointContext))
	_, _ = mac.Write(guidPrefix)
	_, _ = mac.Write([]byte(topic))
	return mac.Sum(nil)
}

// VerifyEndpoint returns true when tag matches the expected HMAC for the
// endpoint identified by guidPrefix and topic. Implements rtps.EndpointPlugin.
func (h *HMACDiscoveryPlugin) VerifyEndpoint(guidPrefix []byte, topic string, tag []byte) bool {
	if len(tag) == 0 {
		return false
	}
	return hmac.Equal(h.SignEndpoint(guidPrefix, topic), tag)
}
