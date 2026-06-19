// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package rtps_test

//fusa:test REQ-PART-006
//fusa:test REQ-PUB-001

import (
	"context"
	"errors"
	"testing"

	dds "github.com/SoundMatt/go-DDS"
	"github.com/SoundMatt/go-DDS/rtps"
)

func TestParticipant_Domain(t *testing.T) {
	p, err := rtps.New(testDomain, rtps.WithNoMulticast())
	if err != nil {
		t.Skipf("rtps.New: %v", err)
	}
	defer p.Close()
	if p.Domain() != testDomain {
		t.Errorf("Domain() = %d, want %d", p.Domain(), testDomain)
	}
}

func TestWriter_WriteCtx(t *testing.T) {
	p, err := rtps.New(testDomain, rtps.WithNoMulticast())
	if err != nil {
		t.Skipf("rtps.New: %v", err)
	}
	defer p.Close()

	pub, err := p.NewPublisher("writectx/topic", dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
	defer pub.Close()

	// Happy path: a live context permits the write.
	if err := pub.WriteCtx(context.Background(), []byte("ok")); err != nil {
		t.Errorf("WriteCtx(live ctx): %v", err)
	}

	// Cancelled context: WriteCtx returns the context error before writing.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := pub.WriteCtx(ctx, []byte("nope")); !errors.Is(err, context.Canceled) {
		t.Errorf("WriteCtx(cancelled) = %v, want context.Canceled", err)
	}
}
