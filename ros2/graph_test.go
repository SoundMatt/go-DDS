// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package ros2

import (
	"bytes"
	"reflect"
	"testing"
)

func gidSeq(vals ...byte) []Gid {
	out := make([]Gid, len(vals))
	for i, v := range vals {
		out[i] = Gid{}
		out[i][0] = v
	}
	return out
}

// TestParticipantEntitiesInfo_RoundTrip exercises Encode/Decode against
// itself across a range of shapes (empty graph, multiple nodes, non-empty
// reader/writer sequences) — the paired-consistency check.
func TestParticipantEntitiesInfo_RoundTrip(t *testing.T) {
	var gid1, gid2 Gid
	for i := range gid1 {
		gid1[i] = byte(i)
	}
	for i := range gid2 {
		gid2[i] = byte(0xF0 + i)
	}

	cases := []ParticipantEntitiesInfo{
		{Gid: gid1},
		{
			Gid: gid1,
			NodeEntitiesInfoSeq: []NodeEntitiesInfo{
				{NodeNamespace: "/", NodeName: "n"},
			},
		},
		{
			Gid: gid2,
			NodeEntitiesInfoSeq: []NodeEntitiesInfo{
				{
					NodeNamespace: "/robot1",
					NodeName:      "camera_driver",
					ReaderGidSeq:  gidSeq(1, 2, 3),
					WriterGidSeq:  gidSeq(4, 5),
				},
				{
					NodeNamespace: "/",
					NodeName:      "talker",
					WriterGidSeq:  gidSeq(9),
				},
			},
		},
	}

	for i, want := range cases {
		encoded := want.Encode()
		got, ok := DecodeParticipantEntitiesInfo(encoded)
		if !ok {
			t.Fatalf("case %d: DecodeParticipantEntitiesInfo: ok = false", i)
		}
		if !reflect.DeepEqual(normalizeInfo(got), normalizeInfo(want)) {
			t.Errorf("case %d: round trip mismatch:\n got=%+v\nwant=%+v", i, got, want)
		}
	}
}

// normalizeInfo replaces nil slices with empty ones so reflect.DeepEqual
// doesn't distinguish "no elements, nil" from "no elements, empty" — Decode
// always produces non-nil empty slices via make(..., 0, n).
func normalizeInfo(info ParticipantEntitiesInfo) ParticipantEntitiesInfo {
	if info.NodeEntitiesInfoSeq == nil {
		info.NodeEntitiesInfoSeq = []NodeEntitiesInfo{}
	}
	for i := range info.NodeEntitiesInfoSeq {
		if info.NodeEntitiesInfoSeq[i].ReaderGidSeq == nil {
			info.NodeEntitiesInfoSeq[i].ReaderGidSeq = []Gid{}
		}
		if info.NodeEntitiesInfoSeq[i].WriterGidSeq == nil {
			info.NodeEntitiesInfoSeq[i].WriterGidSeq = []Gid{}
		}
	}
	return info
}

// TestParticipantEntitiesInfo_GoldenBytes hand-assembles the exact Plain
// CDR, little-endian wire bytes for a minimal ParticipantEntitiesInfo
// (rmw_dds_common's own message shape) independently of cdrWriter, then
// checks both directions against it: Encode produces exactly these bytes,
// and Decode of exactly these bytes produces the expected struct. This is
// the strongest check available without a live ROS 2 peer that Encode/
// Decode implement the real OMG CDR wire format — not just a
// self-consistent one — since golden-byte construction below never calls
// cdrWriter at all.
func TestParticipantEntitiesInfo_GoldenBytes(t *testing.T) {
	var gid Gid
	for i := range gid {
		gid[i] = byte(i)
	}
	info := ParticipantEntitiesInfo{
		Gid: gid,
		NodeEntitiesInfoSeq: []NodeEntitiesInfo{
			{NodeNamespace: "/", NodeName: "n"},
		},
	}

	golden := []byte{
		0x00, 0x01, 0x00, 0x00, // encapsulation header: CDR_LE, options=0
	}
	golden = append(golden, gid[:]...)              // Gid: 24 raw bytes, no length prefix
	golden = append(golden, 0x01, 0x00, 0x00, 0x00) // node_entities_info_seq length = 1
	golden = append(golden, 0x02, 0x00, 0x00, 0x00) // node_namespace length = len("/")+1
	golden = append(golden, '/', 0x00)              // "/\0"
	golden = append(golden, 0x00, 0x00)             // pad to 4-byte alignment
	golden = append(golden, 0x02, 0x00, 0x00, 0x00) // node_name length = len("n")+1
	golden = append(golden, 'n', 0x00)              // "n\0"
	golden = append(golden, 0x00, 0x00)             // pad to 4-byte alignment
	golden = append(golden, 0x00, 0x00, 0x00, 0x00) // reader_gid_seq length = 0
	golden = append(golden, 0x00, 0x00, 0x00, 0x00) // writer_gid_seq length = 0

	got := info.Encode()
	if !bytes.Equal(got, golden) {
		t.Fatalf("Encode mismatch:\n got=% x\nwant=% x", got, golden)
	}

	decoded, ok := DecodeParticipantEntitiesInfo(golden)
	if !ok {
		t.Fatal("DecodeParticipantEntitiesInfo(golden): ok = false")
	}
	if decoded.Gid != info.Gid {
		t.Errorf("decoded.Gid = % x, want % x", decoded.Gid, info.Gid)
	}
	if len(decoded.NodeEntitiesInfoSeq) != 1 {
		t.Fatalf("len(decoded.NodeEntitiesInfoSeq) = %d, want 1", len(decoded.NodeEntitiesInfoSeq))
	}
	n := decoded.NodeEntitiesInfoSeq[0]
	if n.NodeNamespace != "/" || n.NodeName != "n" {
		t.Errorf("decoded node = %+v, want {NodeNamespace:/ NodeName:n}", n)
	}
	if len(n.ReaderGidSeq) != 0 || len(n.WriterGidSeq) != 0 {
		t.Errorf("decoded node has non-empty gid seqs: %+v", n)
	}
}

func TestDecodeParticipantEntitiesInfo_Truncated(t *testing.T) {
	cases := [][]byte{
		nil,
		{0x00, 0x01},             // too short even for the header
		{0x00, 0x01, 0x00, 0x00}, // header only, missing the 24-byte Gid
		{0x00, 0x01, 0x00, 0x00, 0x01, 0x02, 0x03}, // header + partial Gid
	}
	for i, data := range cases {
		if _, ok := DecodeParticipantEntitiesInfo(data); ok {
			t.Errorf("case %d: ok = true for truncated input %v, want false", i, data)
		}
	}
}

func TestDecodeParticipantEntitiesInfo_WrongEncapsulation(t *testing.T) {
	// representation_identifier 0x0000 == CDR_BE, which this package does
	// not implement decoding for.
	data := append([]byte{0x00, 0x00, 0x00, 0x00}, make([]byte, 24)...)
	if _, ok := DecodeParticipantEntitiesInfo(data); ok {
		t.Error("ok = true for CDR_BE encapsulation, want false (unsupported)")
	}
}

func TestGidFromRTPS(t *testing.T) {
	var prefix [12]byte
	var entity [4]byte
	for i := range prefix {
		prefix[i] = byte(i + 1)
	}
	for i := range entity {
		entity[i] = byte(0xA0 + i)
	}
	g := GidFromRTPS(prefix, entity)
	for i := 0; i < 12; i++ {
		if g[i] != prefix[i] {
			t.Errorf("g[%d] = %x, want prefix[%d] = %x", i, g[i], i, prefix[i])
		}
	}
	for i := 0; i < 4; i++ {
		if g[12+i] != entity[i] {
			t.Errorf("g[%d] = %x, want entity[%d] = %x", 12+i, g[12+i], i, entity[i])
		}
	}
	for i := 16; i < 24; i++ {
		if g[i] != 0 {
			t.Errorf("g[%d] = %x, want 0 (zero-padded)", i, g[i])
		}
	}
}
