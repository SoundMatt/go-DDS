// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package security_test

//fusa:test REQ-SEC-001
//fusa:test REQ-SEC-004
//fusa:test REQ-SEC-005
//fusa:test REQ-SEC-006
//fusa:test REQ-SEC-007
//fusa:test REQ-SEC-015
//fusa:test REQ-SEC-016
//fusa:test REQ-SEC-017
//fusa:test REQ-SEC-018
//fusa:test REQ-SEOOC-003
//fusa:test REQ-SEOOC-006

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

// ── EndpointPlugin (SEDP signing) ─────────────────────────────────────────────

func TestHMACDiscoveryPlugin_SignEndpoint_Valid(t *testing.T) {
	plugin := security.NewHMACDiscoveryPlugin([]byte("endpoint-key"))
	prefix := []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12}
	topic := "sensors/temperature"

	tag := plugin.SignEndpoint(prefix, topic)
	if len(tag) == 0 {
		t.Fatal("SignEndpoint returned empty tag")
	}
	if !plugin.VerifyEndpoint(prefix, topic, tag) {
		t.Error("VerifyEndpoint returned false for valid tag")
	}
}

func TestHMACDiscoveryPlugin_VerifyEndpoint_WrongKey(t *testing.T) {
	plug1 := security.NewHMACDiscoveryPlugin([]byte("key-a"))
	plug2 := security.NewHMACDiscoveryPlugin([]byte("key-b"))
	prefix := []byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11}
	tag := plug1.SignEndpoint(prefix, "my/topic")
	if plug2.VerifyEndpoint(prefix, "my/topic", tag) {
		t.Error("VerifyEndpoint should reject tag from a different key")
	}
}

func TestHMACDiscoveryPlugin_VerifyEndpoint_WrongTopic(t *testing.T) {
	plugin := security.NewHMACDiscoveryPlugin([]byte("shared"))
	prefix := []byte{1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1}
	tag := plugin.SignEndpoint(prefix, "topic/a")
	if plugin.VerifyEndpoint(prefix, "topic/b", tag) {
		t.Error("VerifyEndpoint should reject tag for different topic")
	}
}

func TestHMACDiscoveryPlugin_VerifyEndpoint_WrongPrefix(t *testing.T) {
	plugin := security.NewHMACDiscoveryPlugin([]byte("shared"))
	prefix1 := []byte{1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1}
	prefix2 := []byte{2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2}
	tag := plugin.SignEndpoint(prefix1, "my/topic")
	if plugin.VerifyEndpoint(prefix2, "my/topic", tag) {
		t.Error("VerifyEndpoint should reject tag for different prefix")
	}
}

func TestHMACDiscoveryPlugin_VerifyEndpoint_EmptyTag(t *testing.T) {
	plugin := security.NewHMACDiscoveryPlugin([]byte("key"))
	prefix := []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}
	if plugin.VerifyEndpoint(prefix, "t", []byte{}) {
		t.Error("VerifyEndpoint should return false for empty tag")
	}
}

func TestHMACDiscoveryPlugin_VerifyEndpoint_NilTag(t *testing.T) {
	plugin := security.NewHMACDiscoveryPlugin([]byte("key"))
	prefix := []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}
	if plugin.VerifyEndpoint(prefix, "t", nil) {
		t.Error("VerifyEndpoint should return false for nil tag")
	}
}

func TestHMACDiscoveryPlugin_EndpointAndDiscovery_DifferentTags(t *testing.T) {
	plugin := security.NewHMACDiscoveryPlugin([]byte("key"))
	prefix := []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12}
	dTag := plugin.SignDiscovery(prefix)
	eTag := plugin.SignEndpoint(prefix, "sensors/temp")
	if string(dTag) == string(eTag) {
		t.Error("SPDP and SEDP tags must differ (distinct HMAC contexts)")
	}
}

func TestHMACDiscoveryPlugin_DifferentTopics_DifferentTags(t *testing.T) {
	plugin := security.NewHMACDiscoveryPlugin([]byte("key"))
	prefix := []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12}
	t1 := plugin.SignEndpoint(prefix, "topic/a")
	t2 := plugin.SignEndpoint(prefix, "topic/b")
	if string(t1) == string(t2) {
		t.Error("different topics must produce different endpoint tags")
	}
}

// ── Rekey ─────────────────────────────────────────────────────────────────────

func TestHMACDiscoveryPlugin_Rekey_ChangesOutput(t *testing.T) {
	plugin := security.NewHMACDiscoveryPlugin([]byte("old-key"))
	prefix := []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12}

	tagBefore := plugin.SignDiscovery(prefix)
	plugin.Rekey([]byte("new-key"))
	tagAfter := plugin.SignDiscovery(prefix)

	if string(tagBefore) == string(tagAfter) {
		t.Error("Rekey must produce a different tag")
	}
}

func TestHMACDiscoveryPlugin_Rekey_VerifyWithNewKey(t *testing.T) {
	plugin := security.NewHMACDiscoveryPlugin([]byte("old-key"))
	prefix := []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12}

	plugin.Rekey([]byte("new-key"))
	tag := plugin.SignDiscovery(prefix)
	if !plugin.VerifyDiscovery(prefix, tag) {
		t.Error("should verify own tag after Rekey")
	}
}

func TestHMACDiscoveryPlugin_Rekey_OldTagInvalid(t *testing.T) {
	plugin := security.NewHMACDiscoveryPlugin([]byte("old-key"))
	prefix := []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12}

	oldTag := plugin.SignDiscovery(prefix)
	plugin.Rekey([]byte("new-key"))

	if plugin.VerifyDiscovery(prefix, oldTag) {
		t.Error("tag signed with old key must be invalid after Rekey")
	}
}

func TestHMACDiscoveryPlugin_Rekey_Endpoint(t *testing.T) {
	plugin := security.NewHMACDiscoveryPlugin([]byte("old-key"))
	prefix := []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12}
	topic := "sensors/temperature"

	oldTag := plugin.SignEndpoint(prefix, topic)
	plugin.Rekey([]byte("new-key"))
	newTag := plugin.SignEndpoint(prefix, topic)

	if string(oldTag) == string(newTag) {
		t.Error("endpoint tag must change after Rekey")
	}
	if plugin.VerifyEndpoint(prefix, topic, oldTag) {
		t.Error("old endpoint tag must be invalid after Rekey")
	}
	if !plugin.VerifyEndpoint(prefix, topic, newTag) {
		t.Error("new endpoint tag must be valid after Rekey")
	}
}

func TestHMACDiscoveryPlugin_Rekey_CopiesKey(t *testing.T) {
	newKey := []byte("mutable-new-key")
	plugin := security.NewHMACDiscoveryPlugin([]byte("old-key"))
	plugin.Rekey(newKey)

	prefix := []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12}
	tagBefore := plugin.SignDiscovery(prefix)

	// Mutate the key slice after Rekey.
	newKey[0] = 0xFF
	tagAfter := plugin.SignDiscovery(prefix)

	if string(tagBefore) != string(tagAfter) {
		t.Error("Rekey must copy the key; external mutation must not affect plugin")
	}
}
