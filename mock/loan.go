// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package mock

//fusa:req REQ-LOAN-001
//fusa:req REQ-LOAN-002
//fusa:req REQ-LOAN-003

import (
	"fmt"

	dds "github.com/SoundMatt/go-DDS"
	"github.com/SoundMatt/go-DDS/pool"
)

// loaningPublisher wraps a publisher and backs it with a BytePool to provide
// allocation-free loaned-sample publishing.
type loaningPublisher struct {
	*publisher
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
	mpub, ok := pub.(*publisher)
	if !ok {
		_ = pub.Close()
		return nil, fmt.Errorf("mock: %w: participant is not a mock participant", dds.ErrLoanBuffer)
	}
	return &loaningPublisher{
		publisher: mpub,
		bp:        pool.New(bufSize),
	}, nil
}

// Loan returns a pre-allocated byte slice of the requested size from the pool.
func (lp *loaningPublisher) Loan(size int) ([]byte, error) {
	lp.mu.Lock()
	if lp.closed {
		lp.mu.Unlock()
		return nil, fmt.Errorf("mock: %w", dds.ErrClosed)
	}
	lp.mu.Unlock()
	buf := lp.bp.Get()
	if cap(buf) < size {
		lp.bp.Put(buf)
		return nil, fmt.Errorf("mock: %w: requested %d, pool cap %d", dds.ErrLoanBuffer, size, cap(buf))
	}
	return buf[:size], nil
}

// Commit publishes the loaned buffer and returns it to the pool.
func (lp *loaningPublisher) Commit(buf []byte) error {
	err := lp.Write(buf)
	lp.bp.Put(buf)
	return err
}
