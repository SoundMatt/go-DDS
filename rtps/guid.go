// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package rtps

//fusa:req REQ-GUID-001
//fusa:req REQ-GUID-002

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
)

// GuidPrefix is the 12-byte participant identifier (RTPS 2.3 §9.3.1).
type GuidPrefix [12]byte

// EntityId identifies a specific endpoint within a participant (§9.3.1).
type EntityId [4]byte

// GUID globally identifies a DDS entity: participant + endpoint.
type GUID struct {
	Prefix GuidPrefix
	Entity EntityId
}

// Well-known entity IDs per RTPS 2.3 Table 9.1.
var (
	EntityIdParticipant        = EntityId{0x00, 0x00, 0x01, 0xC1}
	EntityIdSPDPWriter         = EntityId{0x00, 0x01, 0x00, 0xC2}
	EntityIdSPDPReader         = EntityId{0x00, 0x01, 0x00, 0xC7}
	EntityIdSEDPPubWriter      = EntityId{0x00, 0x00, 0x03, 0xC2}
	EntityIdSEDPPubReader      = EntityId{0x00, 0x00, 0x03, 0xC7}
	EntityIdSEDPSubWriter      = EntityId{0x00, 0x00, 0x04, 0xC2}
	EntityIdSEDPSubReader      = EntityId{0x00, 0x00, 0x04, 0xC7}
	EntityIdUnknown            = EntityId{0x00, 0x00, 0x00, 0x00}
	EntityIdBuiltinParticipant = EntityId{0x00, 0x00, 0x01, 0xC1}
)

// Builtin endpoint availability bitmask (§8.5.4.3 / §9.6.2.2).
const (
	EndpointSPDPAnnouncer    uint32 = 1 << 0
	EndpointSPDPDetector     uint32 = 1 << 1
	EndpointSEDPPubAnnouncer uint32 = 1 << 2
	EndpointSEDPPubDetector  uint32 = 1 << 3
	EndpointSEDPSubAnnouncer uint32 = 1 << 4
	EndpointSEDPSubDetector  uint32 = 1 << 5
)

// newGuidPrefix generates a random GuidPrefix, stamping the low 4 bytes
// with the process PID so participants on the same host are distinguishable.
func newGuidPrefix() GuidPrefix {
	var p GuidPrefix
	ignoredVal, err := rand.Read(p[:8])
	_ = ignoredVal
	if err != nil {
		// Fall through: PID bytes alone will distinguish the participant.
		_ = err
	}
	pid := uint32(os.Getpid())
	p[8] = byte(pid)
	p[9] = byte(pid >> 8)
	p[10] = byte(pid >> 16)
	p[11] = byte(pid >> 24)
	return p
}

// entityIdForWriter returns a user-defined writer EntityId (kind 0x03 = no key).
func entityIdForWriter(n uint32) EntityId {
	return EntityId{byte(n >> 16), byte(n >> 8), byte(n), 0x03}
}

// entityIdForReader returns a user-defined reader EntityId (kind 0x04 = no key).
func entityIdForReader(n uint32) EntityId {
	return EntityId{byte(n >> 16), byte(n >> 8), byte(n), 0x04}
}

func (p GuidPrefix) String() string { return hex.EncodeToString(p[:]) }
func (e EntityId) String() string   { return hex.EncodeToString(e[:]) }
func (g GUID) String() string       { return fmt.Sprintf("%s/%s", g.Prefix, g.Entity) }
