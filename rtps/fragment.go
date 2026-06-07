// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package rtps

import (
	"encoding/binary"
	"sync"
	"time"
)

// maxFragmentPayload is the maximum bytes placed in a single DATA_FRAG body.
// Chosen to keep the full RTPS packet under 1400 bytes on a typical Ethernet
// MTU of 1500 bytes (headers ≈ 100 bytes).
const maxFragmentPayload = 1200

// maxReassemblyBytes is the maximum DataSize accepted from an incoming
// DATA_FRAG submessage. Frames claiming a larger size are discarded to
// prevent memory exhaustion from malformed or malicious peers.
const maxReassemblyBytes = 16 * 1024 * 1024 // 16 MiB

// staleFragAge is how long an incomplete fragment reassembly is held before
// being discarded. This bounds memory use when fragments are permanently lost.
const staleFragAge = 5 * time.Second

// submsgDATAFRAG is the RTPS submessage ID for DATA_FRAG (§8.3.7.3).
const submsgDATAFRAG = byte(0x16)

// DataFrag carries the fields of an RTPS DATA_FRAG submessage.
type DataFrag struct {
	WriterEntityId      EntityId
	ReaderEntityId      EntityId
	WriterSeqNum        SequenceNumber
	FragmentStartingNum uint32 // 1-based index of the first fragment in this submessage
	FragmentsInSubmsg   uint16 // number of fragments in this submessage
	FragmentSize        uint16 // size of each fragment in bytes (last may be smaller)
	DataSize            uint32 // total (unfragmented) data size in bytes
	Payload             []byte // raw bytes of the fragment(s)
}

// marshalDataFrag serialises a DataFrag into a complete RTPS submessage byte
// slice ready to pass to wrapInRTPSMessage.
func marshalDataFrag(f DataFrag) []byte {
	// Header: extraFlags(2)+octetsToInlineQos(2)+readerEID(4)+writerEID(4)+
	//         writerSeqNum(8)+fragmentStartingNum(4)+fragmentsInSubmsg(2)+
	//         fragmentSize(2)+dataSize(4) = 32 bytes of fixed fields.
	fixedLen := 32
	bodyLen := fixedLen + len(f.Payload)
	buf := make([]byte, 4+bodyLen) // 4-byte submsg header + body

	// Submessage header: id | flags | octetsToNextHeader(LE).
	buf[0] = submsgDATAFRAG
	buf[1] = flagEndianness // little-endian flag
	binary.LittleEndian.PutUint16(buf[2:4], uint16(bodyLen))

	body := buf[4:]
	// extraFlags (2) + octetsToInlineQos (2).
	binary.LittleEndian.PutUint16(body[0:2], 0)
	binary.LittleEndian.PutUint16(body[2:4], 0)
	// readerEntityId (4).
	copy(body[4:8], f.ReaderEntityId[:])
	// writerEntityId (4).
	copy(body[8:12], f.WriterEntityId[:])
	// writerSeqNum (8).
	binary.LittleEndian.PutUint32(body[12:16], uint32(f.WriterSeqNum.High))
	binary.LittleEndian.PutUint32(body[16:20], f.WriterSeqNum.Low)
	// fragmentStartingNum (4).
	binary.LittleEndian.PutUint32(body[20:24], f.FragmentStartingNum)
	// fragmentsInSubmsg (2).
	binary.LittleEndian.PutUint16(body[24:26], f.FragmentsInSubmsg)
	// fragmentSize (2).
	binary.LittleEndian.PutUint16(body[26:28], f.FragmentSize)
	// dataSize (4).
	binary.LittleEndian.PutUint32(body[28:32], f.DataSize)
	// fragment payload.
	copy(body[32:], f.Payload)
	return buf
}

// parseDataFrag parses the body of a DATA_FRAG submessage. ok is false when
// the buffer is too short.
func parseDataFrag(body []byte) (DataFrag, bool) {
	if len(body) < 32 {
		return DataFrag{}, false
	}
	var f DataFrag
	// skip extraFlags(2) + octetsToInlineQos(2).
	copy(f.ReaderEntityId[:], body[4:8])
	copy(f.WriterEntityId[:], body[8:12])
	f.WriterSeqNum.High = int32(binary.LittleEndian.Uint32(body[12:16]))
	f.WriterSeqNum.Low = binary.LittleEndian.Uint32(body[16:20])
	f.FragmentStartingNum = binary.LittleEndian.Uint32(body[20:24])
	f.FragmentsInSubmsg = binary.LittleEndian.Uint16(body[24:26])
	f.FragmentSize = binary.LittleEndian.Uint16(body[26:28])
	f.DataSize = binary.LittleEndian.Uint32(body[28:32])
	f.Payload = append([]byte(nil), body[32:]...)
	return f, true
}

// fragmentAssembler reassembles DATA_FRAG submessages for a single writer+seqnum pair.
type fragmentAssembler struct {
	mu      sync.Mutex
	buffers map[fragKey]*fragBuffer
}

type fragKey struct {
	writer EntityId
	seqLo  uint32
}

type fragBuffer struct {
	data     []byte
	received uint32    // count of fragments received so far
	total    uint32    // total number of fragments expected
	created  time.Time // wall time when the first fragment was received
}

// receive adds a fragment and returns the complete reassembled payload if all
// fragments have arrived, otherwise returns nil.
func (fa *fragmentAssembler) receive(f DataFrag) []byte {
	if f.FragmentSize == 0 || f.DataSize == 0 || f.FragmentsInSubmsg == 0 {
		return nil
	}
	// Reject implausibly large payloads before allocating any memory.
	if uint64(f.DataSize) > maxReassemblyBytes {
		return nil
	}
	total := (uint32(f.DataSize) + uint32(f.FragmentSize) - 1) / uint32(f.FragmentSize)

	key := fragKey{writer: f.WriterEntityId, seqLo: f.WriterSeqNum.Low}
	now := time.Now()
	fa.mu.Lock()
	defer fa.mu.Unlock()
	if fa.buffers == nil {
		fa.buffers = make(map[fragKey]*fragBuffer)
	}
	// Evict incomplete reassemblies that are older than staleFragAge.
	// This prevents memory accumulation from lost or abandoned fragment streams.
	for k, b := range fa.buffers {
		if now.Sub(b.created) > staleFragAge {
			delete(fa.buffers, k)
		}
	}
	fb, ok := fa.buffers[key]
	if !ok {
		fb = &fragBuffer{
			data:    make([]byte, f.DataSize),
			total:   total,
			created: now,
		}
		fa.buffers[key] = fb
	}

	// Copy each fragment's bytes into the correct position.
	fragIdx := f.FragmentStartingNum - 1 // convert to 0-based
	for i := uint16(0); i < f.FragmentsInSubmsg; i++ {
		offset := (fragIdx + uint32(i)) * uint32(f.FragmentSize)
		if offset >= uint32(f.DataSize) {
			break
		}
		fragStart := uint32(i) * uint32(f.FragmentSize)
		fragEnd := fragStart + uint32(f.FragmentSize)
		if fragEnd > uint32(len(f.Payload)) {
			fragEnd = uint32(len(f.Payload))
		}
		end := offset + (fragEnd - fragStart)
		if end > uint32(f.DataSize) {
			end = uint32(f.DataSize)
		}
		copy(fb.data[offset:end], f.Payload[fragStart:fragEnd])
		fb.received++
	}

	if fb.received >= fb.total {
		result := fb.data
		delete(fa.buffers, key)
		return result
	}
	return nil
}

// splitIntoFragments breaks payload into DataFrag slices using the default
// maxFragmentPayload size. writerEID and seqNum identify the writer.
func splitIntoFragments(writerEID EntityId, seqNum SequenceNumber, payload []byte) []DataFrag {
	return splitIntoFragmentsN(writerEID, seqNum, payload, maxFragmentPayload)
}

// splitIntoFragmentsN breaks payload into DataFrag slices with at most
// maxPayloadSize bytes per fragment. Use maxFragmentPayload for the default.
// For TSN streams, pass Stream.MaxFragPayload() to honour the frame-size bound.
func splitIntoFragmentsN(writerEID EntityId, seqNum SequenceNumber, payload []byte, maxPayloadSize int) []DataFrag {
	if maxPayloadSize <= 0 {
		maxPayloadSize = maxFragmentPayload
	}
	size := uint16(maxPayloadSize)
	total := len(payload)
	numFrags := uint32((total + int(size) - 1) / int(size))
	frags := make([]DataFrag, 0, numFrags)
	offset := 0
	fragNum := uint32(1) // 1-based
	for offset < total {
		end := offset + int(size)
		if end > total {
			end = total
		}
		chunk := payload[offset:end]
		frags = append(frags, DataFrag{
			WriterEntityId:      writerEID,
			ReaderEntityId:      EntityIdUnknown,
			WriterSeqNum:        seqNum,
			FragmentStartingNum: fragNum,
			FragmentsInSubmsg:   1,
			FragmentSize:        size,
			DataSize:            uint32(total),
			Payload:             chunk,
		})
		offset = end
		fragNum++
	}
	return frags
}
