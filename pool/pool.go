// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Package pool provides allocation-efficient data structures for high-throughput
// DDS workloads on embedded and edge hardware.
//
// BytePool recycles fixed-capacity byte slices via sync.Pool, reducing GC
// pressure on fast publish paths that would otherwise allocate a new buffer
// per sample.
//
// SampleBuffer is a fixed-capacity ring buffer of dds.Sample values. It
// provides bounded, allocation-free queuing that can be used as a staging
// area between a subscriber channel and an application processing loop.
package pool

import (
	"sync"

	dds "github.com/SoundMatt/go-DDS"
)

// BytePool is a sync.Pool-backed pool of byte slices with a fixed capacity.
// Use Get to obtain a zero-length, pre-allocated buffer, write into it, and
// return it with Put when done. The pool prevents per-sample heap allocations
// on hot publish paths.
type BytePool struct {
	p    sync.Pool
	size int
}

// New returns a BytePool whose Get method returns slices with the given
// capacity. Callers may append up to size bytes without triggering a new
// allocation.
func New(size int) *BytePool {
	if size <= 0 {
		size = 4096
	}
	bp := &BytePool{size: size}
	bp.p = sync.Pool{
		New: func() any {
			buf := make([]byte, 0, size)
			return &buf
		},
	}
	return bp
}

// Get retrieves a zero-length, pre-allocated byte slice from the pool.
// The caller must call Put when done to return the buffer.
func (p *BytePool) Get() []byte {
	v := p.p.Get()
	ptr, ok := v.(*[]byte)
	if !ok || ptr == nil {
		return make([]byte, 0, p.size)
	}
	return (*ptr)[:0]
}

// Put returns buf to the pool. Buffers smaller than the pool's configured
// size are discarded so that the pool never accumulates undersized slices.
func (p *BytePool) Put(buf []byte) {
	if cap(buf) < p.size {
		return
	}
	b := buf[:0]
	p.p.Put(&b)
}

// SampleBuffer is a fixed-capacity concurrent ring buffer of dds.Sample
// values. It provides bounded, allocation-free queuing suitable for use as a
// staging area between a subscriber channel and an application processing
// loop running at a different rate.
type SampleBuffer struct {
	buf  []dds.Sample
	head int
	tail int
	len  int
	cap  int
	mu   sync.Mutex
}

// NewSampleBuffer returns a SampleBuffer with the given capacity. A capacity
// of ≤ 0 uses the default of 64.
func NewSampleBuffer(capacity int) *SampleBuffer {
	if capacity <= 0 {
		capacity = 64
	}
	return &SampleBuffer{
		buf: make([]dds.Sample, capacity),
		cap: capacity,
	}
}

// Push adds s to the ring buffer. Returns false if the buffer is full.
func (sb *SampleBuffer) Push(s dds.Sample) bool {
	sb.mu.Lock()
	defer sb.mu.Unlock()
	if sb.len == sb.cap {
		return false
	}
	sb.buf[sb.tail] = s
	sb.tail = (sb.tail + 1) % sb.cap
	sb.len++
	return true
}

// Pop removes and returns the oldest sample. Returns false if the buffer is
// empty.
func (sb *SampleBuffer) Pop() (dds.Sample, bool) {
	sb.mu.Lock()
	defer sb.mu.Unlock()
	if sb.len == 0 {
		return dds.Sample{}, false
	}
	s := sb.buf[sb.head]
	sb.buf[sb.head] = dds.Sample{} // release payload reference for GC
	sb.head = (sb.head + 1) % sb.cap
	sb.len--
	return s, true
}

// Len returns the number of samples currently held in the buffer.
func (sb *SampleBuffer) Len() int {
	sb.mu.Lock()
	defer sb.mu.Unlock()
	return sb.len
}

// Cap returns the buffer's maximum capacity.
func (sb *SampleBuffer) Cap() int { return sb.cap }
