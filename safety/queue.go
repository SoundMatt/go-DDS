// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package safety

import (
	"errors"
	"fmt"
	"sync"

	dds "github.com/SoundMatt/go-DDS"
)

// ErrQueueFull is returned by Enqueue when the DeterministicQueue has no
// remaining capacity.
var ErrQueueFull = errors.New("safety: queue full")

// DeterministicQueue is a bounded FIFO that decouples application writes from
// the underlying publisher. It contains the publisher's panic (converting it
// to an error on the Errors channel) and provides non-blocking back-pressure
// via ErrQueueFull.
//
// Typical use:
//
//	q := safety.NewDeterministicQueue(pub, 128).Start()
//	defer q.Stop()
//
//	if err := q.Enqueue(payload); err != nil {
//	    // handle back-pressure
//	}
type DeterministicQueue struct {
	pub    dds.Publisher
	ch     chan []byte
	done   chan struct{}
	once   sync.Once
	wg     sync.WaitGroup
	errors chan error
}

// NewDeterministicQueue creates a queue with the given channel depth that
// drains into pub. Call Start to begin delivery. A depth of ≤ 0 uses 64.
func NewDeterministicQueue(pub dds.Publisher, depth int) *DeterministicQueue {
	if depth <= 0 {
		depth = 64
	}
	return &DeterministicQueue{
		pub:    pub,
		ch:     make(chan []byte, depth),
		done:   make(chan struct{}),
		errors: make(chan error, 32),
	}
}

// Start launches the drain goroutine. Returns q for chaining.
func (q *DeterministicQueue) Start() *DeterministicQueue {
	q.wg.Add(1)
	go q.drain()
	return q
}

// Enqueue adds payload to the queue without blocking. Returns ErrQueueFull if
// the queue is at capacity.
func (q *DeterministicQueue) Enqueue(payload []byte) error {
	select {
	case q.ch <- payload:
		return nil
	default:
		return ErrQueueFull
	}
}

// Stop signals the drain goroutine to exit and waits for it to finish. Safe
// to call more than once.
func (q *DeterministicQueue) Stop() {
	q.once.Do(func() { close(q.done) })
	q.wg.Wait()
}

// Errors returns a channel that receives errors from the drain goroutine,
// including publisher errors and panics wrapped as errors.
func (q *DeterministicQueue) Errors() <-chan error { return q.errors }

func (q *DeterministicQueue) drain() {
	defer q.wg.Done()
	for {
		select {
		case payload, ok := <-q.ch:
			if !ok {
				return
			}
			q.safeWrite(payload)
		case <-q.done:
			return
		}
	}
}

// safeWrite writes to the publisher, catching any panic and routing it to the
// errors channel.
func (q *DeterministicQueue) safeWrite(payload []byte) {
	defer func() {
		if r := recover(); r != nil {
			select {
			case q.errors <- fmt.Errorf("safety: publisher panic: %v", r):
			default:
			}
		}
	}()
	if err := q.pub.Write(payload); err != nil {
		select {
		case q.errors <- err:
		default:
		}
	}
}
