// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Package cdr implements CDR/XCDR1 (Common Data Representation) encoding and
// decoding as specified by the OMG DDS-XTypes 1.3 standard.
//
// CDR encodes DDS data types on the wire using little-endian byte order and
// natural alignment. The encapsulation header (4 bytes: {0x00,0x01,0x00,0x00}
// for CDR_LE) is prepended by the Encoder and consumed by the Decoder.
//
// # Alignment
//
// Each primitive type is aligned to its own size (bool/int8: 1, int16: 2,
// int32/float32: 4, int64/float64: 8). Strings are encoded as a 4-byte
// length prefix (including null terminator) followed by the UTF-8 bytes and
// a null byte. Byte sequences use a 4-byte count prefix.
//
// # Usage
//
//	// Encode
//	e := cdr.NewEncoder()
//	e.WriteString("hello")
//	e.WriteInt32(42)
//	data := e.Bytes()
//
//	// Decode
//	d, err := cdr.NewDecoder(data)
//	s, _ := d.ReadString()
//	n, _ := d.ReadInt32()
package cdr

//fusa:req REQ-CDR-004
//fusa:req REQ-CDR-005
//fusa:req REQ-CDR-006

import (
	"encoding/binary"
	"fmt"
	"math"
)

const (
	// encapHeader is the CDR_LE encapsulation header (§10.2 Table 10.1).
	// Bytes 0–1: scheme 0x0001 (CDR little-endian), bytes 2–3: options (0).
	encapHeaderLen = 4
)

// encapHeader is CDR_LE: scheme 0x0001 written little-endian = bytes {0x01,0x00}.
var encapHeader = [4]byte{0x01, 0x00, 0x00, 0x00}

// Encoder writes CDR/XCDR1 little-endian bytes to an internal buffer.
// Call Bytes() to retrieve the complete encoded message including the
// 4-byte encapsulation header.
type Encoder struct {
	buf []byte
}

// NewEncoder returns an Encoder pre-seeded with the CDR_LE encapsulation header.
func NewEncoder() *Encoder {
	e := &Encoder{}
	e.buf = append(e.buf, encapHeader[:]...)
	return e
}

// Bytes returns the complete encoded buffer including the encapsulation header.
func (e *Encoder) Bytes() []byte { return e.buf }

// Len returns the current encoded length in bytes.
func (e *Encoder) Len() int { return len(e.buf) }

// ── Alignment ─────────────────────────────────────────────────────────────────

func (e *Encoder) align(n int) {
	pad := (n - (len(e.buf) % n)) % n
	for i := 0; i < pad; i++ {
		e.buf = append(e.buf, 0)
	}
}

// ── Primitive writes ──────────────────────────────────────────────────────────

// WriteBool encodes a boolean (1 byte: 0 or 1).
func (e *Encoder) WriteBool(v bool) {
	if v {
		e.buf = append(e.buf, 1)
	} else {
		e.buf = append(e.buf, 0)
	}
}

// WriteUint8 encodes an unsigned byte.
func (e *Encoder) WriteUint8(v uint8) { e.buf = append(e.buf, v) }

// WriteInt8 encodes a signed byte.
func (e *Encoder) WriteInt8(v int8) { e.buf = append(e.buf, byte(v)) }

// WriteInt16 encodes a signed 16-bit integer (2-byte aligned).
func (e *Encoder) WriteInt16(v int16) {
	e.align(2)
	e.buf = binary.LittleEndian.AppendUint16(e.buf, uint16(v))
}

// WriteUint16 encodes an unsigned 16-bit integer (2-byte aligned).
func (e *Encoder) WriteUint16(v uint16) {
	e.align(2)
	e.buf = binary.LittleEndian.AppendUint16(e.buf, v)
}

// WriteInt32 encodes a signed 32-bit integer (4-byte aligned).
func (e *Encoder) WriteInt32(v int32) {
	e.align(4)
	e.buf = binary.LittleEndian.AppendUint32(e.buf, uint32(v))
}

// WriteUint32 encodes an unsigned 32-bit integer (4-byte aligned).
func (e *Encoder) WriteUint32(v uint32) {
	e.align(4)
	e.buf = binary.LittleEndian.AppendUint32(e.buf, v)
}

// WriteInt64 encodes a signed 64-bit integer (8-byte aligned).
func (e *Encoder) WriteInt64(v int64) {
	e.align(8)
	e.buf = binary.LittleEndian.AppendUint64(e.buf, uint64(v))
}

// WriteUint64 encodes an unsigned 64-bit integer (8-byte aligned).
func (e *Encoder) WriteUint64(v uint64) {
	e.align(8)
	e.buf = binary.LittleEndian.AppendUint64(e.buf, v)
}

// WriteFloat32 encodes a 32-bit IEEE 754 float (4-byte aligned).
func (e *Encoder) WriteFloat32(v float32) {
	e.align(4)
	e.buf = binary.LittleEndian.AppendUint32(e.buf, math.Float32bits(v))
}

// WriteFloat64 encodes a 64-bit IEEE 754 float (8-byte aligned).
func (e *Encoder) WriteFloat64(v float64) {
	e.align(8)
	e.buf = binary.LittleEndian.AppendUint64(e.buf, math.Float64bits(v))
}

// WriteString encodes a CDR string: uint32 length (including null terminator),
// UTF-8 bytes, null byte. The length field is 4-byte aligned.
func (e *Encoder) WriteString(s string) {
	e.align(4)
	e.buf = binary.LittleEndian.AppendUint32(e.buf, uint32(len(s)+1))
	e.buf = append(e.buf, s...)
	e.buf = append(e.buf, 0) // null terminator
}

// WriteBytes encodes a byte sequence: uint32 count + raw bytes.
func (e *Encoder) WriteBytes(b []byte) {
	e.align(4)
	e.buf = binary.LittleEndian.AppendUint32(e.buf, uint32(len(b)))
	e.buf = append(e.buf, b...)
}

// ── Decoder ───────────────────────────────────────────────────────────────────

// Decoder reads CDR/XCDR1 little-endian bytes from a buffer.
// The buffer must begin with the 4-byte CDR_LE encapsulation header.
type Decoder struct {
	buf []byte
	pos int
}

// NewDecoder creates a Decoder for data. Returns an error if the encapsulation
// header is missing or has an unsupported scheme.
func NewDecoder(data []byte) (*Decoder, error) {
	if len(data) < encapHeaderLen {
		return nil, fmt.Errorf("cdr: data too short for encapsulation header (%d bytes)", len(data))
	}
	// Accept CDR_LE (0x0001) and CDR_BE (0x0000); we decode LE only.
	scheme := binary.LittleEndian.Uint16(data[0:2])
	if scheme != 0x0001 && scheme != 0x0000 {
		return nil, fmt.Errorf("cdr: unsupported encapsulation scheme 0x%04x", scheme)
	}
	return &Decoder{buf: data, pos: encapHeaderLen}, nil
}

// Remaining returns the number of undecoded bytes.
func (d *Decoder) Remaining() int { return len(d.buf) - d.pos }

func (d *Decoder) align(n int) {
	pad := (n - (d.pos % n)) % n
	d.pos += pad
}

func (d *Decoder) need(n int) error {
	if d.pos+n > len(d.buf) {
		return fmt.Errorf("cdr: buffer underrun: need %d bytes at offset %d, have %d", n, d.pos, len(d.buf))
	}
	return nil
}

// ReadBool decodes a boolean byte.
func (d *Decoder) ReadBool() (bool, error) {
	if err := d.need(1); err != nil {
		return false, err
	}
	v := d.buf[d.pos] != 0
	d.pos++
	return v, nil
}

// ReadUint8 decodes an unsigned byte.
func (d *Decoder) ReadUint8() (uint8, error) {
	if err := d.need(1); err != nil {
		return 0, err
	}
	v := d.buf[d.pos]
	d.pos++
	return v, nil
}

// ReadInt8 decodes a signed byte.
func (d *Decoder) ReadInt8() (int8, error) {
	v, err := d.ReadUint8()
	if err != nil {
		return 0, err
	}
	return int8(v), nil
}

// ReadInt16 decodes a signed 16-bit integer.
func (d *Decoder) ReadInt16() (int16, error) {
	d.align(2)
	if err := d.need(2); err != nil {
		return 0, err
	}
	v := int16(binary.LittleEndian.Uint16(d.buf[d.pos:]))
	d.pos += 2
	return v, nil
}

// ReadUint16 decodes an unsigned 16-bit integer.
func (d *Decoder) ReadUint16() (uint16, error) {
	d.align(2)
	if err := d.need(2); err != nil {
		return 0, err
	}
	v := binary.LittleEndian.Uint16(d.buf[d.pos:])
	d.pos += 2
	return v, nil
}

// ReadInt32 decodes a signed 32-bit integer.
func (d *Decoder) ReadInt32() (int32, error) {
	d.align(4)
	if err := d.need(4); err != nil {
		return 0, err
	}
	v := int32(binary.LittleEndian.Uint32(d.buf[d.pos:]))
	d.pos += 4
	return v, nil
}

// ReadUint32 decodes an unsigned 32-bit integer.
func (d *Decoder) ReadUint32() (uint32, error) {
	d.align(4)
	if err := d.need(4); err != nil {
		return 0, err
	}
	v := binary.LittleEndian.Uint32(d.buf[d.pos:])
	d.pos += 4
	return v, nil
}

// ReadInt64 decodes a signed 64-bit integer.
func (d *Decoder) ReadInt64() (int64, error) {
	d.align(8)
	if err := d.need(8); err != nil {
		return 0, err
	}
	v := int64(binary.LittleEndian.Uint64(d.buf[d.pos:]))
	d.pos += 8
	return v, nil
}

// ReadUint64 decodes an unsigned 64-bit integer.
func (d *Decoder) ReadUint64() (uint64, error) {
	d.align(8)
	if err := d.need(8); err != nil {
		return 0, err
	}
	v := binary.LittleEndian.Uint64(d.buf[d.pos:])
	d.pos += 8
	return v, nil
}

// ReadFloat32 decodes a 32-bit IEEE 754 float.
func (d *Decoder) ReadFloat32() (float32, error) {
	v, err := d.ReadUint32()
	if err != nil {
		return 0, err
	}
	return math.Float32frombits(v), nil
}

// ReadFloat64 decodes a 64-bit IEEE 754 float.
func (d *Decoder) ReadFloat64() (float64, error) {
	v, err := d.ReadUint64()
	if err != nil {
		return 0, err
	}
	return math.Float64frombits(v), nil
}

// ReadString decodes a CDR string (uint32 length + bytes + null terminator).
func (d *Decoder) ReadString() (string, error) {
	n, err := d.ReadUint32()
	if err != nil {
		return "", err
	}
	if n == 0 {
		return "", nil
	}
	if err := d.need(int(n)); err != nil {
		return "", fmt.Errorf("cdr: string data: %w", err)
	}
	raw := d.buf[d.pos : d.pos+int(n)]
	d.pos += int(n)
	// Strip null terminator.
	if len(raw) > 0 && raw[len(raw)-1] == 0 {
		raw = raw[:len(raw)-1]
	}
	return string(raw), nil
}

// ReadBytes decodes a byte sequence (uint32 count + bytes).
func (d *Decoder) ReadBytes() ([]byte, error) {
	n, err := d.ReadUint32()
	if err != nil {
		return nil, err
	}
	if err := d.need(int(n)); err != nil {
		return nil, fmt.Errorf("cdr: bytes data: %w", err)
	}
	out := make([]byte, n)
	copy(out, d.buf[d.pos:d.pos+int(n)])
	d.pos += int(n)
	return out, nil
}
