// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Internal tests for unexported functions in the grpcbridge package.
// Kept separate from grpc_test.go (package grpcbridge_test) so that
// private symbols are accessible.

package grpcbridge

//fusa:test REQ-BRIDGE-006
//fusa:test REQ-BRIDGE-007

import (
	"context"
	"errors"
	"testing"
	"time"

	dds "github.com/SoundMatt/go-DDS"
	"github.com/SoundMatt/go-DDS/mock"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// ── checkAuth internal tests ──────────────────────────────────────────────────

// TestCheckAuth_NoMetadata covers the !ok branch in checkAuth where the
// incoming context carries no gRPC metadata (line 484-486).
func TestCheckAuth_NoMetadata(t *testing.T) {
	err := checkAuth(context.Background(), "any-token")
	if err == nil {
		t.Fatal("expected Unauthenticated error when context has no metadata")
	}
}

// TestCheckAuth_EmptyAuthHeader covers the len(vals)==0 branch where metadata
// is present but the "authorization" key is absent.
func TestCheckAuth_EmptyAuthHeader(t *testing.T) {
	ctx := metadata.NewIncomingContext(context.Background(), metadata.MD{})
	err := checkAuth(ctx, "secret")
	if err == nil {
		t.Fatal("expected Unauthenticated error when authorization header is missing")
	}
}

// TestCheckAuth_WrongToken covers the vals[0]!="Bearer <token>" branch.
func TestCheckAuth_WrongToken(t *testing.T) {
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs("authorization", "Bearer wrong"))
	err := checkAuth(ctx, "secret")
	if err == nil {
		t.Fatal("expected Unauthenticated error for wrong token")
	}
}

// TestCheckAuth_CorrectToken covers the success path (returns nil).
func TestCheckAuth_CorrectToken(t *testing.T) {
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs("authorization", "Bearer secret"))
	err := checkAuth(ctx, "secret")
	if err != nil {
		t.Fatalf("expected nil error for correct token, got %v", err)
	}
}

// ── _subscribeHandler internal tests ─────────────────────────────────────────

// failingRecvServerStream is a grpc.ServerStream whose RecvMsg always returns
// an error, exercising the early-return path in _subscribeHandler.
type failingRecvServerStream struct{}

func (f *failingRecvServerStream) SetHeader(metadata.MD) error  { return nil }
func (f *failingRecvServerStream) SendHeader(metadata.MD) error { return nil }
func (f *failingRecvServerStream) SetTrailer(metadata.MD)       {}
func (f *failingRecvServerStream) Context() context.Context     { return context.Background() }
func (f *failingRecvServerStream) SendMsg(m interface{}) error  { return nil }
func (f *failingRecvServerStream) RecvMsg(m interface{}) error  { return errors.New("recv failed") }

// TestSubscribeHandler_RecvMsgError covers the error-return path in
// _subscribeHandler where stream.RecvMsg fails (line 140-142).
// srv is nil because _subscribeHandler returns before calling srv.Subscribe.
func TestSubscribeHandler_RecvMsgError(t *testing.T) {
	err := _subscribeHandler(nil, &failingRecvServerStream{})
	if err == nil {
		t.Fatal("expected error from _subscribeHandler when RecvMsg fails")
	}
	if err.Error() != "recv failed" {
		t.Errorf("unexpected error: %v", err)
	}
}

// ── _publishHandler internal tests ───────────────────────────────────────────

// TestPublishHandler_DecodeError covers the dec(in) error path in
// _publishHandler (line 124-126).
func TestPublishHandler_DecodeError(t *testing.T) {
	wantErr := errors.New("decode failed")
	decFn := func(v interface{}) error { return wantErr }
	_, err := _publishHandler(nil, context.Background(), decFn, nil)
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected %v, got %v", wantErr, err)
	}
}

// TestPublishHandler_WithInterceptor covers the interceptor != nil branch in
// _publishHandler (line 131-135) where the interceptor calls the handler.
type minimalBridgeServer struct{}

func (m *minimalBridgeServer) Publish(_ context.Context, req *PublishRequest) (*PublishAck, error) {
	return &PublishAck{Count: 1}, nil
}
func (m *minimalBridgeServer) Subscribe(_ *SubscribeRequest, _ grpc.ServerStreamingServer[Sample]) error {
	return nil
}
func (m *minimalBridgeServer) StreamPublish(_ grpc.ClientStreamingServer[PublishRequest, PublishAck]) error {
	return nil
}

func TestPublishHandler_WithInterceptor(t *testing.T) {
	interceptorCalled := false
	interceptor := func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		interceptorCalled = true
		return handler(ctx, req)
	}
	decFn := func(v interface{}) error { return nil }
	_, err := _publishHandler(&minimalBridgeServer{}, context.Background(), decFn, interceptor)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !interceptorCalled {
		t.Fatal("expected interceptor to be called")
	}
}

// ── Bridge.Subscribe internal coverage ───────────────────────────────────────

// mockSubscribeStream implements grpc.ServerStreamingServer[Sample] with a
// configurable Send error. SetHeader, SendHeader, SetTrailer, RecvMsg, SendMsg
// are all no-ops.
type mockSubscribeStream struct {
	ctx     context.Context
	sendErr error
}

func (m *mockSubscribeStream) Send(_ *Sample) error          { return m.sendErr }
func (m *mockSubscribeStream) SetHeader(metadata.MD) error   { return nil }
func (m *mockSubscribeStream) SendHeader(metadata.MD) error  { return nil }
func (m *mockSubscribeStream) SetTrailer(metadata.MD)        {}
func (m *mockSubscribeStream) Context() context.Context      { return m.ctx }
func (m *mockSubscribeStream) SendMsg(msg interface{}) error { return nil }
func (m *mockSubscribeStream) RecvMsg(msg interface{}) error { return nil }

// TestBridgeSubscribe_ClosedChannelOK covers the `!ok` branch in
// Bridge.Subscribe where the DDS subscriber's channel closes (line 364-366).
// The test pre-caches a closed subscriber so the server detects the closed
// channel immediately without racing against the stream context.
func TestBridgeSubscribe_ClosedChannelOK(t *testing.T) {
	p, err := mock.New(dds.Domain(0))
	if err != nil {
		t.Fatalf("mock.New: %v", err)
	}
	defer p.Close()

	b := New(p, Options{})

	// Create a subscriber and close it before the Bridge.Subscribe call so the
	// channel is already closed when the select fires.
	sub, err := p.NewSubscriber("internal/closed-ok", dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewSubscriber: %v", err)
	}
	// Pre-populate the bridge's subscriber cache with the open subscriber.
	b.mu.Lock()
	b.subs["internal/closed-ok"] = sub
	b.mu.Unlock()

	// Close the subscriber channel, then give Bridge.Subscribe a live context so
	// ctx.Done() does not fire first.
	sub.Close()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	stream := &mockSubscribeStream{ctx: ctx}

	err = b.Subscribe(&SubscribeRequest{Topic: "internal/closed-ok"}, stream)
	if err != nil {
		t.Fatalf("expected nil from !ok path, got %v", err)
	}
}

// ── Bridge.StreamPublish internal coverage ────────────────────────────────────

// mockStreamPublishServer implements grpc.ClientStreamingServer[PublishRequest, PublishAck].
// recvFn is called on each Recv; sendAndCloseFn is called on SendAndClose.
type mockStreamPublishServer struct {
	ctx      context.Context
	recvFn   func() (*PublishRequest, error)
	closeErr error
}

func (m *mockStreamPublishServer) Recv() (*PublishRequest, error) { return m.recvFn() }
func (m *mockStreamPublishServer) SendAndClose(ack *PublishAck) error {
	return m.closeErr
}
func (m *mockStreamPublishServer) SetHeader(metadata.MD) error   { return nil }
func (m *mockStreamPublishServer) SendHeader(metadata.MD) error  { return nil }
func (m *mockStreamPublishServer) SetTrailer(metadata.MD)        {}
func (m *mockStreamPublishServer) Context() context.Context      { return m.ctx }
func (m *mockStreamPublishServer) SendMsg(msg interface{}) error { return nil }
func (m *mockStreamPublishServer) RecvMsg(msg interface{}) error { return nil }

// TestBridgeStreamPublish_NonEOFRecvError covers the non-EOF error return in
// Bridge.StreamPublish (line 418) where stream.Recv fails with a non-EOF error.
func TestBridgeStreamPublish_NonEOFRecvError(t *testing.T) {
	p, err := mock.New(dds.Domain(0))
	if err != nil {
		t.Fatalf("mock.New: %v", err)
	}
	defer p.Close()

	wantErr := errors.New("recv error")
	b := New(p, Options{})
	stream := &mockStreamPublishServer{
		ctx:    context.Background(),
		recvFn: func() (*PublishRequest, error) { return nil, wantErr },
	}
	err = b.StreamPublish(stream)
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected %v, got %v", wantErr, err)
	}
}

// TestBridgeStreamPublish_WriteError covers the pub.WriteCtx error path in
// Bridge.StreamPublish (line 427-429). MaxSampleSize:1 rejects the 2-byte
// payload, so WriteCtx returns an error.
func TestBridgeStreamPublish_WriteError(t *testing.T) {
	p, err := mock.New(dds.Domain(0))
	if err != nil {
		t.Fatalf("mock.New: %v", err)
	}
	defer p.Close()

	b := New(p, Options{QoS: dds.QoS{MaxSampleSize: 1}})
	called := false
	stream := &mockStreamPublishServer{
		ctx: context.Background(),
		recvFn: func() (*PublishRequest, error) {
			if !called {
				called = true
				// payload "xx" (2 bytes) exceeds MaxSampleSize:1 → WriteCtx fails
				return &PublishRequest{Topic: "sp/write-err", Payload: []byte("xx")}, nil
			}
			return nil, errors.New("should not reach here")
		},
	}
	err = b.StreamPublish(stream)
	if err == nil {
		t.Fatal("expected error from pub.WriteCtx when sample exceeds MaxSampleSize")
	}
}

// TestBridgeSubscribe_SendError covers the stream.Send error return in
// Bridge.Subscribe (line 384-386).
func TestBridgeSubscribe_SendError(t *testing.T) {
	p, err := mock.New(dds.Domain(0))
	if err != nil {
		t.Fatalf("mock.New: %v", err)
	}
	defer p.Close()

	b := New(p, Options{})

	// Create a subscriber and publisher on the same topic.
	sub, err := p.NewSubscriber("internal/send-err", dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewSubscriber: %v", err)
	}
	b.mu.Lock()
	b.subs["internal/send-err"] = sub
	b.mu.Unlock()

	pub, err := p.NewPublisher("internal/send-err", dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
	defer pub.Close()

	wantErr := errors.New("send failed")
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	stream := &mockSubscribeStream{ctx: ctx, sendErr: wantErr}

	// Publish a sample so the bridge's select case fires and calls stream.Send.
	if werr := pub.Write([]byte("trigger")); werr != nil {
		t.Fatalf("Write: %v", werr)
	}

	err = b.Subscribe(&SubscribeRequest{Topic: "internal/send-err"}, stream)
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected %v, got %v", wantErr, err)
	}
}
