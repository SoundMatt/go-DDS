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
	client, _, _ := newTestBridge(t, grpcbridge.Options{})

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
	client, _, _ := newTestBridge(t, grpcbridge.Options{})

	_, err := client.Publish(context.Background(), &grpcbridge.PublishRequest{Topic: ""})
	if err == nil {
		t.Fatal("expected error for empty topic")
	}
	if !strings.Contains(err.Error(), "InvalidArgument") && !strings.Contains(err.Error(), "invalid") {
		t.Errorf("expected InvalidArgument, got %v", err)
	}
}

// ── Subscribe ─────────────────────────────────────────────────────────────────

func TestBridge_Subscribe_ReceivesSample(t *testing.T) {
	client, _, p := newTestBridge(t, grpcbridge.Options{})

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	stream, err := client.Subscribe(ctx, &grpcbridge.SubscribeRequest{Topic: "grpc/sub"})
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	// Give the subscription time to register.
	time.Sleep(20 * time.Millisecond)

	pub, _ := p.NewPublisher("grpc/sub", dds.DefaultQoS)
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
	client, _, _ := newTestBridge(t, grpcbridge.Options{})

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	stream, err := client.Subscribe(ctx, &grpcbridge.SubscribeRequest{Topic: ""})
	if err != nil {
		return // some gRPC impls return the error at Subscribe time
	}
	_, err = stream.Recv()
	if err == nil {
		t.Fatal("expected error for empty topic")
	}
}

// ── StreamPublish ─────────────────────────────────────────────────────────────

func TestBridge_StreamPublish_ReturnsCount(t *testing.T) {
	client, _, _ := newTestBridge(t, grpcbridge.Options{})

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
	client, _, p := newTestBridge(t, opts)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	stream, err := client.Subscribe(ctx, &grpcbridge.SubscribeRequest{Topic: "grpc/filter"})
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	time.Sleep(20 * time.Millisecond)

	pub, _ := p.NewPublisher("grpc/filter", dds.DefaultQoS)
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
	client, _, p := newTestBridge(t, opts)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	stream, err := client.Subscribe(ctx, &grpcbridge.SubscribeRequest{Topic: "grpc/transform"})
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	time.Sleep(20 * time.Millisecond)

	pub, _ := p.NewPublisher("grpc/transform", dds.DefaultQoS)
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
	p, _ := mock.New(dds.Domain(0))
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

	_, err := client.Publish(ctx, &grpcbridge.PublishRequest{Topic: "t", Payload: []byte("x")})
	if err == nil {
		t.Fatal("expected unauthenticated error")
	}
	if !strings.Contains(err.Error(), "Unauthenticated") && !strings.Contains(err.Error(), "unauthenticated") {
		t.Errorf("expected Unauthenticated, got %v", err)
	}
}

func TestBridge_Auth_CorrectToken_Passes(t *testing.T) {
	p, _ := mock.New(dds.Domain(0))
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
	_, err = client.Publish(ctx, &grpcbridge.PublishRequest{Topic: "t", Payload: []byte("x")})
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
	p, _ := mock.New(dds.Domain(0))
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
	_, err := grpcbridge.LoadConfig("/nonexistent/path.yaml")
	if err == nil {
		t.Error("expected error for missing file")
	}
}

func TestLoadConfig_InvalidYAML(t *testing.T) {
	f := filepath.Join(t.TempDir(), "bad.yaml")
	if err := os.WriteFile(f, []byte(":\ninvalid:::yaml"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := grpcbridge.LoadConfig(f)
	if err == nil {
		t.Error("expected error for invalid YAML")
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
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_, _ = client.Publish(ctx, &grpcbridge.PublishRequest{Topic: topic, Payload: payload})
		// Must not panic.
	})
}
