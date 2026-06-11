// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package mock_test

//fusa:test REQ-LOAN-001
//fusa:test REQ-LOAN-002
//fusa:test REQ-LOAN-003

import (
	"errors"
	"testing"
	"time"

	dds "github.com/SoundMatt/go-DDS"
	"github.com/SoundMatt/go-DDS/mock"
)

func TestLoaningPublisher_roundtrip(t *testing.T) {
	p, _ := mock.New(dds.Domain(0))
	defer p.Close()

	sub, err := p.NewSubscriber("loan/test", dds.DefaultQoS)
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Close()

	lp, err := mock.NewLoaningPublisher(p, "loan/test", dds.DefaultQoS, 256)
	if err != nil {
		t.Fatal(err)
	}
	defer lp.Close()

	buf, err := lp.Loan(10)
	if err != nil {
		t.Fatalf("Loan: %v", err)
	}
	copy(buf, "helloworld")

	if err := lp.Commit(buf); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	select {
	case s := <-sub.C():
		if string(s.Payload) != "helloworld" {
			t.Fatalf("got %q want %q", s.Payload, "helloworld")
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for loaned sample")
	}
}

func TestLoaningPublisher_size_exceeded(t *testing.T) {
	p, _ := mock.New(dds.Domain(0))
	defer p.Close()

	lp, err := mock.NewLoaningPublisher(p, "loan/test2", dds.DefaultQoS, 64)
	if err != nil {
		t.Fatal(err)
	}
	defer lp.Close()

	_, err = lp.Loan(1024)
	if !errors.Is(err, dds.ErrLoanBuffer) {
		t.Fatalf("expected ErrLoanBuffer, got %v", err)
	}
}

func TestLoaningPublisher_closed(t *testing.T) {
	p, _ := mock.New(dds.Domain(0))
	defer p.Close()

	lp, err := mock.NewLoaningPublisher(p, "loan/test3", dds.DefaultQoS, 256)
	if err != nil {
		t.Fatal(err)
	}
	_ = lp.Close()

	_, err = lp.Loan(10)
	if !errors.Is(err, dds.ErrClosed) {
		t.Fatalf("expected ErrClosed, got %v", err)
	}
}
