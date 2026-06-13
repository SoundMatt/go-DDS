// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Internal tests for error paths that require constructor injection.

package auto

import (
	"errors"
	"testing"

	dds "github.com/SoundMatt/go-DDS"
	"github.com/SoundMatt/go-DDS/rtps"
)

var errForced = errors.New("forced failure")

func withShmemError(t *testing.T) {
	t.Helper()
	orig := newShmem
	newShmem = func(_ dds.Domain) (dds.Participant, error) { return nil, errForced }
	t.Cleanup(func() { newShmem = orig })
}

func withRTPSError(t *testing.T) {
	t.Helper()
	orig := newRTPS
	newRTPS = func(_ dds.Domain, _ ...rtps.Option) (dds.Participant, error) { return nil, errForced }
	t.Cleanup(func() { newRTPS = orig })
}

func TestNewParticipant_shmem_error(t *testing.T) {
	withShmemError(t)
	_, err := NewParticipant(dds.Domain(0), WithTransport(TransportShmem))
	if err == nil {
		t.Fatal("expected error when shmem.New fails")
	}
}

func TestNewParticipant_rtps_error(t *testing.T) {
	withRTPSError(t)
	_, err := NewParticipant(dds.Domain(0), WithTransport(TransportRTPS))
	if err == nil {
		t.Fatal("expected error when rtps.New fails")
	}
}

func TestNewParticipant_auto_fallback_success(t *testing.T) {
	withShmemError(t)
	// shmem fails; RTPS fallback should succeed normally.
	p, err := NewParticipant(dds.Domain(7))
	if err != nil {
		t.Fatalf("expected rtps fallback to succeed: %v", err)
	}
	defer p.Close()
}

func TestNewParticipant_auto_both_fail(t *testing.T) {
	withShmemError(t)
	withRTPSError(t)
	_, err := NewParticipant(dds.Domain(0))
	if err == nil {
		t.Fatal("expected error when both backends fail")
	}
}
