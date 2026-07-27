// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package record

//fusa:req REQ-REC-004
//fusa:req REQ-REC-005
//fusa:req REQ-REC-006
//fusa:req REQ-REC-007
//fusa:req REQ-REC-008
//fusa:req REQ-REC-009

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"time"

	dds "github.com/SoundMatt/go-DDS"
)

// FaultOptions configures the fault injection behaviour of a FaultPublisher.
// All probability fields are in [0.0, 1.0]; values outside that range are
// clamped at call time.
type FaultOptions struct {
	// LossRate is the probability that a Write call is silently dropped.
	LossRate float64
	// DelayMin and DelayMax bound the random additional latency added before
	// forwarding a sample. DelayMin == DelayMax imposes a fixed delay.
	// Both zero means no added latency.
	DelayMin time.Duration
	DelayMax time.Duration
	// CorruptRate is the probability that one byte of the payload is bit-flipped
	// before forwarding.
	CorruptRate float64
	// DuplicateRate is the probability that a sample is forwarded twice.
	DuplicateRate float64
	// ReorderWindow, when > 1, buffers up to ReorderWindow samples and emits
	// them in randomised order when the window fills, simulating out-of-order
	// delivery. Samples buffered at Close are flushed in shuffled order.
	ReorderWindow int
}

// FaultPublisher wraps a dds.Publisher and injects faults on Write according
// to the given FaultOptions. It satisfies the dds.Publisher interface.
type FaultPublisher struct {
	pub    dds.Publisher
	opts   FaultOptions
	rng    *rand.Rand
	mu     sync.Mutex
	closed bool
	window [][]byte // reorder buffer; non-nil only when ReorderWindow > 1
}

// NewFaultPublisher wraps pub with fault injection configured by opts.
// seed is passed to the internal PRNG; pass 0 for a time-derived seed.
func NewFaultPublisher(pub dds.Publisher, opts FaultOptions, seed int64) *FaultPublisher {
	if seed == 0 {
		seed = time.Now().UnixNano()
	}
	return &FaultPublisher{
		pub:  pub,
		opts: opts,
		rng:  rand.New(rand.NewSource(seed)), //nolint:gosec // deterministic fault injection; non-cryptographic use
	}
}

// Write applies configured faults and then forwards to the underlying publisher.
// The call may block if DelayMin > 0.
func (f *FaultPublisher) Write(payload []byte) error {
	return f.write(context.Background(), payload)
}

// WriteCtx applies configured faults and forwards to the underlying publisher,
// honouring ctx cancellation during any configured delay.
func (f *FaultPublisher) WriteCtx(ctx context.Context, payload []byte) error {
	return f.write(ctx, payload)
}

func (f *FaultPublisher) write(ctx context.Context, payload []byte) error {
	f.mu.Lock()
	if f.closed {
		f.mu.Unlock()
		return fmt.Errorf("record: %w", dds.ErrClosed)
	}

	drop := f.opts.LossRate > 0 && f.rng.Float64() < f.opts.LossRate
	dup := f.opts.DuplicateRate > 0 && f.rng.Float64() < f.opts.DuplicateRate
	corrupt := f.opts.CorruptRate > 0 && len(payload) > 0 && f.rng.Float64() < f.opts.CorruptRate

	var delay time.Duration
	if span := f.opts.DelayMax - f.opts.DelayMin; span > 0 {
		delay = f.opts.DelayMin + time.Duration(f.rng.Int63n(int64(span)))
	} else if f.opts.DelayMin > 0 {
		delay = f.opts.DelayMin
	}

	var corruptIdx int
	if corrupt {
		corruptIdx = f.rng.Intn(len(payload))
	}
	f.mu.Unlock()

	if drop {
		return nil
	}
	if delay > 0 {
		select {
		case <-time.After(delay):
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	data := payload
	if corrupt {
		cp := make([]byte, len(payload))
		copy(cp, payload)
		cp[corruptIdx] ^= 0xFF
		data = cp
	}

	// Reorder: buffer and emit when the window fills.
	if f.opts.ReorderWindow > 1 {
		cp := make([]byte, len(data))
		copy(cp, data)
		f.mu.Lock()
		f.window = append(f.window, cp)
		if len(f.window) < f.opts.ReorderWindow {
			f.mu.Unlock()
			return nil
		}
		window := f.window
		f.window = nil
		f.rng.Shuffle(len(window), func(i, j int) { window[i], window[j] = window[j], window[i] })
		f.mu.Unlock()
		return f.emitWindow(window)
	}

	if err := f.pub.Write(data); err != nil {
		return err
	}
	if dup {
		return f.pub.Write(data)
	}
	return nil
}

func (f *FaultPublisher) emitWindow(window [][]byte) error {
	for _, w := range window {
		if err := f.pub.Write(w); err != nil {
			return err
		}
	}
	return nil
}

// Close flushes any buffered reorder window, then closes the underlying publisher.
func (f *FaultPublisher) Close() error {
	f.mu.Lock()
	f.closed = true
	window := f.window
	f.window = nil
	if len(window) > 1 {
		f.rng.Shuffle(len(window), func(i, j int) { window[i], window[j] = window[j], window[i] })
	}
	f.mu.Unlock()

	_ = f.emitWindow(window)
	return f.pub.Close()
}
