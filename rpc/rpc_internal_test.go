// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Internal tests for unexported paths in the rpc package.
// Kept in package rpc so that rl.sub and r.sub are accessible.

package rpc

//fusa:test REQ-RPC-003
//fusa:test REQ-RPC-004

import (
	"testing"
	"time"

	dds "github.com/SoundMatt/go-DDS"
	"github.com/SoundMatt/go-DDS/mock"
)

func newInternalPart(t *testing.T) dds.Participant {
	t.Helper()
	p, err := mock.New(0, mock.IsolatedBroker())
	if err != nil {
		t.Fatalf("mock.New: %v", err)
	}
	t.Cleanup(func() { _ = p.Close() })
	return p
}

// bytesCodec is a minimal Codec[[]byte] for internal tests.
type bytesCodec struct{}

func (bytesCodec) Marshal(v []byte) ([]byte, error)   { return v, nil }
func (bytesCodec) Unmarshal(data []byte) ([]byte, error) { return data, nil }

// TestReplier_Pump_SubClosed covers the !ok branch in pump (rpc.go:238-240)
// where rl.sub's channel is closed while the pump goroutine is running.
// We access rl.sub directly (internal test) and close it before rl.done fires.
func TestReplier_Pump_SubClosed(t *testing.T) {
	p := newInternalPart(t)
	replier, err := NewReplier[[]byte, []byte](p, "rpc/internal/sub-closed",
		bytesCodec{}, bytesCodec{}, dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewReplier: %v", err)
	}

	// Close the replier's internal subscriber directly so the pump sees !ok.
	replier.sub.Close()
	time.Sleep(30 * time.Millisecond)

	// replier.Close() should return quickly since the pump already exited.
	if err := replier.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

// TestRequester_Demux_SubClosed covers the !ok branch in demux (rpc.go:117-119)
// where r.sub's channel is closed while the demux goroutine is running.
func TestRequester_Demux_SubClosed(t *testing.T) {
	p := newInternalPart(t)
	requester, err := NewRequester[[]byte, []byte](p, "rpc/internal/demux-closed",
		bytesCodec{}, bytesCodec{}, dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewRequester: %v", err)
	}

	requester.sub.Close()
	time.Sleep(30 * time.Millisecond)

	if err := requester.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}
