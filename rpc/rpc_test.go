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

// failCodec always returns an error on Marshal/Unmarshal, used to cover
// codec-error paths in Requester.Request and Replier.Reply.
type failCodec[T any] struct{}

func (failCodec[T]) Marshal(_ T) ([]byte, error)   { return nil, errors.New("marshal failed") }
func (failCodec[T]) Unmarshal(_ []byte) (T, error) { var z T; return z, errors.New("unmarshal failed") }

// decodeFailCodec marshals successfully but fails on Unmarshal, used to cover
// the unmarshal-reply error path in Requester.Request.
type decodeFailCodec[T any] struct{ inner dds.Codec[T] }

func (c decodeFailCodec[T]) Marshal(v T) ([]byte, error) { return c.inner.Marshal(v) }
func (decodeFailCodec[T]) Unmarshal(_ []byte) (T, error) {
	var z T
	return z, errors.New("unmarshal failed")
}

// brokenSubParticipant wraps a real participant but always fails NewSubscriber,
// letting NewPublisher succeed so we can cover the second error path in NewRequester.
type brokenSubParticipant struct {
	dds.Participant
}

func (p *brokenSubParticipant) NewSubscriber(_ string, _ dds.QoS, _ ...dds.SubscriberOption) (dds.Subscriber, error) {
	return nil, errors.New("injected subscriber failure")
}

// brokenPubParticipant wraps a real participant but always fails NewPublisher,
// letting NewSubscriber succeed so we can cover the second error path in NewReplier.
type brokenPubParticipant struct {
	dds.Participant
}

func (p *brokenPubParticipant) NewPublisher(_ string, _ dds.QoS) (dds.Publisher, error) {
	return nil, errors.New("injected publisher failure")
}

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

// ── Error-path coverage ───────────────────────────────────────────────────────

func TestNewRequester_PublisherError(t *testing.T) {
	p := newPart(t)
	_ = p.Close() // closed → NewPublisher fails
	_, err := rpc.NewRequester[addReq, addRep](p, "rpc/pub-err",
		dds.JSONCodec[addReq]{}, dds.JSONCodec[addRep]{}, dds.DefaultQoS)
	if err == nil {
		t.Fatal("expected error when participant is closed")
	}
}

func TestNewRequester_SubscriberError(t *testing.T) {
	p := newPart(t)
	bp := &brokenSubParticipant{p}
	_, err := rpc.NewRequester[addReq, addRep](bp, "rpc/sub-err",
		dds.JSONCodec[addReq]{}, dds.JSONCodec[addRep]{}, dds.DefaultQoS)
	if err == nil {
		t.Fatal("expected error when subscriber creation fails")
	}
}

func TestNewReplier_SubscriberError(t *testing.T) {
	p := newPart(t)
	_ = p.Close() // closed → NewSubscriber fails
	_, err := rpc.NewReplier[addReq, addRep](p, "rpc/rsub-err",
		dds.JSONCodec[addReq]{}, dds.JSONCodec[addRep]{}, dds.DefaultQoS)
	if err == nil {
		t.Fatal("expected error when participant is closed")
	}
}

func TestNewReplier_PublisherError(t *testing.T) {
	p := newPart(t)
	bp := &brokenPubParticipant{p}
	_, err := rpc.NewReplier[addReq, addRep](bp, "rpc/rpub-err",
		dds.JSONCodec[addReq]{}, dds.JSONCodec[addRep]{}, dds.DefaultQoS)
	if err == nil {
		t.Fatal("expected error when publisher creation fails")
	}
}

func TestRequest_EncodeError(t *testing.T) {
	p := newPart(t)
	req, err := rpc.NewRequester[addReq, addRep](p, "rpc/enc-err",
		failCodec[addReq]{}, dds.JSONCodec[addRep]{}, dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewRequester: %v", err)
	}
	defer req.Close()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, reqErr := req.Request(ctx, addReq{A: 1, B: 2})
	if reqErr == nil {
		t.Fatal("expected error from marshal failure")
	}
}

func TestRequest_DoneCase(t *testing.T) {
	// Close the requester while a request is in flight (no replier) so
	// the r.done case in Request fires.
	p := newPart(t)
	req, err := rpc.NewRequester[addReq, addRep](p, "rpc/done-case",
		dds.JSONCodec[addReq]{}, dds.JSONCodec[addRep]{}, dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewRequester: %v", err)
	}

	got := make(chan error, 1)
	go func() {
		_, e := req.Request(context.Background(), addReq{A: 1, B: 2})
		got <- e
	}()
	time.Sleep(20 * time.Millisecond)
	_ = req.Close() // triggers r.done → Request returns dds.ErrClosed

	select {
	case e := <-got:
		if !errors.Is(e, dds.ErrClosed) {
			t.Errorf("expected ErrClosed, got %v", e)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Request did not return after Close")
	}
}

func TestReply_EncodeError(t *testing.T) {
	p := newPart(t)
	replier, err := rpc.NewReplier[addReq, addRep](p, "rpc/rep-enc-err",
		dds.JSONCodec[addReq]{}, failCodec[addRep]{}, dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewReplier: %v", err)
	}
	defer replier.Close()

	req := rpc.Request[addReq]{Value: addReq{A: 1, B: 2}}
	if repErr := replier.Reply(context.Background(), req, addRep{Sum: 3}); repErr == nil {
		t.Fatal("expected error from marshal failure")
	}
}

func TestRequest_UnmarshalReplyError(t *testing.T) {
	// decodeFailCodec encodes requests fine but fails to decode replies,
	// covering the unmarshal-reply error branch in Requester.Request.
	p := newPart(t)
	req, err := rpc.NewRequester[addReq, addRep](p, "rpc/unmarshal-rep",
		dds.JSONCodec[addReq]{}, decodeFailCodec[addRep]{inner: dds.JSONCodec[addRep]{}}, dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewRequester: %v", err)
	}
	replier, err := rpc.NewReplier[addReq, addRep](p, "rpc/unmarshal-rep",
		dds.JSONCodec[addReq]{}, dds.JSONCodec[addRep]{}, dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewReplier: %v", err)
	}
	defer replier.Close()
	defer req.Close()

	go func() {
		for r := range replier.Requests() {
			_ = replier.Reply(context.Background(), r, addRep{Sum: r.Value.A + r.Value.B})
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, reqErr := req.Request(ctx, addReq{A: 1, B: 2})
	if reqErr == nil {
		t.Fatal("expected unmarshal error from decodeFailCodec")
	}
}

func TestReply_WriteError(t *testing.T) {
	p := newPart(t)
	// MaxSampleSize: 1 forces WriteCtx to fail (reply payload is always > 1 byte).
	qos := dds.QoS{MaxSampleSize: 1}
	replier, replErr := rpc.NewReplier[addReq, addRep](p, "rpc/rep-write-err",
		dds.JSONCodec[addReq]{}, dds.JSONCodec[addRep]{}, qos)
	if replErr != nil {
		t.Fatalf("NewReplier: %v", replErr)
	}
	defer replier.Close()

	req := rpc.Request[addReq]{Value: addReq{A: 1, B: 2}}
	if repErr := replier.Reply(context.Background(), req, addRep{Sum: 3}); repErr == nil {
		t.Fatal("expected write error due to MaxSampleSize constraint")
	}
}

// TestRequester_Demux_ShortReply exercises the decodeRPC-error continue branch
// inside demux by publishing a short (<16 byte) payload directly to the reply topic.
func TestRequester_Demux_ShortReply(t *testing.T) {
	p := newPart(t)
	req, err := rpc.NewRequester[addReq, addRep](p, "rpc/demux-short",
		dds.JSONCodec[addReq]{}, dds.JSONCodec[addRep]{}, dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewRequester: %v", err)
	}
	defer req.Close()

	pub, err := p.NewPublisher("rpc/demux-short/reply", dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
	defer pub.Close()
	_ = pub.Write([]byte("short")) // <16 bytes → decodeRPC error → continue
	time.Sleep(20 * time.Millisecond)
}

// TestReplier_Pump_ShortRequest exercises the decodeRPC-error continue branch
// inside pump by publishing a short (<16 byte) payload directly to the request topic.
func TestReplier_Pump_ShortRequest(t *testing.T) {
	p := newPart(t)
	replier, err := rpc.NewReplier[addReq, addRep](p, "rpc/pump-short",
		dds.JSONCodec[addReq]{}, dds.JSONCodec[addRep]{}, dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewReplier: %v", err)
	}
	defer replier.Close()

	pub, err := p.NewPublisher("rpc/pump-short/request", dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
	defer pub.Close()
	_ = pub.Write([]byte("short")) // <16 bytes → decodeRPC error → continue
	time.Sleep(20 * time.Millisecond)
}

// TestReplier_Pump_InvalidJSON exercises the Unmarshal-error continue branch
// inside pump by publishing a valid 16-byte RPC header followed by invalid JSON.
func TestReplier_Pump_InvalidJSON(t *testing.T) {
	p := newPart(t)
	replier, err := rpc.NewReplier[addReq, addRep](p, "rpc/pump-invalid",
		dds.JSONCodec[addReq]{}, dds.JSONCodec[addRep]{}, dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewReplier: %v", err)
	}
	defer replier.Close()

	pub, err := p.NewPublisher("rpc/pump-invalid/request", dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
	defer pub.Close()

	var id [16]byte // zero correlation ID
	payload := append(id[:], []byte("{not valid json}")...)
	_ = pub.Write(payload) // valid header + bad JSON → Unmarshal error → continue
	time.Sleep(20 * time.Millisecond)
}
