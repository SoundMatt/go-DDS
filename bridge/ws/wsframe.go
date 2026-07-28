// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package ws

// A minimal RFC 6455 WebSocket frame codec, server-role only: this package
// only ever accepts inbound browser/JS-client connections (see ws.go's
// ServeHTTP), it never dials out, so unlike rtps/transport_ws.go's wsConn
// (which plays both client and server roles for peer-to-peer RTPS-over-WS)
// this codec always writes unmasked frames and always expects — and
// unmasks — masked frames on read, per RFC 6455 §5.1's client/server
// masking rule. It is intentionally not shared code with
// rtps/transport_ws.go: this is a separate Go module (bridge, see go.mod's
// `replace` directive) with no access to that package's unexported types,
// and the two protocols this codec and that one carry (this package's JSON
// pub/sub gateway protocol vs. raw RTPS) are genuinely different anyway —
// see ws.go's package doc comment.

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
)

// maxFrameBytes bounds a single WebSocket message this gateway will accept
// from a client, guarding against a hostile or buggy client claiming an
// unbounded payload length — the same protection go-DDS's other
// length-prefixed/frame-based transports and bridges (see e.g. bridge/wan's
// maxFrameBytes) apply to their own inputs.
const maxFrameBytes = 1 << 20 // 1 MiB

// WebSocket opcodes (RFC 6455 §5.2).
const (
	opContinuation = 0x0
	opText         = 0x1
	opBinary       = 0x2
	opClose        = 0x8
	opPing         = 0x9
	opPong         = 0xA
)

// readMessage reads one complete WebSocket message from br, reassembling
// any continuation frames (RFC 6455 §5.4), unmasking every frame (a
// well-behaved client always masks; see writeMessage's doc comment for why
// this side never does), and transparently answering Ping frames with a
// Pong on w without returning them to the caller. Returns
// (opClose, payload, nil) when a Close frame is received.
func readMessage(br *bufio.Reader, w io.Writer) (opcode byte, payload []byte, err error) {
	var buf []byte
	msgOp := byte(0xFF) // unset until the first non-continuation data frame
	for {
		fin, op, frame, ferr := readFrame(br)
		if ferr != nil {
			return 0, nil, ferr
		}
		switch op {
		case opPing:
			if werr := writeFrame(w, opPong, frame); werr != nil {
				return 0, nil, werr
			}
			continue
		case opPong:
			continue
		case opClose:
			return opClose, frame, nil
		case opContinuation:
			buf = append(buf, frame...)
		default: // opText / opBinary: starts a new (possibly fragmented) message
			msgOp = op
			buf = append(buf, frame...)
		}
		if len(buf) > maxFrameBytes {
			return 0, nil, fmt.Errorf("ws: message too large: > %d bytes", maxFrameBytes)
		}
		if fin {
			return msgOp, buf, nil
		}
	}
}

// readFrame reads and decodes exactly one WebSocket frame header plus
// payload from br (RFC 6455 §5.2), unmasking the payload (every frame from
// a spec-compliant client is masked). It rejects any claimed payload length
// exceeding maxFrameBytes before allocating a buffer for it.
func readFrame(br *bufio.Reader) (fin bool, opcode byte, payload []byte, err error) {
	var hdr [2]byte
	if _, err = io.ReadFull(br, hdr[:]); err != nil {
		return false, 0, nil, err
	}
	fin = hdr[0]&0x80 != 0
	opcode = hdr[0] & 0x0F
	masked := hdr[1]&0x80 != 0
	plen := uint64(hdr[1] & 0x7F)

	switch plen {
	case 126:
		var ext [2]byte
		if _, err = io.ReadFull(br, ext[:]); err != nil {
			return false, 0, nil, err
		}
		plen = uint64(binary.BigEndian.Uint16(ext[:]))
	case 127:
		var ext [8]byte
		if _, err = io.ReadFull(br, ext[:]); err != nil {
			return false, 0, nil, err
		}
		plen = binary.BigEndian.Uint64(ext[:])
	}
	if plen > maxFrameBytes {
		return false, 0, nil, fmt.Errorf("ws: frame too large: %d bytes", plen)
	}

	var maskKey [4]byte
	if masked {
		if _, err = io.ReadFull(br, maskKey[:]); err != nil {
			return false, 0, nil, err
		}
	}

	payload = make([]byte, plen)
	if _, err = io.ReadFull(br, payload); err != nil {
		return false, 0, nil, err
	}
	if masked {
		for i := range payload {
			payload[i] ^= maskKey[i%4]
		}
	}
	return fin, opcode, payload, nil
}

// writeMessage writes data as a single unfragmented (FIN=1) unmasked TEXT
// frame to w — this gateway's protocol is always JSON (see ws.go), so every
// outbound message uses opText. Per RFC 6455 §5.1, a server must never mask
// the frames it sends, which is why writeFrame is always called with
// mask=false from this package.
func writeMessage(w io.Writer, data []byte) error {
	return writeFrame(w, opText, data)
}

// writeFrame writes a single, unfragmented (FIN=1), unmasked WebSocket
// frame carrying opcode/payload to w.
func writeFrame(w io.Writer, opcode byte, payload []byte) error {
	n := len(payload)
	var hdr [10]byte
	hdr[0] = 0x80 | (opcode & 0x0F) // FIN=1, RSV1-3=0
	var hdrLen int
	switch {
	case n < 126:
		hdr[1] = byte(n)
		hdrLen = 2
	case n <= 0xFFFF:
		hdr[1] = 126
		binary.BigEndian.PutUint16(hdr[2:4], uint16(n))
		hdrLen = 4
	default:
		hdr[1] = 127
		binary.BigEndian.PutUint64(hdr[2:10], uint64(n))
		hdrLen = 10
	}
	out := make([]byte, 0, hdrLen+n)
	out = append(out, hdr[:hdrLen]...)
	out = append(out, payload...)
	_, err := w.Write(out)
	return err
}
