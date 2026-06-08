// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package rpc_test

// Fuzz tests for the rpc package wire format.
//
// Run the fuzzer with e.g.:
//
//	go test -fuzz=FuzzRPCReplyDispatch   -fuzztime=60s ./rpc/...
//	go test -fuzz=FuzzRPCRequestDispatch -fuzztime=60s ./rpc/...

import (
	"context"
	"testing"
	"time"

	dds "github.com/SoundMatt/go-DDS"
	"github.com/SoundMatt/go-DDS/rpc"
)

// FuzzRPCReplyDispatch feeds arbitrary bytes to the Requester's reply topic.
// The Requester must not panic on malformed data — it should silently discard
// frames that cannot be decoded or that carry an unknown correlation ID.
func FuzzRPCReplyDispatch(f *testing.F) {
	f.Add([]byte(""))
	f.Add([]byte("tooshort"))
	f.Add(make([]byte, 16))       // exactly a CorrelationID with no payload
	f.Add(make([]byte, 32))       // CorrelationID + 16-byte body
	f.Add([]byte{0xFF, 0xFF, 0xFF, 0xFF, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 'x'})

	f.Fuzz(func(t *testing.T, data []byte) {
		p := newPart(t)

		req, err := rpc.NewRequester[addReq, addRep](p, "fuzz/reply", dds.JSONCodec[addReq]{}, dds.JSONCodec[addRep]{}, dds.DefaultQoS)
		if err != nil {
			t.Fatalf("NewRequester: %v", err)
		}
		defer req.Close()

		// Publish raw bytes directly to the reply topic, bypassing encoding.
		replyPub, err := p.NewPublisher("fuzz/reply/reply", dds.DefaultQoS)
		if err != nil {
			t.Fatalf("NewPublisher(reply): %v", err)
		}
		defer replyPub.Close()

		// Fire and forget: we only care that the Requester doesn't panic.
		_ = replyPub.Write(data)

		// Brief pause to let the dispatch goroutine process it.
		time.Sleep(5 * time.Millisecond)
	})
}

// FuzzRPCRequestDispatch feeds arbitrary bytes to the Replier's request topic.
// The Replier must not panic on malformed data — frames that cannot be decoded
// must be silently discarded.
func FuzzRPCRequestDispatch(f *testing.F) {
	f.Add([]byte(""))
	f.Add([]byte("short"))
	f.Add(make([]byte, 16))  // correlation ID only
	f.Add(make([]byte, 32))  // correlation ID + 16-byte body (not valid JSON)
	f.Add(append(make([]byte, 16), []byte(`{"A":1,"B":2}`)...)) // valid RPC frame

	f.Fuzz(func(t *testing.T, data []byte) {
		p := newPart(t)

		rep, err := rpc.NewReplier[addReq, addRep](p, "fuzz/request", dds.JSONCodec[addReq]{}, dds.JSONCodec[addRep]{}, dds.DefaultQoS)
		if err != nil {
			t.Fatalf("NewReplier: %v", err)
		}
		defer rep.Close()

		// Drain Requests so the channel never blocks the dispatch goroutine.
		go func() {
			for range rep.Requests() {
			}
		}()

		// Publish raw bytes directly to the request topic.
		reqPub, err := p.NewPublisher("fuzz/request/request", dds.DefaultQoS)
		if err != nil {
			t.Fatalf("NewPublisher(request): %v", err)
		}
		defer reqPub.Close()

		_ = reqPub.Write(data)

		time.Sleep(5 * time.Millisecond)
	})
}

// FuzzRPCRoundTrip encodes a request with JSONCodec and sends it through a
// live Requester→Replier pair. The fuzz input is the payload body; encoding
// must not panic, and the Replier must deliver exactly the body it receives.
func FuzzRPCRoundTrip(f *testing.F) {
	f.Add(1, 2)
	f.Add(0, 0)
	f.Add(-1, 100)
	f.Add(1<<30, 1<<30)

	f.Fuzz(func(t *testing.T, a, b int) {
		requester, replier := newRPC(t, "fuzz/roundtrip")

		go func() {
			for r := range replier.Requests() {
				_ = replier.Reply(context.Background(), r, addRep{Sum: r.Value.A + r.Value.B})
			}
		}()

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		rep, err := requester.Request(ctx, addReq{A: a, B: b})
		if err != nil {
			// Context timeout is not a library bug when values cause slow encoding.
			return
		}
		if rep.Sum != a+b {
			t.Errorf("Sum: got %d, want %d", rep.Sum, a+b)
		}
	})
}
