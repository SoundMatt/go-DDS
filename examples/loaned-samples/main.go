// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Command loaned-samples demonstrates zero-copy publishing with LoaningPublisher.
//
// LoaningPublisher.Loan returns a buffer owned by the pool. The caller fills it
// and calls Commit to publish and recycle it. No allocation occurs on the
// publish hot path once the pool is warm.
//
// Run:
//
//	go run ./examples/loaned-samples
package main

import (
	"fmt"
	"log"
	"time"

	dds "github.com/SoundMatt/go-DDS"
	"github.com/SoundMatt/go-DDS/mock"
)

func main() {
	p, err := mock.New(dds.Domain(0))
	if err != nil {
		log.Fatalf("mock.New: %v", err)
	}
	defer func() { _ = p.Close() }()

	// Subscribe before publishing so we receive every sample.
	sub, err := p.NewSubscriber("sensors/imu", dds.DefaultQoS)
	if err != nil {
		log.Fatalf("NewSubscriber: %v", err)
	}
	defer func() { _ = sub.Close() }()

	// LoaningPublisher with a 128-byte pool buffer.
	lp, err := mock.NewLoaningPublisher(p, "sensors/imu", dds.DefaultQoS, 128)
	if err != nil {
		log.Fatalf("NewLoaningPublisher: %v", err)
	}
	defer func() { _ = lp.Close() }()

	for seq := 1; seq <= 5; seq++ {
		// Loan a buffer from the pool — zero allocation on the hot path.
		buf, err := lp.Loan(64)
		if err != nil {
			log.Fatalf("Loan: %v", err)
		}

		n := copy(buf, fmt.Sprintf(`{"seq":%d,"ax":0.01,"ay":-0.02,"az":9.81}`, seq))
		buf = buf[:n]

		// Commit publishes the filled slice and returns it to the pool.
		if err := lp.Commit(buf); err != nil {
			log.Fatalf("Commit: %v", err)
		}

		select {
		case s := <-sub.C():
			log.Printf("received: %s", s.Payload)
		case <-time.After(time.Second):
			log.Fatalf("timeout waiting for sample %d", seq)
		}
	}
}
