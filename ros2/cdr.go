// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package ros2

import "encoding/binary"

// cdrWriter is a minimal, alignment-correct OMG CDR (Plain CDR,
// little-endian) encoder — just enough to serialize the fixed-shape
// rmw_dds_common graph messages this package exchanges over the
// "ros_discovery_info" topic (see graph.go). It intentionally is not a
// general-purpose CDR codec: go-DDS's IDL/CDR compiler (tools/cdr, driven
// by the idl package) already fills that role for arbitrary user-defined
// message types. Plain CDR (not PL_CDR/parameter-list, which rtps/cdr.go
// uses for SPDP/SEDP built-in discovery data) is what every DDS vendor —
// including every ROS 2 rmw implementation — uses for ordinary topic user
// data, which is exactly what "ros_discovery_info" is: a normal DDS topic.
type cdrWriter struct {
	buf []byte
}

// cdrLEHeader is the 4-byte CDR encapsulation header (RTPS 2.3 §10.2) for
// Plain CDR, little-endian: representation_identifier = 0x0001 (CDR_LE),
// representation_options = 0x0000.
var cdrLEHeader = [4]byte{0x00, 0x01, 0x00, 0x00}

func newCDRWriter() *cdrWriter {
	w := &cdrWriter{buf: make([]byte, 0, 64)}
	w.buf = append(w.buf, cdrLEHeader[:]...)
	return w
}

// dataOffset is w's position relative to the end of the 4-byte
// encapsulation header — CDR alignment is computed against this offset,
// not against the start of the buffer (RTPS 2.3 §10.2 note).
func (w *cdrWriter) dataOffset() int { return len(w.buf) - 4 }

func (w *cdrWriter) align(n int) {
	if pad := (n - w.dataOffset()%n) % n; pad > 0 {
		w.buf = append(w.buf, make([]byte, pad)...)
	}
}

func (w *cdrWriter) rawBytes(b []byte) { w.buf = append(w.buf, b...) }

func (w *cdrWriter) uint32(v uint32) {
	w.align(4)
	var b [4]byte
	binary.LittleEndian.PutUint32(b[:], v)
	w.buf = append(w.buf, b[:]...)
}

// str writes a CDR string: a uint32 length (including the trailing NUL)
// followed by the string's bytes and a trailing NUL. No alignment padding
// follows the NUL beyond what the next field's own alignment demands.
func (w *cdrWriter) str(s string) {
	w.uint32(uint32(len(s) + 1))
	w.buf = append(w.buf, s...)
	w.buf = append(w.buf, 0)
}

func (w *cdrWriter) finish() []byte { return w.buf }

// cdrReader is cdrWriter's counterpart.
type cdrReader struct {
	buf []byte
	pos int // offset relative to the end of the encapsulation header
}

// newCDRReader validates the 4-byte encapsulation header and returns a
// reader positioned at the start of the data. ok is false if data is too
// short or its representation_identifier is not CDR_LE (0x0001) — this
// package always writes CDR_LE and does not attempt to decode big-endian
// or parameter-list encapsulations.
func newCDRReader(data []byte) (*cdrReader, bool) {
	if len(data) < 4 || data[1] != cdrLEHeader[1] {
		return nil, false
	}
	return &cdrReader{buf: data[4:]}, true
}

func (r *cdrReader) align(n int) {
	if pad := (n - r.pos%n) % n; pad > 0 {
		r.pos += pad
	}
}

func (r *cdrReader) rawBytes(n int) ([]byte, bool) {
	if n < 0 || r.pos+n > len(r.buf) {
		return nil, false
	}
	b := r.buf[r.pos : r.pos+n]
	r.pos += n
	return b, true
}

func (r *cdrReader) uint32() (uint32, bool) {
	r.align(4)
	b, ok := r.rawBytes(4)
	if !ok {
		return 0, false
	}
	return binary.LittleEndian.Uint32(b), true
}

// maxSeqLen bounds every sequence/string length this package will accept
// while decoding, so a corrupt or hostile "ros_discovery_info" sample can
// never trigger an oversized allocation.
const maxSeqLen = 1 << 20

func (r *cdrReader) str() (string, bool) {
	n, ok := r.uint32()
	if !ok || n == 0 || n > maxSeqLen {
		return "", false
	}
	b, ok := r.rawBytes(int(n))
	if !ok {
		return "", false
	}
	return string(b[:len(b)-1]), true // drop trailing NUL
}
