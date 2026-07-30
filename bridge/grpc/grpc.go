// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Package grpcbridge provides a gRPC gateway that bridges a DDS participant to
// gRPC clients using JSON encoding.
//
// The DDSBridge service exposes three RPCs:
//
//   - Subscribe(SubscribeRequest) → stream(Sample): server-streaming, delivers
//     DDS samples as they arrive.
//   - Publish(PublishRequest) → PublishAck: unary publish of a single sample.
//   - StreamPublish(stream PublishRequest) → PublishAck: client-streaming for
//     high-throughput writes; returns the total count when the stream closes.
//
// Messages are JSON-encoded over standard gRPC framing
// (Content-Type: application/grpc+json). Go clients set this via
// grpc.ForceCodec(grpcbridge.JSONCodec{}) on the dial options.
//
// Usage:
//
//	p, _ := rtps.New(dds.Domain(0))
//	b := grpcbridge.New(p, grpcbridge.Options{})
//	lis, _ := net.Listen("tcp", ":9090")
//	b.Server().Serve(lis)
package grpcbridge

//fusa:req REQ-BRIDGE-006
//fusa:req REQ-BRIDGE-007
//fusa:req REQ-BRIDGE-008
//fusa:req REQ-BRIDGE-009
//fusa:req REQ-BRIDGE-010
//fusa:req REQ-BRIDGE-011
//fusa:req REQ-BRIDGE-012

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"sync"

	dds "github.com/SoundMatt/go-DDS"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/encoding"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// JSONCodec encodes gRPC messages as JSON. Register it via
// encoding.RegisterCodec(JSONCodec{}) or grpc.ForceCodec(JSONCodec{}).
type JSONCodec struct{}

func (JSONCodec) Marshal(v interface{}) ([]byte, error)      { return json.Marshal(v) }
func (JSONCodec) Unmarshal(data []byte, v interface{}) error { return json.Unmarshal(data, v) }
func (JSONCodec) Name() string                               { return "json" }

func init() { encoding.RegisterCodec(JSONCodec{}) }

// ── Message types ─────────────────────────────────────────────────────────────

// SubscribeRequest is the request message for the Subscribe RPC.
type SubscribeRequest struct {
	Topic string `json:"topic"`
}

// PublishRequest is the request message for Publish and StreamPublish RPCs.
type PublishRequest struct {
	Topic   string `json:"topic"`
	Payload []byte `json:"payload"`
}

// Sample is a DDS sample delivered by the Subscribe RPC.
type Sample struct {
	Topic          string `json:"topic"`
	Payload        []byte `json:"payload"`
	SequenceNumber uint64 `json:"seq_num"`
	TimestampNs    int64  `json:"timestamp_ns"`
	WriterGUID     []byte `json:"writer_guid"`
}

// PublishAck is the response from Publish and StreamPublish RPCs.
type PublishAck struct {
	Count uint32 `json:"count"`
}

// ── Server interface ──────────────────────────────────────────────────────────

// DDSBridgeServer is implemented by Bridge and registered with a grpc.Server.
type DDSBridgeServer interface {
	Subscribe(*SubscribeRequest, grpc.ServerStreamingServer[Sample]) error
	Publish(context.Context, *PublishRequest) (*PublishAck, error)
	StreamPublish(grpc.ClientStreamingServer[PublishRequest, PublishAck]) error
}

// ── Service descriptor ────────────────────────────────────────────────────────

var _serviceDesc = grpc.ServiceDesc{
	ServiceName: "dds.bridge.v1.DDSBridge",
	HandlerType: (*DDSBridgeServer)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "Publish",
			Handler:    _publishHandler,
		},
	},
	Streams: []grpc.StreamDesc{
		{
			StreamName:    "Subscribe",
			Handler:       _subscribeHandler,
			ServerStreams: true,
		},
		{
			StreamName:    "StreamPublish",
			Handler:       _streamPublishHandler,
			ClientStreams: true,
		},
	},
}

func _publishHandler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(PublishRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		ack, err := srv.(DDSBridgeServer).Publish(ctx, in) //nolint:errcheck // error IS returned
		return ack, err
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/dds.bridge.v1.DDSBridge/Publish"}
	return interceptor(ctx, in, info, func(ctx context.Context, req interface{}) (interface{}, error) {
		pub, ok := req.(*PublishRequest)
		if !ok {
			return nil, fmt.Errorf("bridge/grpc: unexpected request type %T", req)
		}
		return srv.(DDSBridgeServer).Publish(ctx, pub) //nolint:errcheck // error IS returned
	})
}

func _subscribeHandler(srv interface{}, stream grpc.ServerStream) error {
	req := new(SubscribeRequest)
	if err := stream.RecvMsg(req); err != nil {
		return err
	}
	err := srv.(DDSBridgeServer).Subscribe(req, &subscribeServer{stream}) //nolint:errcheck // error IS returned
	return err
}

type subscribeServer struct{ grpc.ServerStream }

func (s *subscribeServer) Send(m *Sample) error { return s.SendMsg(m) }

func _streamPublishHandler(srv interface{}, stream grpc.ServerStream) error {
	err := srv.(DDSBridgeServer).StreamPublish(&streamPublishServer{stream}) //nolint:errcheck // error IS returned
	return err
}

type streamPublishServer struct{ grpc.ServerStream }

func (s *streamPublishServer) Recv() (*PublishRequest, error) {
	m := new(PublishRequest)
	if err := s.RecvMsg(m); err != nil {
		return nil, err
	}
	return m, nil
}

func (s *streamPublishServer) SendAndClose(ack *PublishAck) error {
	return s.SendMsg(ack)
}

// RegisterDDSBridgeServer registers srv on s.
func RegisterDDSBridgeServer(s *grpc.Server, srv DDSBridgeServer) {
	s.RegisterService(&_serviceDesc, srv)
}

// ── Client ────────────────────────────────────────────────────────────────────

// NewClient returns a DDSBridgeClient that speaks to addr using JSON encoding.
// The returned conn must be closed by the caller when done.
func NewClient(ctx context.Context, addr string, extra ...grpc.DialOption) (DDSBridgeClient, *grpc.ClientConn, error) {
	opts := append([]grpc.DialOption{
		grpc.WithDefaultCallOptions(grpc.ForceCodec(JSONCodec{})),
	}, extra...)
	// grpc.NewClient replaces the deprecated grpc.DialContext (SA1019). It
	// creates a lazy connection; per-RPC deadlines use the caller's context.
	_ = ctx
	conn, err := grpc.NewClient(addr, opts...)
	if err != nil {
		return nil, nil, err
	}
	return &ddsClient{conn}, conn, nil
}

// NewRawClient wraps an existing *grpc.ClientConn as a DDSBridgeClient.
// The caller is responsible for closing the connection.
func NewRawClient(cc grpc.ClientConnInterface) DDSBridgeClient {
	return &ddsClient{cc}
}

// DDSBridgeClient is the client-side interface for DDSBridge.
type DDSBridgeClient interface {
	Subscribe(ctx context.Context, req *SubscribeRequest, opts ...grpc.CallOption) (grpc.ServerStreamingClient[Sample], error)
	Publish(ctx context.Context, req *PublishRequest, opts ...grpc.CallOption) (*PublishAck, error)
	StreamPublish(ctx context.Context, opts ...grpc.CallOption) (grpc.ClientStreamingClient[PublishRequest, PublishAck], error)
}

type ddsClient struct{ cc grpc.ClientConnInterface }

func (c *ddsClient) Subscribe(ctx context.Context, req *SubscribeRequest, opts ...grpc.CallOption) (grpc.ServerStreamingClient[Sample], error) {
	stream, err := c.cc.NewStream(ctx, &grpc.StreamDesc{ServerStreams: true},
		"/dds.bridge.v1.DDSBridge/Subscribe", opts...)
	if err != nil {
		return nil, err
	}
	x := &subscribeClientStream{stream}
	if err := stream.SendMsg(req); err != nil {
		return nil, err
	}
	if err := stream.CloseSend(); err != nil {
		return nil, err
	}
	return x, nil
}

type subscribeClientStream struct{ grpc.ClientStream }

func (s *subscribeClientStream) Recv() (*Sample, error) {
	m := new(Sample)
	if err := s.RecvMsg(m); err != nil {
		return nil, err
	}
	return m, nil
}

func (c *ddsClient) Publish(ctx context.Context, req *PublishRequest, opts ...grpc.CallOption) (*PublishAck, error) {
	out := new(PublishAck)
	err := c.cc.Invoke(ctx, "/dds.bridge.v1.DDSBridge/Publish", req, out, opts...)
	return out, err
}

func (c *ddsClient) StreamPublish(ctx context.Context, opts ...grpc.CallOption) (grpc.ClientStreamingClient[PublishRequest, PublishAck], error) {
	stream, err := c.cc.NewStream(ctx, &grpc.StreamDesc{ClientStreams: true},
		"/dds.bridge.v1.DDSBridge/StreamPublish", opts...)
	if err != nil {
		return nil, err
	}
	return &streamPublishClientStream{stream}, nil
}

type streamPublishClientStream struct{ grpc.ClientStream }

func (s *streamPublishClientStream) Send(req *PublishRequest) error {
	return s.SendMsg(req)
}

func (s *streamPublishClientStream) CloseAndRecv() (*PublishAck, error) {
	if err := s.CloseSend(); err != nil {
		return nil, err
	}
	ack := new(PublishAck)
	if err := s.RecvMsg(ack); err != nil {
		return nil, err
	}
	return ack, nil
}

// ── Bridge implementation ─────────────────────────────────────────────────────

// FilterFunc decides whether to forward a sample. Return false to drop it.
type FilterFunc func(topic string, payload []byte) bool

// TransformFunc rewrites a sample payload before forwarding. Return the new
// payload or an error to drop the sample.
type TransformFunc func(topic string, payload []byte) ([]byte, error)

// Options configures a Bridge.
type Options struct {
	// AuthToken, if non-empty, requires every RPC to carry the metadata key
	//   authorization: Bearer <AuthToken>
	AuthToken string

	// QoS is applied to all subscribers and publishers created by the bridge.
	QoS dds.QoS

	// Filter, if non-nil, is called for each outbound sample (Subscribe path).
	// Return false to drop the sample.
	Filter FilterFunc

	// Transform, if non-nil, rewrites the payload of each outbound sample
	// (Subscribe path) before delivery to the gRPC client.
	Transform TransformFunc
}

func (o Options) qos() dds.QoS {
	// dds.QoS gained a []string field (Partition) in Milestone 14 "QoS
	// Enforcement — Active Policy", making it non-comparable with ==;
	// reflect.DeepEqual is the equivalent zero-value check.
	if reflect.DeepEqual(o.QoS, dds.QoS{}) {
		return dds.DefaultQoS
	}
	return o.QoS
}

// Bridge implements DDSBridgeServer and manages lazy subscribers/publishers.
type Bridge struct {
	p    dds.Participant
	opts Options
	srv  *grpc.Server

	mu   sync.Mutex
	subs map[string]dds.Subscriber
	pubs map[string]dds.Publisher
}

// New returns a Bridge wrapping p. Call Server().Serve(lis) to start accepting
// gRPC connections.
func New(p dds.Participant, opts Options) *Bridge {
	var srvOpts []grpc.ServerOption
	if opts.AuthToken != "" {
		srvOpts = append(srvOpts,
			grpc.UnaryInterceptor(authUnary(opts.AuthToken)),
			grpc.StreamInterceptor(authStream(opts.AuthToken)),
		)
	}
	srv := grpc.NewServer(srvOpts...)

	b := &Bridge{
		p:    p,
		opts: opts,
		srv:  srv,
		subs: make(map[string]dds.Subscriber),
		pubs: make(map[string]dds.Publisher),
	}
	RegisterDDSBridgeServer(srv, b)
	return b
}

// Server returns the underlying grpc.Server. Use it to call Serve(lis).
func (b *Bridge) Server() *grpc.Server { return b.srv }

// Close closes all open subscribers, publishers, and the gRPC server.
func (b *Bridge) Close() error {
	b.srv.GracefulStop()
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, sub := range b.subs {
		_ = sub.Close()
	}
	for _, pub := range b.pubs {
		_ = pub.Close()
	}
	b.subs = make(map[string]dds.Subscriber)
	b.pubs = make(map[string]dds.Publisher)
	return nil
}

// Subscribe implements DDSBridgeServer.
func (b *Bridge) Subscribe(req *SubscribeRequest, stream grpc.ServerStreamingServer[Sample]) error {
	if req.Topic == "" {
		return status.Error(codes.InvalidArgument, "topic must not be empty")
	}
	sub, err := b.getOrCreateSub(req.Topic)
	if err != nil {
		return status.Errorf(codes.Internal, "subscriber: %v", err)
	}
	ctx := stream.Context()
	for {
		select {
		case s, ok := <-sub.C():
			if !ok {
				return nil
			}
			payload := s.Payload
			if b.opts.Filter != nil && !b.opts.Filter(req.Topic, payload) {
				continue
			}
			if b.opts.Transform != nil {
				payload, err = b.opts.Transform(req.Topic, payload)
				if err != nil {
					continue
				}
			}
			msg := &Sample{
				Topic:          req.Topic,
				Payload:        payload,
				SequenceNumber: s.SequenceNumber,
				TimestampNs:    s.Timestamp.UnixNano(),
				WriterGUID:     s.WriterGUID[:],
			}
			if err := stream.Send(msg); err != nil {
				return err
			}
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// Publish implements DDSBridgeServer.
func (b *Bridge) Publish(ctx context.Context, req *PublishRequest) (*PublishAck, error) {
	if req.Topic == "" {
		return nil, status.Error(codes.InvalidArgument, "topic must not be empty")
	}
	pub, err := b.getOrCreatePub(req.Topic)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "publisher: %v", err)
	}
	if err := pub.WriteCtx(ctx, req.Payload); err != nil {
		return nil, status.Errorf(codes.Internal, "write: %v", err)
	}
	return &PublishAck{Count: 1}, nil
}

// StreamPublish implements DDSBridgeServer.
func (b *Bridge) StreamPublish(stream grpc.ClientStreamingServer[PublishRequest, PublishAck]) error {
	var count uint32
	for {
		req, err := stream.Recv()
		if err != nil {
			// io.EOF signals normal stream close
			if isEOF(err) {
				return stream.SendAndClose(&PublishAck{Count: count})
			}
			return err
		}
		if req.Topic == "" {
			return status.Error(codes.InvalidArgument, "topic must not be empty")
		}
		pub, err := b.getOrCreatePub(req.Topic)
		if err != nil {
			return status.Errorf(codes.Internal, "publisher: %v", err)
		}
		if err := pub.WriteCtx(stream.Context(), req.Payload); err != nil {
			return status.Errorf(codes.Internal, "write: %v", err)
		}
		count++
	}
}

func (b *Bridge) getOrCreateSub(topic string) (dds.Subscriber, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if sub, ok := b.subs[topic]; ok {
		return sub, nil
	}
	sub, err := b.p.NewSubscriber(topic, b.opts.qos())
	if err != nil {
		return nil, err
	}
	b.subs[topic] = sub
	return sub, nil
}

func (b *Bridge) getOrCreatePub(topic string) (dds.Publisher, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if pub, ok := b.pubs[topic]; ok {
		return pub, nil
	}
	pub, err := b.p.NewPublisher(topic, b.opts.qos())
	if err != nil {
		return nil, err
	}
	b.pubs[topic] = pub
	return pub, nil
}

// ── Auth interceptors ─────────────────────────────────────────────────────────

func authUnary(token string) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		if err := checkAuth(ctx, token); err != nil {
			return nil, err
		}
		return handler(ctx, req)
	}
}

func authStream(token string) grpc.StreamServerInterceptor {
	return func(srv interface{}, ss grpc.ServerStream, _ *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		if err := checkAuth(ss.Context(), token); err != nil {
			return err
		}
		return handler(srv, ss)
	}
}

func checkAuth(ctx context.Context, token string) error {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return status.Error(codes.Unauthenticated, "missing metadata")
	}
	vals := md.Get("authorization")
	if len(vals) == 0 || vals[0] != "Bearer "+token {
		return status.Error(codes.Unauthenticated, "invalid token")
	}
	return nil
}

func isEOF(err error) bool { return err == io.EOF }
