// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package security_test

import (
	"testing"

	"github.com/SoundMatt/go-DDS/security"
)

func TestHMACDiscoveryPlugin_SignVerify(t *testing.T) {
	key := []byte("test-secret-key")
	plugin := security.NewHMACDiscoveryPlugin(key)

	prefix := []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12}
	tag := plugin.SignDiscovery(prefix)
	if len(tag) == 0 {
		t.Fatal("SignDiscovery returned empty tag")
	}

	if !plugin.VerifyDiscovery(prefix, tag) {
		t.Error("VerifyDiscovery returned false for valid tag")
	}
}

func TestHMACDiscoveryPlugin_WrongKey(t *testing.T) {
	plug1 := security.NewHMACDiscoveryPlugin([]byte("key-a"))
	plug2 := security.NewHMACDiscoveryPlugin([]byte("key-b"))

	prefix := []byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11}
	tag := plug1.SignDiscovery(prefix)

	if plug2.VerifyDiscovery(prefix, tag) {
		t.Error("VerifyDiscovery should reject tag from a different key")
	}
}

func TestHMACDiscoveryPlugin_WrongPrefix(t *testing.T) {
	plugin := security.NewHMACDiscoveryPlugin([]byte("shared-key"))

	prefix1 := []byte{1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1}
	prefix2 := []byte{2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2}
	tag := plugin.SignDiscovery(prefix1)

	if plugin.VerifyDiscovery(prefix2, tag) {
		t.Error("VerifyDiscovery should reject tag for different prefix")
	}
}

func TestHMACDiscoveryPlugin_NilTag(t *testing.T) {
	plugin := security.NewHMACDiscoveryPlugin([]byte("key"))
	prefix := []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}
	if plugin.VerifyDiscovery(prefix, nil) {
		t.Error("VerifyDiscovery should return false for nil tag")
	}
}

func TestHMACDiscoveryPlugin_EmptyTag(t *testing.T) {
	plugin := security.NewHMACDiscoveryPlugin([]byte("key"))
	prefix := []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}
	if plugin.VerifyDiscovery(prefix, []byte{}) {
		t.Error("VerifyDiscovery should return false for empty tag")
	}
}

func TestHMACDiscoveryPlugin_KeyIsNotShared(t *testing.T) {
	key := []byte("mutable-key")
	plugin := security.NewHMACDiscoveryPlugin(key)
	prefix := []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12}
	tag1 := plugin.SignDiscovery(prefix)

	// Mutate original key slice; plugin should be unaffected.
	key[0] = 0xFF
	tag2 := plugin.SignDiscovery(prefix)

	if string(tag1) != string(tag2) {
		t.Error("plugin key was mutated externally; NewHMACDiscoveryPlugin must copy the key")
	}
}

func TestHMACDiscoveryPlugin_DifferentPrefixesDifferentTags(t *testing.T) {
	plugin := security.NewHMACDiscoveryPlugin([]byte("key"))
	p1 := []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1}
	p2 := []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 2}
	if string(plugin.SignDiscovery(p1)) == string(plugin.SignDiscovery(p2)) {
		t.Error("different prefixes must produce different tags")
	}
}

func TestHMACDiscoveryPlugin_ImplementsDiscoveryPlugin(t *testing.T) {
	var _ security.DiscoveryPlugin = security.NewHMACDiscoveryPlugin([]byte("k"))
}
