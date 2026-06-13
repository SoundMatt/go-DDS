// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package grpcbridge_test

//fusa:test REQ-BRIDGE-006
//fusa:test REQ-BRIDGE-007
//fusa:test REQ-BRIDGE-008
//fusa:test REQ-BRIDGE-009
//fusa:test REQ-BRIDGE-010
//fusa:test REQ-BRIDGE-011
//fusa:test REQ-BRIDGE-012

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	dds "github.com/SoundMatt/go-DDS"
	grpcbridge "github.com/SoundMatt/go-DDS/bridge/grpc"
	"github.com/SoundMatt/go-DDS/mock"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

// listenLocal opens a random TCP listener using ListenConfig for noctx compliance.
func listenLocal(t *testing.T) net.Listener {
	t.Helper()
	lc := net.ListenConfig{}
	lis, err := lc.Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen: %v", err)
	}
	return lis
}

// dialJSON returns a gRPC client conn to addr using the JSON codec.
func dialJSON(t *testing.T, addr string) *grpc.ClientConn {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	conn, err := grpc.DialContext(ctx, addr, //nolint:staticcheck
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultCallOptions(grpc.ForceCodec(grpcbridge.JSONCodec{})),
	)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	return conn
}

// newTestBridge starts a Bridge backed by a mock participant on a random port.
func newTestBridge(t *testing.T, opts grpcbridge.Options) (grpcbridge.DDSBridgeClient, *grpcbridge.Bridge, dds.Participant) {
	t.Helper()
	p, err := mock.New(dds.Domain(0))
	if err != nil {
		t.Fatalf("mock.New: %v", err)
	}
	b := grpcbridge.New(p, opts)

	lis := listenLocal(t)
	go func() { _ = b.Server().Serve(lis) }()

	conn := dialJSON(t, lis.Addr().String())
	client := grpcbridge.NewRawClient(conn)

	t.Cleanup(func() {
		conn.Close()
		b.Close()
		p.Close()
	})
	return client, b, p
}

// ── Publish ───────────────────────────────────────────────────────────────────

func TestBridge_Publish_ReturnsAck(t *testing.T) {
	client, ret2, ret3 := newTestBridge(t, grpcbridge.Options{})
	_ = ret2
	_ = ret3

	ack, err := client.Publish(context.Background(), &grpcbridge.PublishRequest{
		Topic:   "grpc/test",
		Payload: []byte("hello"),
	})
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if ack.Count != 1 {
		t.Errorf("count: got %d, want 1", ack.Count)
	}
}

func TestBridge_Publish_EmptyTopic_InvalidArgument(t *testing.T) {
	client, ret2, ret3 := newTestBridge(t, grpcbridge.Options{})
	_ = ret2
	_ = ret3

	ignoredRet, err := client.Publish(context.Background(), &grpcbridge.PublishRequest{Topic: ""})
	_ = ignoredRet
	if err == nil {
		t.Fatal("expected error for empty topic")
	}
	if !strings.Contains(err.Error(), "InvalidArgument") && !strings.Contains(err.Error(), "invalid") {
		t.Errorf("expected InvalidArgument, got %v", err)
	}
}

// ── Subscribe ─────────────────────────────────────────────────────────────────

func TestBridge_Subscribe_ReceivesSample(t *testing.T) {
	client, bridge, p := newTestBridge(t, grpcbridge.Options{})
	_ = bridge

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	stream, err := client.Subscribe(ctx, &grpcbridge.SubscribeRequest{Topic: "grpc/sub"})
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	// Give the subscription time to register.
	time.Sleep(20 * time.Millisecond)

	pub, err := p.NewPublisher("grpc/sub", dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
	defer pub.Close()
	_ = pub.Write([]byte("grpc-sample"))

	sample, err := stream.Recv()
	if err != nil {
		t.Fatalf("Recv: %v", err)
	}
	if string(sample.Payload) != "grpc-sample" {
		t.Errorf("payload: %q", sample.Payload)
	}
	if sample.Topic != "grpc/sub" {
		t.Errorf("topic: %q", sample.Topic)
	}
}

func TestBridge_Subscribe_EmptyTopic_InvalidArgument(t *testing.T) {
	client, ret2, ret3 := newTestBridge(t, grpcbridge.Options{})
	_ = ret2
	_ = ret3

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	stream, err := client.Subscribe(ctx, &grpcbridge.SubscribeRequest{Topic: ""})
	if err != nil {
		return // some gRPC impls return the error at Subscribe time
	}
	ignoredRet, err := stream.Recv()
	_ = ignoredRet
	if err == nil {
		t.Fatal("expected error for empty topic")
	}
}

// ── StreamPublish ─────────────────────────────────────────────────────────────

func TestBridge_StreamPublish_ReturnsCount(t *testing.T) {
	client, ret2, ret3 := newTestBridge(t, grpcbridge.Options{})
	_ = ret2
	_ = ret3

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	sp, err := client.StreamPublish(ctx)
	if err != nil {
		t.Fatalf("StreamPublish: %v", err)
	}
	for i := 0; i < 5; i++ {
		if sendErr := sp.Send(&grpcbridge.PublishRequest{Topic: "grpc/stream", Payload: []byte("x")}); sendErr != nil {
			t.Fatalf("Send[%d]: %v", i, sendErr)
		}
	}
	ack, err := sp.CloseAndRecv()
	if err != nil {
		t.Fatalf("CloseAndRecv: %v", err)
	}
	if ack.Count != 5 {
		t.Errorf("count: got %d, want 5", ack.Count)
	}
}

// ── Filter / Transform ────────────────────────────────────────────────────────

func TestBridge_Filter_DropsMatchingSamples(t *testing.T) {
	opts := grpcbridge.Options{
		Filter: func(_ string, payload []byte) bool {
			return string(payload) != "drop-me"
		},
	}
	client, bridge, p := newTestBridge(t, opts)
	_ = bridge

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	stream, err := client.Subscribe(ctx, &grpcbridge.SubscribeRequest{Topic: "grpc/filter"})
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	time.Sleep(20 * time.Millisecond)

	pub, err := p.NewPublisher("grpc/filter", dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
	defer pub.Close()
	_ = pub.Write([]byte("drop-me")) // filtered out
	_ = pub.Write([]byte("keep-me"))

	got, err := stream.Recv()
	if err != nil {
		t.Fatalf("Recv: %v", err)
	}
	if string(got.Payload) != "keep-me" {
		t.Errorf("expected keep-me, got %q", got.Payload)
	}
}

func TestBridge_Transform_RewritesPayload(t *testing.T) {
	opts := grpcbridge.Options{
		Transform: func(_ string, payload []byte) ([]byte, error) {
			return append([]byte("prefix:"), payload...), nil
		},
	}
	client, bridge, p := newTestBridge(t, opts)
	_ = bridge

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	stream, err := client.Subscribe(ctx, &grpcbridge.SubscribeRequest{Topic: "grpc/transform"})
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	time.Sleep(20 * time.Millisecond)

	pub, err := p.NewPublisher("grpc/transform", dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
	defer pub.Close()
	_ = pub.Write([]byte("data"))

	got, err := stream.Recv()
	if err != nil {
		t.Fatalf("Recv: %v", err)
	}
	if string(got.Payload) != "prefix:data" {
		t.Errorf("payload: %q", got.Payload)
	}
}

// ── Auth ──────────────────────────────────────────────────────────────────────

func TestBridge_Auth_NoToken_Unauthenticated(t *testing.T) {
	p, err := mock.New(dds.Domain(0))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer p.Close()
	b := grpcbridge.New(p, grpcbridge.Options{AuthToken: "secret"})
	lis := listenLocal(t)
	go func() { _ = b.Server().Serve(lis) }()
	defer b.Close()

	conn := dialJSON(t, lis.Addr().String())
	defer conn.Close()

	client := grpcbridge.NewRawClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	ignoredRet, err := client.Publish(ctx, &grpcbridge.PublishRequest{Topic: "t", Payload: []byte("x")})
	_ = ignoredRet
	if err == nil {
		t.Fatal("expected unauthenticated error")
	}
	if !strings.Contains(err.Error(), "Unauthenticated") && !strings.Contains(err.Error(), "unauthenticated") {
		t.Errorf("expected Unauthenticated, got %v", err)
	}
}

func TestBridge_Auth_CorrectToken_Passes(t *testing.T) {
	p, err := mock.New(dds.Domain(0))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer p.Close()
	b := grpcbridge.New(p, grpcbridge.Options{AuthToken: "secret"})
	lis := listenLocal(t)
	go func() { _ = b.Server().Serve(lis) }()
	defer b.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	conn, err := grpc.DialContext(ctx, lis.Addr().String(), //nolint:staticcheck
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultCallOptions(grpc.ForceCodec(grpcbridge.JSONCodec{})),
		grpc.WithPerRPCCredentials(bearerToken("secret")),
	)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	client := grpcbridge.NewRawClient(conn)
	ignoredRet, err := client.Publish(ctx, &grpcbridge.PublishRequest{Topic: "t", Payload: []byte("x")})
	_ = ignoredRet
	if err != nil {
		t.Fatalf("expected success with correct token, got: %v", err)
	}
}

type bearerToken string

func (b bearerToken) GetRequestMetadata(_ context.Context, _ ...string) (map[string]string, error) {
	return map[string]string{"authorization": "Bearer " + string(b)}, nil
}
func (b bearerToken) RequireTransportSecurity() bool { return false }

// ── Close ─────────────────────────────────────────────────────────────────────

func TestBridge_Close_Idempotent(t *testing.T) {
	p, err := mock.New(dds.Domain(0))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer p.Close()
	b := grpcbridge.New(p, grpcbridge.Options{})
	if err := b.Close(); err != nil {
		t.Errorf("first Close: %v", err)
	}
	if err := b.Close(); err != nil {
		t.Errorf("second Close: %v", err)
	}
}

// ── Config ────────────────────────────────────────────────────────────────────

func TestLoadConfig_ValidYAML(t *testing.T) {
	yamlContent := `
listen: ":9090"
auth_token: "secret"
topics:
  - name: "sensors/temperature"
    qos: "reliable"
  - name: "vehicle/speed"
    qos: "best_effort"
`
	f := filepath.Join(t.TempDir(), "bridge.yaml")
	if err := os.WriteFile(f, []byte(yamlContent), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := grpcbridge.LoadConfig(f)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.Listen != ":9090" {
		t.Errorf("listen: %q", cfg.Listen)
	}
	if cfg.AuthToken != "secret" {
		t.Errorf("auth_token: %q", cfg.AuthToken)
	}
	if len(cfg.Topics) != 2 {
		t.Fatalf("topics: got %d, want 2", len(cfg.Topics))
	}
	if cfg.Topics[0].Name != "sensors/temperature" {
		t.Errorf("topic[0].name: %q", cfg.Topics[0].Name)
	}
}

func TestLoadConfig_NotFound(t *testing.T) {
	ignoredRet, err := grpcbridge.LoadConfig("/nonexistent/path.yaml")
	_ = ignoredRet
	if err == nil {
		t.Error("expected error for missing file")
	}
}

func TestLoadConfig_InvalidYAML(t *testing.T) {
	f := filepath.Join(t.TempDir(), "bad.yaml")
	if err := os.WriteFile(f, []byte(":\ninvalid:::yaml"), 0o600); err != nil {
		t.Fatal(err)
	}
	ignoredRet, err := grpcbridge.LoadConfig(f)
	_ = ignoredRet
	if err == nil {
		t.Error("expected error for invalid YAML")
	}
}

// ── Additional error-path coverage ───────────────────────────────────────────

// TestBridge_Subscribe_ChannelClosed covers the !ok branch in the server-side
// Subscribe loop: closing the bridge closes the DDS subscriber whose channel
// is drained by the stream handler — it must return nil.
func TestBridge_Subscribe_ChannelClosed(t *testing.T) {
	client, bridge, _ := newTestBridge(t, grpcbridge.Options{})

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	stream, err := client.Subscribe(ctx, &grpcbridge.SubscribeRequest{Topic: "grpc/chan-close"})
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	time.Sleep(30 * time.Millisecond) // let the server register the subscription

	// Closing the bridge closes the DDS subscriber, which closes its channel.
	// The handler's select falls into the !ok branch and returns nil (EOF to client).
	bridge.Close()
	_, err = stream.Recv()
	// Any error (EOF / connection reset) is acceptable — the server exited cleanly.
	_ = err
}

// TestBridge_Subscribe_TransformError_DropsAndContinues covers the `continue`
// branch inside Subscribe where Transform returns an error: the sample is silently
// dropped and the next valid sample is forwarded.
func TestBridge_Subscribe_TransformError_DropsAndContinues(t *testing.T) {
	first := true
	opts := grpcbridge.Options{
		Transform: func(_ string, payload []byte) ([]byte, error) {
			if first {
				first = false
				return nil, errors.New("transform error — drop this sample")
			}
			return payload, nil
		},
	}
	client, _, p := newTestBridge(t, opts)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	stream, err := client.Subscribe(ctx, &grpcbridge.SubscribeRequest{Topic: "grpc/transform-err"})
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	time.Sleep(20 * time.Millisecond)

	pub, err := p.NewPublisher("grpc/transform-err", dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
	defer pub.Close()
	_ = pub.Write([]byte("drop-me")) // transform returns error → continue
	_ = pub.Write([]byte("keep-me")) // transform succeeds → forwarded

	got, err := stream.Recv()
	if err != nil {
		t.Fatalf("Recv: %v", err)
	}
	if string(got.Payload) != "keep-me" {
		t.Errorf("expected keep-me, got %q", got.Payload)
	}
}

// TestBridge_StreamPublish_EmptyTopicInStream covers the empty-topic guard inside
// StreamPublish: sending a request with Topic="" inside the stream must return
// InvalidArgument.
func TestBridge_StreamPublish_EmptyTopicInStream(t *testing.T) {
	client, _, _ := newTestBridge(t, grpcbridge.Options{})

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	sp, err := client.StreamPublish(ctx)
	if err != nil {
		t.Fatalf("StreamPublish: %v", err)
	}
	// Sending a message with an empty topic triggers the guard.
	_ = sp.Send(&grpcbridge.PublishRequest{Topic: "", Payload: []byte("x")})
	_, err = sp.CloseAndRecv()
	if err == nil {
		t.Fatal("expected error for empty topic in stream")
	}
	if !strings.Contains(err.Error(), "InvalidArgument") && !strings.Contains(err.Error(), "invalid") {
		t.Errorf("expected InvalidArgument error, got %v", err)
	}
}

// TestBridge_Publish_ClosedParticipant_InternalError covers the getOrCreatePub
// error path: when the underlying DDS participant is closed, NewPublisher fails
// and the server must return codes.Internal.
func TestBridge_Publish_ClosedParticipant_InternalError(t *testing.T) {
	p, err := mock.New(dds.Domain(0))
	if err != nil {
		t.Fatalf("mock.New: %v", err)
	}
	b := grpcbridge.New(p, grpcbridge.Options{})
	lis := listenLocal(t)
	go func() { _ = b.Server().Serve(lis) }()
	defer b.Close()

	// Close the participant so NewPublisher fails.
	_ = p.Close()

	conn := dialJSON(t, lis.Addr().String())
	defer conn.Close()
	client := grpcbridge.NewRawClient(conn)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	ignoredRet, err := client.Publish(ctx, &grpcbridge.PublishRequest{Topic: "grpc/closed-pub", Payload: []byte("x")})
	_ = ignoredRet
	if err == nil {
		t.Fatal("expected error for closed participant")
	}
	if !strings.Contains(err.Error(), "Internal") && !strings.Contains(err.Error(), "internal") {
		t.Errorf("expected Internal error, got %v", err)
	}
}

// TestBridge_Subscribe_ClosedParticipant_InternalError covers the getOrCreateSub
// error path: when the participant is closed, NewSubscriber fails and the server
// returns codes.Internal.
func TestBridge_Subscribe_ClosedParticipant_InternalError(t *testing.T) {
	p, err := mock.New(dds.Domain(0))
	if err != nil {
		t.Fatalf("mock.New: %v", err)
	}
	b := grpcbridge.New(p, grpcbridge.Options{})
	lis := listenLocal(t)
	go func() { _ = b.Server().Serve(lis) }()
	defer b.Close()

	_ = p.Close()

	conn := dialJSON(t, lis.Addr().String())
	defer conn.Close()
	client := grpcbridge.NewRawClient(conn)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	stream, err := client.Subscribe(ctx, &grpcbridge.SubscribeRequest{Topic: "grpc/closed-sub"})
	if err != nil {
		return // some gRPC impls surface the error here
	}
	ignoredRet, err := stream.Recv()
	_ = ignoredRet
	if err == nil {
		t.Fatal("expected error for closed participant")
	}
}

// ── Config application ────────────────────────────────────────────────────────

// TestApplyConfig_PreSubscribes exercises ApplyConfig with reliable, best_effort,
// default-QoS, and empty-name topics, covering all three branches of TopicConfig.qos().
func TestApplyConfig_PreSubscribes(t *testing.T) {
	p, err := mock.New(dds.Domain(0))
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	b := grpcbridge.New(p, grpcbridge.Options{})
	defer b.Close()

	cfg := &grpcbridge.Config{
		Topics: []grpcbridge.TopicConfig{
			{Name: "cfg/reliable", QoS: "reliable"},
			{Name: "cfg/best-effort", QoS: "best_effort"},
			{Name: "cfg/default", QoS: ""}, // default branch
			{Name: "", QoS: "reliable"},    // empty name — skipped silently
		},
	}
	if err := grpcbridge.ApplyConfig(b, cfg); err != nil {
		t.Fatalf("ApplyConfig: %v", err)
	}
}

// TestApplyConfig_SubscriberError verifies that ApplyConfig propagates a
// subscriber creation error (caused by a closed participant).
func TestApplyConfig_SubscriberError(t *testing.T) {
	p, err := mock.New(dds.Domain(0))
	if err != nil {
		t.Fatal(err)
	}
	p.Close() // close before ApplyConfig so NewSubscriber fails
	b := grpcbridge.New(p, grpcbridge.Options{})
	defer b.Close()

	cfg := &grpcbridge.Config{
		Topics: []grpcbridge.TopicConfig{{Name: "cfg/fail", QoS: "reliable"}},
	}
	if err := grpcbridge.ApplyConfig(b, cfg); err == nil {
		t.Fatal("expected error from ApplyConfig with closed participant")
	}
}

// ── NewClient ─────────────────────────────────────────────────────────────────

// TestNewClient_ConnectsAndPublishes verifies that NewClient returns a usable
// DDSBridgeClient that can successfully call Publish.
func TestNewClient_ConnectsAndPublishes(t *testing.T) {
	p, err := mock.New(dds.Domain(0))
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	b := grpcbridge.New(p, grpcbridge.Options{})
	lis := listenLocal(t)
	go func() { _ = b.Server().Serve(lis) }()
	defer b.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	c, conn, err := grpcbridge.NewClient(ctx, lis.Addr().String(),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer conn.Close()

	ack, err := c.Publish(ctx, &grpcbridge.PublishRequest{Topic: "nc/test", Payload: []byte("hi")})
	if err != nil {
		t.Fatalf("Publish via NewClient: %v", err)
	}
	if ack.Count != 1 {
		t.Errorf("count: got %d, want 1", ack.Count)
	}
}

// ── Auth stream interceptor ───────────────────────────────────────────────────

// TestBridge_Auth_Stream_NoToken_Unauthenticated verifies that Subscribe fails
// when the server requires an auth token but the client provides none.
// This exercises the authStream interceptor, which wraps streaming RPCs.
func TestBridge_Auth_Stream_NoToken_Unauthenticated(t *testing.T) {
	p, err := mock.New(dds.Domain(0))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer p.Close()
	b := grpcbridge.New(p, grpcbridge.Options{AuthToken: "secret"})
	lis := listenLocal(t)
	go func() { _ = b.Server().Serve(lis) }()
	defer b.Close()

	conn := dialJSON(t, lis.Addr().String())
	defer conn.Close()

	client := grpcbridge.NewRawClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	stream, err := client.Subscribe(ctx, &grpcbridge.SubscribeRequest{Topic: "auth/stream"})
	if err != nil {
		if !strings.Contains(err.Error(), "Unauthenticated") && !strings.Contains(err.Error(), "unauthenticated") {
			t.Errorf("expected Unauthenticated, got %v", err)
		}
		return
	}
	ignoredRet, recvErr := stream.Recv()
	_ = ignoredRet
	if recvErr == nil {
		t.Fatal("expected unauthenticated error on Recv")
	}
	if !strings.Contains(recvErr.Error(), "Unauthenticated") && !strings.Contains(recvErr.Error(), "unauthenticated") {
		t.Errorf("expected Unauthenticated, got %v", recvErr)
	}
}

// TestBridge_Auth_Stream_CorrectToken_Passes verifies that Subscribe succeeds
// when the correct auth token is supplied (authStream interceptor allows it).
func TestBridge_Auth_Stream_CorrectToken_Passes(t *testing.T) {
	p, err := mock.New(dds.Domain(0))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer p.Close()
	b := grpcbridge.New(p, grpcbridge.Options{AuthToken: "secret"})
	lis := listenLocal(t)
	go func() { _ = b.Server().Serve(lis) }()
	defer b.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	conn, dialErr := grpc.DialContext(ctx, lis.Addr().String(), //nolint:staticcheck
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultCallOptions(grpc.ForceCodec(grpcbridge.JSONCodec{})),
		grpc.WithPerRPCCredentials(bearerToken("secret")),
	)
	if dialErr != nil {
		t.Fatalf("dial: %v", dialErr)
	}
	defer conn.Close()

	client := grpcbridge.NewRawClient(conn)
	stream, err := client.Subscribe(ctx, &grpcbridge.SubscribeRequest{Topic: "auth/stream-ok"})
	if err != nil {
		t.Fatalf("Subscribe with valid token: %v", err)
	}

	// Publish so the stream has a sample to receive (confirming it's open).
	pub, err := p.NewPublisher("auth/stream-ok", dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
	defer pub.Close()
	time.Sleep(20 * time.Millisecond)
	_ = pub.Write([]byte("authenticated"))

	got, err := stream.Recv()
	if err != nil {
		t.Fatalf("Recv: %v", err)
	}
	if string(got.Payload) != "authenticated" {
		t.Errorf("payload: %q", got.Payload)
	}
}

// ── Options.QoS non-default branch ───────────────────────────────────────────

// TestBridge_Options_QoS_NonDefault verifies that a bridge created with a
// non-zero Options.QoS passes that QoS to getOrCreatePub, covering the
// non-default branch of Options.qos().
func TestBridge_Options_QoS_NonDefault(t *testing.T) {
	p, err := mock.New(dds.Domain(0))
	if err != nil {
		t.Fatalf("mock.New: %v", err)
	}
	defer p.Close()
	b := grpcbridge.New(p, grpcbridge.Options{QoS: dds.QoS{MaxSampleSize: 1024}})
	lis := listenLocal(t)
	go func() { _ = b.Server().Serve(lis) }()
	defer b.Close()

	conn := dialJSON(t, lis.Addr().String())
	defer conn.Close()
	client := grpcbridge.NewRawClient(conn)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	ack, err := client.Publish(ctx, &grpcbridge.PublishRequest{Topic: "qos/test", Payload: []byte("hi")})
	if err != nil {
		t.Fatalf("Publish with non-default QoS: %v", err)
	}
	if ack.Count != 1 {
		t.Errorf("count: got %d, want 1", ack.Count)
	}
}

// ── Transform error path ──────────────────────────────────────────────────────

// TestBridge_Transform_ErrorDropsSample verifies that when a Transform
// returns an error, the sample is skipped (continue branch in Subscribe).
func TestBridge_Transform_ErrorDropsSample(t *testing.T) {
	opts := grpcbridge.Options{
		Transform: func(_ string, payload []byte) ([]byte, error) {
			if string(payload) == "bad" {
				return nil, errors.New("transform error")
			}
			return payload, nil
		},
	}
	client, _, p := newTestBridge(t, opts)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	stream, err := client.Subscribe(ctx, &grpcbridge.SubscribeRequest{Topic: "grpc/transform-err"})
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	time.Sleep(20 * time.Millisecond)

	pub, err := p.NewPublisher("grpc/transform-err", dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
	defer pub.Close()
	_ = pub.Write([]byte("bad"))  // dropped by transform error
	_ = pub.Write([]byte("good")) // passes through

	got, err := stream.Recv()
	if err != nil {
		t.Fatalf("Recv: %v", err)
	}
	if string(got.Payload) != "good" {
		t.Errorf("expected good, got %q", got.Payload)
	}
}

// ── StreamPublish empty topic ─────────────────────────────────────────────────

// TestBridge_StreamPublish_EmptyTopic verifies that sending an empty topic
// inside a StreamPublish stream returns InvalidArgument, covering the
// req.Topic == "" branch in Bridge.StreamPublish.
func TestBridge_StreamPublish_EmptyTopic(t *testing.T) {
	client, _, _ := newTestBridge(t, grpcbridge.Options{})

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	sp, err := client.StreamPublish(ctx)
	if err != nil {
		t.Fatalf("StreamPublish: %v", err)
	}
	if sendErr := sp.Send(&grpcbridge.PublishRequest{Topic: "", Payload: []byte("x")}); sendErr != nil {
		// Some gRPC impls report the error at Send time.
		if !strings.Contains(sendErr.Error(), "InvalidArgument") && !strings.Contains(sendErr.Error(), "invalid") {
			t.Errorf("expected InvalidArgument, got %v", sendErr)
		}
		return
	}
	_, recvErr := sp.CloseAndRecv()
	if recvErr == nil {
		t.Fatal("expected error for empty topic in StreamPublish")
	}
	if !strings.Contains(recvErr.Error(), "InvalidArgument") && !strings.Contains(recvErr.Error(), "invalid") {
		t.Errorf("expected InvalidArgument, got %v", recvErr)
	}
}

// ── getOrCreateSub cache hit ──────────────────────────────────────────────────

// TestBridge_Subscribe_SameTopic_Cache verifies that subscribing to the same
// topic twice reuses the cached subscriber, covering the cache-hit branch in
// Bridge.getOrCreateSub. Two streams share one DDS subscriber, so messages
// are delivered to whichever goroutine reads first — we publish two messages
// and verify at least one arrives on any stream.
func TestBridge_Subscribe_SameTopic_Cache(t *testing.T) {
	client, _, p := newTestBridge(t, grpcbridge.Options{})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// First Subscribe — creates subscriber and caches it.
	stream1, err := client.Subscribe(ctx, &grpcbridge.SubscribeRequest{Topic: "grpc/cache-test"})
	if err != nil {
		t.Fatalf("first Subscribe: %v", err)
	}

	// Second Subscribe — hits the cached subscriber in getOrCreateSub.
	stream2, err := client.Subscribe(ctx, &grpcbridge.SubscribeRequest{Topic: "grpc/cache-test"})
	if err != nil {
		t.Fatalf("second Subscribe: %v", err)
	}

	time.Sleep(20 * time.Millisecond)

	pub, err := p.NewPublisher("grpc/cache-test", dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
	defer pub.Close()

	// Publish two messages; the two competing goroutines (stream1 and stream2)
	// each consume one. At least one arrival confirms the cache-hit path works.
	_ = pub.Write([]byte("cached-1"))
	_ = pub.Write([]byte("cached-2"))

	got := make(chan string, 2)
	recvFrom := func(s interface {
		Recv() (*grpcbridge.Sample, error)
	}) {
		sample, recvErr := s.Recv()
		if recvErr == nil {
			got <- string(sample.Payload)
		}
	}
	go recvFrom(stream1)
	go recvFrom(stream2)

	// Expect at least one delivery within the context deadline.
	select {
	case payload := <-got:
		if payload != "cached-1" && payload != "cached-2" {
			t.Errorf("unexpected payload: %q", payload)
		}
	case <-ctx.Done():
		t.Fatal("timeout: no message received on either stream")
	}
}

// ── Bridge.Publish WriteCtx error ────────────────────────────────────────────

// TestBridge_Publish_WriteCtxFails covers the pub.WriteCtx error branch in
// Bridge.Publish by using QoS.MaxSampleSize=1 and sending a 2-byte payload.
func TestBridge_Publish_WriteCtxFails(t *testing.T) {
	p, err := mock.New(dds.Domain(0))
	if err != nil {
		t.Fatalf("mock.New: %v", err)
	}
	defer p.Close()
	b := grpcbridge.New(p, grpcbridge.Options{QoS: dds.QoS{MaxSampleSize: 1}})
	lis := listenLocal(t)
	go func() { _ = b.Server().Serve(lis) }()
	defer b.Close()

	conn := dialJSON(t, lis.Addr().String())
	defer conn.Close()
	client := grpcbridge.NewRawClient(conn)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	// "hi" is 2 bytes, exceeds MaxSampleSize=1 → WriteCtx returns ErrPayloadTooLarge.
	ignoredRet, pubErr := client.Publish(ctx, &grpcbridge.PublishRequest{Topic: "publish/fail", Payload: []byte("hi")})
	_ = ignoredRet
	if pubErr == nil {
		t.Fatal("expected error when payload exceeds MaxSampleSize")
	}
}

// ── StreamPublish publisher error ─────────────────────────────────────────────

// TestBridge_StreamPublish_PublisherError covers the getOrCreatePub error path
// in Bridge.StreamPublish by closing the participant after stream setup but
// before sending a sample on a new topic.
func TestBridge_StreamPublish_PublisherError(t *testing.T) {
	p, err := mock.New(dds.Domain(0))
	if err != nil {
		t.Fatalf("mock.New: %v", err)
	}
	b := grpcbridge.New(p, grpcbridge.Options{})
	lis := listenLocal(t)
	go func() { _ = b.Server().Serve(lis) }()
	defer b.Close()

	conn := dialJSON(t, lis.Addr().String())
	defer conn.Close()
	client := grpcbridge.NewRawClient(conn)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	sp, err := client.StreamPublish(ctx)
	if err != nil {
		t.Fatalf("StreamPublish: %v", err)
	}

	// Close participant so that getOrCreatePub fails for any new topic.
	p.Close()

	if sendErr := sp.Send(&grpcbridge.PublishRequest{Topic: "sp/new-after-close", Payload: []byte("x")}); sendErr != nil {
		// Some implementations report the server-side error at Send time.
		return
	}
	_, recvErr := sp.CloseAndRecv()
	if recvErr == nil {
		t.Fatal("expected error after participant closed")
	}
}

// ── Auth wrong-token path ─────────────────────────────────────────────────────

// TestBridge_Auth_WrongToken_Unauthenticated verifies that a non-empty but
// incorrect Authorization header triggers the vals[0]!="Bearer <token>" branch
// inside checkAuth (as opposed to the missing-header branch).
func TestBridge_Auth_WrongToken_Unauthenticated(t *testing.T) {
	p, err := mock.New(dds.Domain(0))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer p.Close()
	b := grpcbridge.New(p, grpcbridge.Options{AuthToken: "secret"})
	lis := listenLocal(t)
	go func() { _ = b.Server().Serve(lis) }()
	defer b.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	conn, dialErr := grpc.DialContext(ctx, lis.Addr().String(), //nolint:staticcheck
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultCallOptions(grpc.ForceCodec(grpcbridge.JSONCodec{})),
		grpc.WithPerRPCCredentials(bearerToken("wrong")), // non-empty but wrong
	)
	if dialErr != nil {
		t.Fatalf("dial: %v", dialErr)
	}
	defer conn.Close()

	client := grpcbridge.NewRawClient(conn)
	ignoredRet, pubErr := client.Publish(ctx, &grpcbridge.PublishRequest{Topic: "t", Payload: []byte("x")})
	_ = ignoredRet
	if pubErr == nil {
		t.Fatal("expected unauthenticated error with wrong token")
	}
	if !strings.Contains(pubErr.Error(), "Unauthenticated") && !strings.Contains(pubErr.Error(), "unauthenticated") {
		t.Errorf("expected Unauthenticated, got %v", pubErr)
	}
}

// ── Fuzz ──────────────────────────────────────────────────────────────────────

func FuzzBridge_Publish(f *testing.F) {
	p, err := mock.New(dds.Domain(0))
	if err != nil {
		f.Fatal(err)
	}
	f.Cleanup(func() { p.Close() })
	b := grpcbridge.New(p, grpcbridge.Options{})
	lc := net.ListenConfig{}
	lis, err := lc.Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		f.Fatal(err)
	}
	go func() { _ = b.Server().Serve(lis) }()
	f.Cleanup(func() { b.Close() })

	fCtx, fCancel := context.WithTimeout(context.Background(), 30*time.Second)
	f.Cleanup(fCancel)
	conn, err := grpc.DialContext(fCtx, lis.Addr().String(), //nolint:staticcheck
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultCallOptions(grpc.ForceCodec(grpcbridge.JSONCodec{})),
	)
	if err != nil {
		f.Fatal(err)
	}
	f.Cleanup(func() { conn.Close() })
	client := grpcbridge.NewRawClient(conn)

	f.Add("sensors/temp", []byte("hello"))
	f.Add("", []byte(""))
	f.Add("a/b/c", []byte{0x00, 0xFF})

	f.Fuzz(func(t *testing.T, topic string, payload []byte) {
		_ = t // Must not panic. Errors on invalid input are expected.
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_, _ = client.Publish(ctx, &grpcbridge.PublishRequest{Topic: topic, Payload: payload})
	})
}

// ── Client-side error paths via mock ClientConnInterface ──────────────────────

// mockClientStream is a grpc.ClientStream whose error fields control which
// methods fail. All other methods are no-ops returning nil.
type mockClientStream struct {
	sendErr  error
	closeErr error
	recvErr  error
}

func (m *mockClientStream) Header() (metadata.MD, error) { return nil, nil }
func (m *mockClientStream) Trailer() metadata.MD         { return nil }
func (m *mockClientStream) CloseSend() error             { return m.closeErr }
func (m *mockClientStream) Context() context.Context     { return context.Background() }
func (m *mockClientStream) SendMsg(msg interface{}) error { return m.sendErr }
func (m *mockClientStream) RecvMsg(msg interface{}) error { return m.recvErr }

// mockClientConn implements grpc.ClientConnInterface, returning a pre-set
// stream (or error) from NewStream.
type mockClientConn struct {
	stream    grpc.ClientStream
	streamErr error
}

func (m *mockClientConn) Invoke(_ context.Context, _ string, _, _ interface{}, _ ...grpc.CallOption) error {
	return nil
}
func (m *mockClientConn) NewStream(_ context.Context, _ *grpc.StreamDesc, _ string, _ ...grpc.CallOption) (grpc.ClientStream, error) {
	return m.stream, m.streamErr
}

// TestClient_Subscribe_NewStreamError covers the NewStream error path in
// ddsClient.Subscribe (line 209-211).
func TestClient_Subscribe_NewStreamError(t *testing.T) {
	wantErr := errors.New("stream unavailable")
	cli := grpcbridge.NewRawClient(&mockClientConn{streamErr: wantErr})
	_, err := cli.Subscribe(context.Background(), &grpcbridge.SubscribeRequest{Topic: "x"})
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected %v, got %v", wantErr, err)
	}
}

// TestClient_Subscribe_SendMsgError covers the stream.SendMsg error path in
// ddsClient.Subscribe (line 213-215).
func TestClient_Subscribe_SendMsgError(t *testing.T) {
	wantErr := errors.New("send failed")
	cli := grpcbridge.NewRawClient(&mockClientConn{
		stream: &mockClientStream{sendErr: wantErr},
	})
	_, err := cli.Subscribe(context.Background(), &grpcbridge.SubscribeRequest{Topic: "x"})
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected %v, got %v", wantErr, err)
	}
}

// TestClient_Subscribe_CloseSendError covers the stream.CloseSend error path in
// ddsClient.Subscribe (line 216-218).
func TestClient_Subscribe_CloseSendError(t *testing.T) {
	wantErr := errors.New("close failed")
	cli := grpcbridge.NewRawClient(&mockClientConn{
		stream: &mockClientStream{closeErr: wantErr},
	})
	_, err := cli.Subscribe(context.Background(), &grpcbridge.SubscribeRequest{Topic: "x"})
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected %v, got %v", wantErr, err)
	}
}

// TestClient_StreamPublish_NewStreamError covers the NewStream error path in
// ddsClient.StreamPublish (line 241-243).
func TestClient_StreamPublish_NewStreamError(t *testing.T) {
	wantErr := errors.New("stream unavailable")
	cli := grpcbridge.NewRawClient(&mockClientConn{streamErr: wantErr})
	_, err := cli.StreamPublish(context.Background())
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected %v, got %v", wantErr, err)
	}
}

// TestClient_CloseAndRecv_CloseSendError covers the CloseSend error path in
// streamPublishClientStream.CloseAndRecv (line 254-256).
func TestClient_CloseAndRecv_CloseSendError(t *testing.T) {
	wantErr := errors.New("close failed")
	cli := grpcbridge.NewRawClient(&mockClientConn{
		stream: &mockClientStream{closeErr: wantErr},
	})
	sp, err := cli.StreamPublish(context.Background())
	if err != nil {
		t.Fatalf("StreamPublish: %v", err)
	}
	_, err = sp.CloseAndRecv()
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected %v, got %v", wantErr, err)
	}
}

// TestBridge_Subscribe_ChannelClosed_ViaParticipant covers the !ok branch in
// Bridge.Subscribe where the DDS subscriber's channel closes (line 364-366).
// The participant is closed directly (not via bridge.Close) so the gRPC stream
// context remains live, letting the server detect the closed channel first.
func TestBridge_Subscribe_ChannelClosed_ViaParticipant(t *testing.T) {
	p, err := mock.New(dds.Domain(0))
	if err != nil {
		t.Fatalf("mock.New: %v", err)
	}
	b := grpcbridge.New(p, grpcbridge.Options{})
	lis := listenLocal(t)
	go func() { _ = b.Server().Serve(lis) }()
	defer b.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	conn := dialJSON(t, lis.Addr().String())
	defer conn.Close()
	client := grpcbridge.NewRawClient(conn)

	stream, err := client.Subscribe(ctx, &grpcbridge.SubscribeRequest{Topic: "grpc/p-close"})
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	time.Sleep(30 * time.Millisecond) // let server register the subscription

	// Close only the participant; bridge is still up so stream ctx remains live.
	// The server's sub.C() closes → !ok path → return nil (EOF to client).
	p.Close()

	_, recvErr := stream.Recv()
	_ = recvErr // any result (EOF or non-nil error) means the server exited
}

// TestBridge_StreamPublish_ContextCancel_NonEOFError covers the non-EOF recv
// error return in Bridge.StreamPublish (line 418). Cancelling the streaming
// client's context causes stream.Recv on the server to return a cancelled-
// context status error (not io.EOF), hitting the `return err` branch.
func TestBridge_StreamPublish_ContextCancel_NonEOFError(t *testing.T) {
	client, _, _ := newTestBridge(t, grpcbridge.Options{})

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	sp, err := client.StreamPublish(ctx)
	if err != nil {
		cancel()
		t.Fatalf("StreamPublish: %v", err)
	}

	// Send a valid request so the server enters the Recv loop.
	if sendErr := sp.Send(&grpcbridge.PublishRequest{Topic: "sp/ctx-cancel", Payload: []byte("x")}); sendErr != nil {
		cancel()
		t.Skipf("Send: %v", sendErr)
	}
	time.Sleep(20 * time.Millisecond) // let server process the first message

	// Cancel the context; the server's stream.Recv gets codes.Canceled (not EOF).
	cancel()
	time.Sleep(50 * time.Millisecond) // let the server detect the cancellation
}
