// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package record

import (
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
}

// FaultPublisher wraps a dds.Publisher and injects faults on Write according
// to the given FaultOptions. It satisfies the dds.Publisher interface.
type FaultPublisher struct {
	pub    dds.Publisher
	opts   FaultOptions
	rng    *rand.Rand
	mu     sync.Mutex
	closed bool
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
		time.Sleep(delay)
	}

	data := payload
	if corrupt {
		cp := make([]byte, len(payload))
		copy(cp, payload)
		cp[corruptIdx] ^= 0xFF
		data = cp
	}

	if err := f.pub.Write(data); err != nil {
		return err
	}
	if dup {
		return f.pub.Write(data)
	}
	return nil
}

// Close closes the underlying publisher.
func (f *FaultPublisher) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closed = true
	return f.pub.Close()
}
