// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package rpc_test

//fusa:test REQ-RPC-001
//fusa:test REQ-RPC-002
//fusa:test REQ-RPC-003
//fusa:test REQ-RPC-004
//fusa:test REQ-RPC-005
//fusa:test REQ-RPC-006

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	dds "github.com/SoundMatt/go-DDS"
	"github.com/SoundMatt/go-DDS/mock"
	"github.com/SoundMatt/go-DDS/rpc"
)

type addReq struct{ A, B int }
type addRep struct{ Sum int }

func newPart(t *testing.T) dds.Participant {
	t.Helper()
	p, err := mock.New(0, mock.IsolatedBroker())
	if err != nil {
		t.Fatalf("mock.New: %v", err)
	}
	t.Cleanup(func() { _ = p.Close() })
	return p
}

func newRPC(t *testing.T, topic string) (*rpc.Requester[addReq, addRep], *rpc.Replier[addReq, addRep]) {
	t.Helper()
	p := newPart(t)
	req, err := rpc.NewRequester[addReq, addRep](p, topic, dds.JSONCodec[addReq]{}, dds.JSONCodec[addRep]{}, dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewRequester: %v", err)
	}
	rep, err := rpc.NewReplier[addReq, addRep](p, topic, dds.JSONCodec[addReq]{}, dds.JSONCodec[addRep]{}, dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewReplier: %v", err)
	}
	t.Cleanup(func() {
		_ = req.Close()
		_ = rep.Close()
	})
	return req, rep
}

func TestRPC_BasicRequestReply(t *testing.T) {
	requester, replier := newRPC(t, "rpc/add")

	go func() {
		for r := range replier.Requests() {
			_ = replier.Reply(context.Background(), r, addRep{Sum: r.Value.A + r.Value.B})
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	got, err := requester.Request(ctx, addReq{A: 3, B: 4})
	if err != nil {
		t.Fatalf("Request: %v", err)
	}
	if got.Sum != 7 {
		t.Errorf("Sum: got %d, want 7", got.Sum)
	}
}

func TestRPC_MultipleInFlight(t *testing.T) {
	requester, replier := newRPC(t, "rpc/multi")

	go func() {
		for r := range replier.Requests() {
			go func() {
				_ = replier.Reply(context.Background(), r, addRep{Sum: r.Value.A + r.Value.B})
			}()
		}
	}()

	const n = 10
	type result struct {
		sum int
		err error
	}
	results := make(chan result, n)

	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(a int) {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			got, err := requester.Request(ctx, addReq{A: a, B: 1})
			results <- result{sum: got.Sum, err: err}
		}(i)
	}
	wg.Wait()
	close(results)

	for r := range results {
		if r.err != nil {
			t.Errorf("Request error: %v", r.err)
		}
	}
}

func TestRPC_ContextTimeout(t *testing.T) {
	requester, replier := newRPC(t, "rpc/timeout")
	_ = replier

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	rep, err := requester.Request(ctx, addReq{A: 1, B: 2})
	_ = rep
	if err == nil {
		t.Fatal("expected error on timeout")
	}
	if !errors.Is(err, rpc.ErrNoReply) {
		t.Errorf("expected ErrNoReply, got %v", err)
	}
}

func TestRPC_ContextCancelBeforeSend(t *testing.T) {
	requester, replier := newRPC(t, "rpc/ctxcancel")
	_ = replier

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already cancelled

	rep, err := requester.Request(ctx, addReq{A: 0, B: 0})
	_ = rep
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

func TestRPC_CloseRequester(t *testing.T) {
	p := newPart(t)
	requester, err := rpc.NewRequester[addReq, addRep](p, "rpc/closereq",
		dds.JSONCodec[addReq]{}, dds.JSONCodec[addRep]{}, dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewRequester: %v", err)
	}

	if err := requester.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
	if err := requester.Close(); err != nil {
		t.Errorf("Close (idempotent): %v", err)
	}
}

func TestRPC_CloseReplier(t *testing.T) {
	p := newPart(t)
	replier, err := rpc.NewReplier[addReq, addRep](p, "rpc/closerep",
		dds.JSONCodec[addReq]{}, dds.JSONCodec[addRep]{}, dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewReplier: %v", err)
	}

	if err := replier.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
	if err := replier.Close(); err != nil {
		t.Errorf("Close (idempotent): %v", err)
	}

	// Requests channel must be closed after Close.
	select {
	case _, ok := <-replier.Requests():
		if ok {
			t.Error("requests channel should be closed after replier Close")
		}
	case <-time.After(500 * time.Millisecond):
		t.Error("requests channel not closed after replier Close")
	}
}

func TestRPC_CorrelationIsolation(t *testing.T) {
	p := newPart(t)

	req1, err := rpc.NewRequester[addReq, addRep](p, "rpc/iso",
		dds.JSONCodec[addReq]{}, dds.JSONCodec[addRep]{}, dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewRequester[1]: %v", err)
	}
	defer req1.Close()

	req2, err := rpc.NewRequester[addReq, addRep](p, "rpc/iso",
		dds.JSONCodec[addReq]{}, dds.JSONCodec[addRep]{}, dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewRequester[2]: %v", err)
	}
	defer req2.Close()

	replier, err := rpc.NewReplier[addReq, addRep](p, "rpc/iso",
		dds.JSONCodec[addReq]{}, dds.JSONCodec[addRep]{}, dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewReplier: %v", err)
	}
	defer replier.Close()

	go func() {
		for r := range replier.Requests() {
			go func() {
				_ = replier.Reply(context.Background(), r, addRep{Sum: r.Value.A + r.Value.B})
			}()
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	var wg sync.WaitGroup
	for i, req := range []*rpc.Requester[addReq, addRep]{req1, req2} {
		wg.Add(1)
		go func(idx int, r *rpc.Requester[addReq, addRep]) {
			defer wg.Done()
			got, err := r.Request(ctx, addReq{A: idx * 10, B: 1})
			if err != nil {
				t.Errorf("req%d.Request: %v", idx, err)
				return
			}
			want := idx*10 + 1
			if got.Sum != want {
				t.Errorf("req%d: got Sum=%d, want %d", idx, got.Sum, want)
			}
		}(i, req)
	}
	wg.Wait()
}
