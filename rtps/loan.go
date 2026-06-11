// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package rtps

//fusa:req REQ-LOAN-001
//fusa:req REQ-LOAN-002
//fusa:req REQ-LOAN-003

import (
	"fmt"

	dds "github.com/SoundMatt/go-DDS"
	"github.com/SoundMatt/go-DDS/pool"
)

// loaningWriter wraps rtpsWriter with a BytePool for allocation-free loaned-sample
// publishing.
type loaningWriter struct {
	*rtpsWriter
	bp *pool.BytePool
}

// NewLoaningPublisher creates a LoaningPublisher for topic using the given QoS.
// bufSize is the maximum sample size the pool will pre-allocate; pass 0 for the
// pool default (4096 bytes).
func NewLoaningPublisher(p dds.Participant, topic string, qos dds.QoS, bufSize int) (dds.LoaningPublisher, error) {
	pub, err := p.NewPublisher(topic, qos)
	if err != nil {
		return nil, err
	}
	rw, ok := pub.(*rtpsWriter)
	if !ok {
		_ = pub.Close()
		return nil, fmt.Errorf("rtps: %w: participant is not an rtps participant", dds.ErrLoanBuffer)
	}
	return &loaningWriter{
		rtpsWriter: rw,
		bp:         pool.New(bufSize),
	}, nil
}

// Loan returns a pre-allocated byte slice of the requested size from the pool.
func (lw *loaningWriter) Loan(size int) ([]byte, error) {
	lw.mu.Lock()
	if lw.closed {
		lw.mu.Unlock()
		return nil, fmt.Errorf("rtps: %w", dds.ErrClosed)
	}
	lw.mu.Unlock()
	buf := lw.bp.Get()
	if cap(buf) < size {
		lw.bp.Put(buf)
		return nil, fmt.Errorf("rtps: %w: requested %d, pool cap %d", dds.ErrLoanBuffer, size, cap(buf))
	}
	return buf[:size], nil
}

// Commit publishes the loaned buffer and returns it to the pool.
func (lw *loaningWriter) Commit(buf []byte) error {
	err := lw.Write(buf)
	lw.bp.Put(buf)
	return err
}
