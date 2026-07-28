// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// ROS 2 graph introspection: every conformant rmw implementation
// (rmw_fastrtps, rmw_cyclonedds) publishes a rmw_dds_common/msg/
// ParticipantEntitiesInfo sample, TRANSIENT_LOCAL + RELIABLE, on the
// well-known "ros_discovery_info" topic (DiscoveryTopicName) every time its
// set of local nodes or their reader/writer GUIDs changes. That sample is
// the *entire* current state for that participant (not a delta), which is
// exactly how `ros2 node list` / `ros2 topic list` learn about the graph
// without a central registry: every participant is its own source of
// truth for its own nodes, and every peer's copy is refreshed wholesale on
// each change. graph.go implements the wire format (a plain, non-parameter-
// list CDR message — see cdr.go) and the message shape itself, unchanged
// since ROS 2 Dashing and still current in Jazzy/Rolling.
package ros2

// Gid is rmw_dds_common's 24-byte participant/entity identifier. DDS GUIDs
// are 16 bytes (RTPS 2.3 §9.3.1); rmw_dds_common historically over-allocates
// to 24 bytes for headroom across vendors, zero-padding the remainder.
type Gid [24]byte

// GidFromRTPS builds a Gid from a 16-byte RTPS GUID (12-byte prefix + 4-byte
// entity id), zero-padding the remaining 8 bytes.
func GidFromRTPS(guidPrefix [12]byte, entityID [4]byte) Gid {
	var g Gid
	copy(g[0:12], guidPrefix[:])
	copy(g[12:16], entityID[:])
	return g
}

// NodeEntitiesInfo describes one ROS 2 node hosted by a participant: its
// fully-qualified namespace/name, and the GUIDs of every reader/writer it
// owns (rmw_dds_common/msg/NodeEntitiesInfo).
type NodeEntitiesInfo struct {
	NodeNamespace string
	NodeName      string
	ReaderGidSeq  []Gid
	WriterGidSeq  []Gid
}

// ParticipantEntitiesInfo is the full graph-state sample a participant
// publishes on DiscoveryTopicName: its own participant Gid, and every node
// it currently hosts (rmw_dds_common/msg/ParticipantEntitiesInfo).
type ParticipantEntitiesInfo struct {
	Gid                 Gid
	NodeEntitiesInfoSeq []NodeEntitiesInfo
}

// Encode serializes info as Plain CDR, little-endian (the encapsulation
// every ROS 2 rmw implementation uses for ordinary topic user data).
func (info ParticipantEntitiesInfo) Encode() []byte {
	w := newCDRWriter()
	w.rawBytes(info.Gid[:])
	w.uint32(uint32(len(info.NodeEntitiesInfoSeq)))
	for _, n := range info.NodeEntitiesInfoSeq {
		w.str(n.NodeNamespace)
		w.str(n.NodeName)
		w.uint32(uint32(len(n.ReaderGidSeq)))
		for _, g := range n.ReaderGidSeq {
			w.rawBytes(g[:])
		}
		w.uint32(uint32(len(n.WriterGidSeq)))
		for _, g := range n.WriterGidSeq {
			w.rawBytes(g[:])
		}
	}
	return w.finish()
}

// DecodeParticipantEntitiesInfo parses the Plain-CDR-LE wire format Encode
// produces. ok is false if payload is truncated, carries an unsupported
// encapsulation, or exceeds internal sanity limits (see maxSeqLen).
func DecodeParticipantEntitiesInfo(payload []byte) (info ParticipantEntitiesInfo, ok bool) {
	r, ok := newCDRReader(payload)
	if !ok {
		return ParticipantEntitiesInfo{}, false
	}
	gidBytes, ok := r.rawBytes(24)
	if !ok {
		return ParticipantEntitiesInfo{}, false
	}
	copy(info.Gid[:], gidBytes)

	nodeCount, ok := r.uint32()
	if !ok || nodeCount > maxSeqLen {
		return ParticipantEntitiesInfo{}, false
	}
	info.NodeEntitiesInfoSeq = make([]NodeEntitiesInfo, 0, nodeCount)
	for i := uint32(0); i < nodeCount; i++ {
		var n NodeEntitiesInfo
		if n.NodeNamespace, ok = r.str(); !ok {
			return ParticipantEntitiesInfo{}, false
		}
		if n.NodeName, ok = r.str(); !ok {
			return ParticipantEntitiesInfo{}, false
		}
		readerCount, ok := r.uint32()
		if !ok || readerCount > maxSeqLen {
			return ParticipantEntitiesInfo{}, false
		}
		n.ReaderGidSeq = make([]Gid, 0, readerCount)
		for j := uint32(0); j < readerCount; j++ {
			b, gok := r.rawBytes(24)
			if !gok {
				return ParticipantEntitiesInfo{}, false
			}
			var g Gid
			copy(g[:], b)
			n.ReaderGidSeq = append(n.ReaderGidSeq, g)
		}
		writerCount, ok := r.uint32()
		if !ok || writerCount > maxSeqLen {
			return ParticipantEntitiesInfo{}, false
		}
		n.WriterGidSeq = make([]Gid, 0, writerCount)
		for j := uint32(0); j < writerCount; j++ {
			b, gok := r.rawBytes(24)
			if !gok {
				return ParticipantEntitiesInfo{}, false
			}
			var g Gid
			copy(g[:], b)
			n.WriterGidSeq = append(n.WriterGidSeq, g)
		}
		info.NodeEntitiesInfoSeq = append(info.NodeEntitiesInfoSeq, n)
	}
	return info, true
}
