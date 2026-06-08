// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package security_test

//fusa:test REQ-SEC-013

import (
	"testing"

	"github.com/SoundMatt/go-DDS/security"
)

func TestAccessPolicy_ExactMatch_Read(t *testing.T) {
	p := security.NewAccessPolicy(
		security.Rule{Pattern: "vehicle/speed", Allow: security.PermRead},
	)
	if !p.CanRead("vehicle/speed") {
		t.Error("CanRead: expected true for exact match")
	}
	if p.CanWrite("vehicle/speed") {
		t.Error("CanWrite: expected false for read-only rule")
	}
}

func TestAccessPolicy_ExactMatch_Write(t *testing.T) {
	p := security.NewAccessPolicy(
		security.Rule{Pattern: "actuator/brake", Allow: security.PermWrite},
	)
	if !p.CanWrite("actuator/brake") {
		t.Error("CanWrite: expected true for write rule")
	}
	if p.CanRead("actuator/brake") {
		t.Error("CanRead: expected false for write-only rule")
	}
}

func TestAccessPolicy_ReadWrite(t *testing.T) {
	p := security.NewAccessPolicy(
		security.Rule{Pattern: "sensor/temp", Allow: security.PermReadWrite},
	)
	if !p.CanRead("sensor/temp") {
		t.Error("CanRead: expected true for ReadWrite rule")
	}
	if !p.CanWrite("sensor/temp") {
		t.Error("CanWrite: expected true for ReadWrite rule")
	}
}

func TestAccessPolicy_GlobMatch_SingleSegment(t *testing.T) {
	p := security.NewAccessPolicy(
		security.Rule{Pattern: "vehicle/*", Allow: security.PermRead},
	)
	if !p.CanRead("vehicle/speed") {
		t.Error("CanRead vehicle/speed: expected true")
	}
	if !p.CanRead("vehicle/rpm") {
		t.Error("CanRead vehicle/rpm: expected true")
	}
	// Multi-segment child must not match (path.Match '*' stops at '/')
	if p.CanRead("vehicle/engine/rpm") {
		t.Error("CanRead vehicle/engine/rpm: expected false (multi-segment child)")
	}
}

func TestAccessPolicy_GlobMatch_AllTopLevel(t *testing.T) {
	p := security.NewAccessPolicy(
		security.Rule{Pattern: "*", Allow: security.PermRead},
	)
	if !p.CanRead("speed") {
		t.Error("CanRead speed: expected true")
	}
	// '*' does not match a topic with a '/' separator.
	if p.CanRead("vehicle/speed") {
		t.Error("CanRead vehicle/speed: expected false for top-level '*'")
	}
}

func TestAccessPolicy_NoMatch_DenyAll(t *testing.T) {
	p := security.NewAccessPolicy(
		security.Rule{Pattern: "allowed/topic", Allow: security.PermReadWrite},
	)
	if p.CanRead("other/topic") {
		t.Error("CanRead other/topic: expected false (no match)")
	}
	if p.CanWrite("other/topic") {
		t.Error("CanWrite other/topic: expected false (no match)")
	}
}

func TestAccessPolicy_EmptyPolicy_DenyAll(t *testing.T) {
	p := security.NewAccessPolicy()
	if p.CanRead("any/topic") {
		t.Error("empty policy should deny all reads")
	}
	if p.CanWrite("any/topic") {
		t.Error("empty policy should deny all writes")
	}
}

func TestAccessPolicy_FirstMatchWins(t *testing.T) {
	// First rule grants Read; second rule (never reached) grants Write.
	p := security.NewAccessPolicy(
		security.Rule{Pattern: "topic", Allow: security.PermRead},
		security.Rule{Pattern: "topic", Allow: security.PermWrite},
	)
	if !p.CanRead("topic") {
		t.Error("CanRead: expected true from first rule")
	}
	// Second rule is shadowed — Write should be denied.
	if p.CanWrite("topic") {
		t.Error("CanWrite: expected false; first rule takes priority")
	}
}

func TestAccessPolicy_MalformedPattern_Skipped(t *testing.T) {
	// path.Match returns an error for certain malformed patterns (e.g. '[').
	// The policy should skip such rules rather than panic.
	p := security.NewAccessPolicy(
		security.Rule{Pattern: "[bad", Allow: security.PermReadWrite},
		security.Rule{Pattern: "good", Allow: security.PermRead},
	)
	if !p.CanRead("good") {
		t.Error("CanRead good: expected true; malformed rule should be skipped")
	}
}
