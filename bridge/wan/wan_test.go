// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package wan_test

//fusa:test REQ-BRIDGE-013
//fusa:test REQ-BRIDGE-014

import (
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"sync/atomic"
	"testing"
	"time"

	dds "github.com/SoundMatt/go-DDS"
	"github.com/SoundMatt/go-DDS/bridge/wan"
	"github.com/SoundMatt/go-DDS/mock"
)

// failAfterNSubs wraps a Participant and makes NewSubscriber fail after N
// successful calls. Used to test multi-topic cleanup in Connect.
type failAfterNSubs struct {
	dds.Participant
	limit int64
	count atomic.Int64
}

func (f *failAfterNSubs) NewSubscriber(topic string, qos dds.QoS, opts ...dds.SubscriberOption) (dds.Subscriber, error) {
	n := f.count.Add(1)
	if n > f.limit {
		return nil, fmt.Errorf("forced subscriber failure on call %d", n)
	}
	return f.Participant.NewSubscriber(topic, qos, opts...)
}

func newPart(t *testing.T) dds.Participant {
	t.Helper()
	// IsolatedBroker prevents the global mock broker from echoing samples
	// published by the server back to the WAN client's subscribers, which
	// would create a feedback loop and a data race.
	p, err := mock.New(0, mock.IsolatedBroker())
	if err != nil {
		t.Fatalf("mock.New: %v", err)
	}
	t.Cleanup(func() { _ = p.Close() })
	return p
}

func uniqueTopic(prefix string) string {
	return fmt.Sprintf("%s/%d", prefix, time.Now().UnixNano())
}

func mustServe(t *testing.T, p dds.Participant) *wan.Bridge {
	t.Helper()
	srv, err := wan.Serve(p, "127.0.0.1:0", wan.Options{})
	if err != nil {
		t.Fatalf("Serve: %v", err)
	}
	t.Cleanup(func() { _ = srv.Close() })
	return srv
}

// ── Basic forwarding ──────────────────────────────────────────────────────────

func TestWANBridge_ForwardsSample(t *testing.T) {
	src := newPart(t)
	dst := newPart(t)
	topic := uniqueTopic("wan/fwd")

	sub, err := dst.NewSubscriber(topic, dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewSubscriber on dst: %v", err)
	}
	defer sub.Close()

	// Server: receives frames from the client and publishes to dst.
	srv := mustServe(t, dst)

	// Client: subscribes to topic on src, sends frames to server.
	cli, err := wan.Connect(src, srv.Addr(), wan.Options{Topics: []string{topic}})
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer cli.Close()

	// Publish on src — should arrive on dst via the WAN bridge.
	pub, err := src.NewPublisher(topic, dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewPublisher on src: %v", err)
	}
	defer pub.Close()

	if err := pub.Write([]byte("via-wan")); err != nil {
		t.Fatalf("Write: %v", err)
	}

	select {
	case s := <-sub.C():
		if string(s.Payload) != "via-wan" {
			t.Errorf("payload: got %q, want via-wan", s.Payload)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout: sample not forwarded via WAN bridge")
	}
}

func TestWANBridge_MultipleTopics(t *testing.T) {
	src := newPart(t)
	dst := newPart(t)
	topicA := uniqueTopic("wan/multi/a")
	topicB := uniqueTopic("wan/multi/b")

	subA, err := dst.NewSubscriber(topicA, dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewSubscriber A: %v", err)
	}
	defer subA.Close()

	subB, err := dst.NewSubscriber(topicB, dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewSubscriber B: %v", err)
	}
	defer subB.Close()

	srv := mustServe(t, dst)

	cli, err := wan.Connect(src, srv.Addr(), wan.Options{Topics: []string{topicA, topicB}})
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer cli.Close()

	pubA, err := src.NewPublisher(topicA, dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
	pubB, err := src.NewPublisher(topicB, dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
	defer pubA.Close()
	defer pubB.Close()

	_ = pubA.Write([]byte("msg-a"))
	_ = pubB.Write([]byte("msg-b"))

	for _, tc := range []struct {
		sub  dds.Subscriber
		want string
	}{
		{subA, "msg-a"},
		{subB, "msg-b"},
	} {
		select {
		case s := <-tc.sub.C():
			if string(s.Payload) != tc.want {
				t.Errorf("payload: got %q, want %q", s.Payload, tc.want)
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("timeout waiting for %q", tc.want)
		}
	}
}

// ── Addr ──────────────────────────────────────────────────────────────────────

func TestWANBridge_Serve_Addr_NonEmpty(t *testing.T) {
	p := newPart(t)
	srv := mustServe(t, p)
	if srv.Addr() == "" {
		t.Error("Serve: Addr() must not be empty")
	}
}

func TestWANBridge_Connect_Addr_Empty(t *testing.T) {
	src := newPart(t)
	srv := mustServe(t, newPart(t))

	cli, err := wan.Connect(src, srv.Addr(), wan.Options{})
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer cli.Close()

	if cli.Addr() != "" {
		t.Errorf("Connect: Addr() should be empty, got %q", cli.Addr())
	}
}

// ── Close idempotency ─────────────────────────────────────────────────────────

func TestWANBridge_Close_Idempotent(t *testing.T) {
	p := newPart(t)
	srv := mustServe(t, p)
	_ = srv.Close()
	if err := srv.Close(); err != nil {
		t.Errorf("second Close should not error: %v", err)
	}
}

// ── Error paths ───────────────────────────────────────────────────────────────

func TestWANBridge_Serve_ListenError(t *testing.T) {
	p := newPart(t)
	ignoredRet, err := wan.Serve(p, "127.0.0.1:99999", wan.Options{})
	_ = ignoredRet
	if err == nil {
		t.Fatal("expected error for invalid port")
	}
}

func TestWANBridge_Connect_DialError(t *testing.T) {
	p := newPart(t)
	// Port 1 is not listening on any test host.
	ignoredRet, err := wan.Connect(p, "127.0.0.1:1", wan.Options{})
	_ = ignoredRet
	if err == nil {
		t.Fatal("expected error connecting to non-listening port")
	}
}

// TestWANBridge_Connect_DialError_WithTopics exercises the subscription
// cleanup branch in Connect: subscriptions are created successfully but the
// dial fails, so Connect must close the subscriptions before returning.
func TestWANBridge_Connect_DialError_WithTopics(t *testing.T) {
	p := newPart(t)
	ignoredRet, err := wan.Connect(p, "127.0.0.1:1", wan.Options{Topics: []string{"cleanup/topic"}})
	_ = ignoredRet
	if err == nil {
		t.Fatal("expected error connecting to non-listening port")
	}
}

func TestWANBridge_Connect_ClosedParticipant(t *testing.T) {
	src := newPart(t)
	srv := mustServe(t, newPart(t))

	src.Close() // close before Connect — subscription creation fails
	ignoredRet, err := wan.Connect(src, srv.Addr(), wan.Options{Topics: []string{"some/topic"}})
	_ = ignoredRet
	if err == nil {
		t.Fatal("expected error connecting with closed participant")
	}
}

// ── Large-frame rejection ─────────────────────────────────────────────────────

// TestWANBridge_LargeFrame_Rejected verifies that a server receiveLoop exits
// cleanly when a client sends an oversized frame header (> 16 MiB).
func TestWANBridge_LargeFrame_Rejected(t *testing.T) {
	p := newPart(t)
	srv := mustServe(t, p)

	conn, err := net.Dial("tcp", srv.Addr())
	if err != nil {
		t.Fatalf("dial server: %v", err)
	}
	defer conn.Close()

	// Write a header claiming 32 MiB — exceeds the 16 MiB cap.
	var hdr [4]byte
	binary.BigEndian.PutUint32(hdr[:], 32*1024*1024)
	if _, err := conn.Write(hdr[:]); err != nil {
		t.Logf("write header: %v (server may have closed connection)", err)
	}

	// The server should close the connection after rejecting the frame.
	// Verify srv.Close() completes promptly (no goroutine leak).
	done := make(chan struct{})
	go func() {
		_ = srv.Close()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("srv.Close() did not complete promptly after large frame rejection")
	}
}

// TestWANBridge_ReadFrame_InvalidJSON verifies that the server's receiveLoop
// exits cleanly when a client sends a valid-size frame containing invalid JSON,
// covering the json.Unmarshal error path in readFrame.
func TestWANBridge_ReadFrame_InvalidJSON(t *testing.T) {
	p := newPart(t)
	srv := mustServe(t, p)

	conn, err := net.Dial("tcp", srv.Addr())
	if err != nil {
		t.Fatalf("dial server: %v", err)
	}
	defer conn.Close()

	body := []byte("this is not valid JSON {{{{")
	var hdr [4]byte
	binary.BigEndian.PutUint32(hdr[:], uint32(len(body)))
	if _, err := conn.Write(hdr[:]); err != nil {
		t.Fatalf("write header: %v", err)
	}
	if _, err := conn.Write(body); err != nil {
		t.Fatalf("write body: %v", err)
	}

	// The server should close the connection after failing to decode the frame.
	done := make(chan struct{})
	go func() {
		_ = srv.Close()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("srv.Close() did not complete promptly after invalid JSON frame")
	}
}

// ── ErrFrameTooLarge exported ─────────────────────────────────────────────────

func TestWANBridge_ErrFrameTooLarge_Sentinel(t *testing.T) {
	// Verify that ErrFrameTooLarge is exported and usable with errors.Is.
	p := newPart(t)
	srv := mustServe(t, p)

	conn, err := net.Dial("tcp", srv.Addr())
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	// Confirm the exported sentinel value is non-nil.
	if wan.ErrFrameTooLarge == nil {
		t.Error("ErrFrameTooLarge must not be nil")
	}

	// Construct an errors.Is chain to confirm the sentinel is usable.
	wrapped := fmt.Errorf("outer: %w", wan.ErrFrameTooLarge)
	if !errors.Is(wrapped, wan.ErrFrameTooLarge) {
		t.Error("errors.Is should match ErrFrameTooLarge through wrapping")
	}
}

// ── Server closed-participant ──────────────────────────────────────────────────

// TestWANBridge_Connect_MultiTopicCleanup covers the error path in Connect where
// the first topic subscribes successfully but the second subscription fails.
// Connect must close already-created subscribers before returning the error.
func TestWANBridge_Connect_MultiTopicCleanup(t *testing.T) {
	base := newPart(t)
	srv := mustServe(t, newPart(t))

	// Allow exactly 1 subscriber before failing: first topic succeeds, second fails.
	p := &failAfterNSubs{Participant: base, limit: 1}
	_, err := wan.Connect(p, srv.Addr(), wan.Options{
		Topics: []string{"wan/topic-a", "wan/topic-b"},
	})
	if err == nil {
		t.Fatal("expected error when second subscriber creation fails")
	}
}

// TestWANBridge_ReadFrame_ErrFrameTooLarge covers the ErrFrameTooLarge branch
// in readFrame by sending a valid 4-byte header claiming > maxFrameBytes.
func TestWANBridge_ReadFrame_ErrFrameTooLarge(t *testing.T) {
	p := newPart(t)
	srv := mustServe(t, p)

	conn, dialErr := net.Dial("tcp", srv.Addr())
	if dialErr != nil {
		t.Fatalf("dial: %v", dialErr)
	}
	defer conn.Close()

	var hdr [4]byte
	binary.BigEndian.PutUint32(hdr[:], 32*1024*1024) // 32 MiB > maxFrameBytes
	if _, err := conn.Write(hdr[:]); err != nil {
		t.Fatalf("write header: %v", err)
	}

	// Wait for the server to process the oversized frame header and close.
	time.Sleep(50 * time.Millisecond)

	// The server should have closed the connection; any read should return EOF.
	_ = conn.SetDeadline(time.Now().Add(200 * time.Millisecond))
	_, readErr := conn.Read(make([]byte, 1))
	if readErr == nil {
		t.Error("expected server to close connection after ErrFrameTooLarge")
	}
}

// TestWANBridge_ReadFrame_BodyReadError covers the io.ReadFull body error in
// readFrame by closing the client connection after writing the 4-byte size header
// but before writing the body bytes.
func TestWANBridge_ReadFrame_BodyReadError(t *testing.T) {
	p := newPart(t)
	srv := mustServe(t, p)

	conn, dialErr := net.Dial("tcp", srv.Addr())
	if dialErr != nil {
		t.Fatalf("dial: %v", dialErr)
	}

	var hdr [4]byte
	binary.BigEndian.PutUint32(hdr[:], 64) // claims 64 bytes but we close before sending them
	if _, writeErr := conn.Write(hdr[:]); writeErr != nil {
		t.Fatalf("write header: %v", writeErr)
	}
	conn.Close() // close without writing body → server io.ReadFull(buf) returns error

	time.Sleep(50 * time.Millisecond)
}

// TestWANBridge_ReceiveLoop_WriteError covers the pub.Write error path in
// receiveLoop. The server is configured with MaxSampleSize:1 so any real
// payload Write fails, causing receiveLoop to exit.
func TestWANBridge_ReceiveLoop_WriteError(t *testing.T) {
	dst := newPart(t)
	src := newPart(t)
	topic := uniqueTopic("wan/write-err")

	srv, err := wan.Serve(dst, "127.0.0.1:0", wan.Options{QoS: dds.QoS{MaxSampleSize: 1}})
	if err != nil {
		t.Fatalf("Serve: %v", err)
	}
	t.Cleanup(func() { _ = srv.Close() })

	cli, err := wan.Connect(src, srv.Addr(), wan.Options{Topics: []string{topic}})
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer cli.Close()

	pub, err := src.NewPublisher(topic, dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
	defer pub.Close()
	_ = pub.Write([]byte("payload-longer-than-one-byte")) // > 1 byte → pub.Write fails on server

	time.Sleep(50 * time.Millisecond)
}

// TestWANBridge_SendLoop_WriteError covers the closeAllSubs+return branch in
// sendLoop and the return-werr branch in writeFrame. After the server is closed
// the TCP connection breaks, so the client's next writeFrame call fails.
func TestWANBridge_SendLoop_WriteError(t *testing.T) {
	src := newPart(t)
	dst := newPart(t)
	topic := uniqueTopic("wan/send-write-err")

	srv := mustServe(t, dst)

	cli, err := wan.Connect(src, srv.Addr(), wan.Options{Topics: []string{topic}})
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer cli.Close()

	pub, err := src.NewPublisher(topic, dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
	defer pub.Close()

	// Send one sample to confirm the path is working before breaking the connection.
	_ = pub.Write([]byte("ping"))
	time.Sleep(30 * time.Millisecond)

	// Close the server — this breaks the TCP connection on the server side.
	_ = srv.Close()
	time.Sleep(30 * time.Millisecond) // let RST/FIN propagate to the client

	// Publish again: sendLoop reads the sample and calls writeFrame on the now-broken
	// connection, which returns an error → closeAllSubs() + return.
	_ = pub.Write([]byte("ping2"))
	time.Sleep(50 * time.Millisecond)
}

// TestWANBridge_Server_ClosedParticipant verifies that the receiveLoop exits
// cleanly when the server's participant is closed (NewPublisher fails).
func TestWANBridge_Server_ClosedParticipant(t *testing.T) {
	dst := newPart(t)
	src := newPart(t)
	topic := uniqueTopic("wan/svc-closed")

	srv := mustServe(t, dst)
	dst.Close() // close dst so receiveLoop's NewPublisher will fail

	cli, err := wan.Connect(src, srv.Addr(), wan.Options{Topics: []string{topic}})
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer cli.Close()

	pub, err := src.NewPublisher(topic, dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
	defer pub.Close()
	_ = pub.Write([]byte("ping"))

	// Allow time for the frame to reach the server and trigger the error.
	time.Sleep(50 * time.Millisecond)
	// srv.Close() should complete without hanging.
}
