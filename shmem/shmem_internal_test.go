// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Internal tests for the cross-process shmem path.
// These tests access unexported types (shmListener, shmBroker, shmPublish)
// that are not reachable from the external test package.

package shmem

import (
	"context"
	"encoding/binary"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	dds "github.com/SoundMatt/go-DDS"
)

// TestShmListener_ReceivesPublishedData exercises the cross-process delivery
// path: newShmListener → loop → readData → sample delivered to listener.ch.
// This path is not covered by external tests because broker.publish calls
// shmPublish in a goroutine that races with test-cleanup.
func TestShmListener_ReceivesPublishedData(t *testing.T) {
	topic := "internal/xproc-test"
	listener, err := newShmListener(topic, nil, 4)
	if err != nil {
		t.Skipf("newShmListener: %v (socket setup may be unavailable)", err)
	}
	defer listener.close()

	// Give the loop goroutine a moment to start and block on Read.
	time.Sleep(10 * time.Millisecond)

	payload := []byte("cross-process-hello")
	// shmPublish writes the data file and sends a socket notification synchronously.
	shmPublish(topic, payload)

	select {
	case s := <-listener.ch:
		if string(s.Payload) != string(payload) {
			t.Errorf("got %q, want %q", s.Payload, payload)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timeout: cross-process sample not delivered via shmListener")
	}
}

// TestShmListener_Filter_DropsNonMatching verifies that the listener's filter
// is applied and non-matching samples are not forwarded to listener.ch.
func TestShmListener_Filter_DropsNonMatching(t *testing.T) {
	topic := "internal/xproc-filter"
	filter := func(s dds.Sample) bool { return string(s.Payload) == "keep" }
	listener, err := newShmListener(topic, filter, 4)
	if err != nil {
		t.Skipf("newShmListener: %v", err)
	}
	defer listener.close()

	time.Sleep(10 * time.Millisecond)

	shmPublish(topic, []byte("drop"))
	time.Sleep(30 * time.Millisecond)

	shmPublish(topic, []byte("keep"))

	select {
	case s := <-listener.ch:
		if string(s.Payload) != "keep" {
			t.Errorf("filter: got %q, want keep", s.Payload)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timeout: filtered cross-process sample not delivered")
	}
}

// TestCloseWithDrain_ShmemParticipant verifies that the shmem participant's
// CloseWithDrain method is callable and returns nil.
func TestCloseWithDrain_ShmemParticipant(t *testing.T) {
	p, err := New(0)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	part, ok := p.(*participant)
	if !ok {
		t.Fatalf("expected *participant, got %T", p)
	}
	if err := part.CloseWithDrain(context.Background()); err != nil {
		t.Fatalf("CloseWithDrain: %v", err)
	}
}

// TestPump_CrossProcessPath exercises the xpCh branch of shmSubscriber.pump
// by wiring a shmSubscriber with a listener and triggering delivery through
// the listener channel rather than through the in-process broker.
func TestPump_CrossProcessPath(t *testing.T) {
	topic := "internal/pump-xp"
	listener, err := newShmListener(topic, nil, 4)
	if err != nil {
		t.Skipf("newShmListener: %v", err)
	}

	// Build a subscriber wired to the listener but with an inProc channel that
	// receives nothing, so the only delivery path is the cross-process xpCh.
	inProc := make(chan dds.Sample, 4)
	b := brokerFor(dds.Domain(199))
	sub := &shmSubscriber{
		topic:    topic,
		broker:   b,
		inProc:   inProc,
		listener: listener,
		ch:       make(chan dds.Sample, 4),
		done:     make(chan struct{}),
	}
	// Start pump (normally called lazily from C()).
	sub.pump()

	time.Sleep(10 * time.Millisecond)

	// Publish via the socket path so the listener goroutine picks it up.
	shmPublish(topic, []byte("xp-payload"))

	select {
	case s := <-sub.ch:
		if string(s.Payload) != "xp-payload" {
			t.Errorf("got %q, want xp-payload", s.Payload)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timeout: cross-process pump sample not delivered")
	}

	// Close sub so the pump goroutine exits; close listener to release socket.
	close(sub.done)
	listener.close()
}

// TestPump_CrossProcessChannel_Closed verifies the pump sets xpCh to nil
// when the listener channel closes (the !ok branch).
func TestPump_CrossProcessChannel_Closed(t *testing.T) {
	topic := "internal/pump-xp-close"
	listener, err := newShmListener(topic, nil, 4)
	if err != nil {
		t.Skipf("newShmListener: %v", err)
	}

	inProc := make(chan dds.Sample, 4)
	b := brokerFor(dds.Domain(198))
	sub := &shmSubscriber{
		topic:    topic,
		broker:   b,
		inProc:   inProc,
		listener: listener,
		ch:       make(chan dds.Sample, 4),
		done:     make(chan struct{}),
	}
	sub.pump()
	time.Sleep(10 * time.Millisecond)

	// Close listener — this closes listener.ch, triggering the !ok branch.
	listener.close()
	time.Sleep(50 * time.Millisecond)

	// Pump should still be running (inProc still open). Send via inProc.
	inProc <- dds.Sample{Topic: topic, Payload: []byte("inproc-after-xp-close")}
	select {
	case s := <-sub.ch:
		if string(s.Payload) != "inproc-after-xp-close" {
			t.Errorf("got %q after xpCh close", s.Payload)
		}
	case <-time.After(300 * time.Millisecond):
		t.Fatal("timeout: inProc sample not delivered after xpCh close")
	}
	close(sub.done)
}

// TestDeliverSub_BlockPolicy exercises the Block back-pressure case in
// deliverSub, which is not covered by external tests.
func TestDeliverSub_BlockPolicy(t *testing.T) {
	b := &shmBroker{}
	ch := make(chan dds.Sample, 2)
	sub := shmSub{ch: ch, backPressure: dds.Block}
	sample := dds.Sample{Topic: "t", Payload: []byte("x")}

	// Block policy: sample is delivered without a default/drop branch.
	b.deliverSub(sub, sample, &shmTopicCounter{})

	select {
	case s := <-ch:
		if string(s.Payload) != "x" {
			t.Errorf("got %q", s.Payload)
		}
	default:
		t.Error("expected sample in channel")
	}
}

// TestDeliverSub_DefaultDrop exercises the drop branch of the default case
// in deliverSub: when sub.ch is full, the sample is counted as dropped.
func TestDeliverSub_DefaultDrop(t *testing.T) {
	b := &shmBroker{}
	ch := make(chan dds.Sample, 1) // depth 1
	sub := shmSub{ch: ch, backPressure: dds.DropNewest}
	sample := dds.Sample{Topic: "t", Payload: []byte("x")}

	// Fill the channel.
	ch <- dds.Sample{Payload: []byte("existing")}

	// deliverSub with a full channel → drop branch.
	b.deliverSub(sub, sample, &shmTopicCounter{})

	if b.drops.Load() != 1 {
		t.Errorf("expected 1 drop, got %d", b.drops.Load())
	}
}

// TestReadData_OversizeRejected verifies that readData returns an error when
// the data file's length header exceeds maxPayload.
func TestReadData_OversizeRejected(t *testing.T) {
	topic := "internal/readdata-oversize"
	dir := shmTopicDir(topic)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	path := filepath.Join(dir, "data.bin")

	// Write a header claiming maxPayload+1 bytes.
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	var hdr [4]byte
	binary.LittleEndian.PutUint32(hdr[:], maxPayload+1)
	writeN, writeErr := f.Write(hdr[:])
	_ = writeN
	if writeErr != nil {
		t.Fatalf("Write: %v", writeErr)
	}
	_ = f.Close()
	defer func() { _ = os.Remove(path) }()

	l := &shmListener{topic: topic}
	ignoredRet, err := l.readData()
	_ = ignoredRet
	if err == nil {
		t.Error("expected error for payload exceeding maxPayload")
	}
}

// TestDeliverSub_ResetDeadline exercises the deadline-reset branch in
// deliverSub: when resetDeadline is non-nil it must be called after delivery.
func TestDeliverSub_ResetDeadline(t *testing.T) {
	b := &shmBroker{}
	ch := make(chan dds.Sample, 2)
	var reset bool
	sub := shmSub{ch: ch, resetDeadline: func() { reset = true }}
	sample := dds.Sample{Topic: "t", Payload: []byte("x")}
	b.deliverSub(sub, sample, &shmTopicCounter{})
	if !reset {
		t.Error("resetDeadline was not called after successful delivery")
	}
}

// TestReadData_FileNotFound verifies that readData returns an error when the
// data file for the topic does not exist (covers the os.Open error path).
func TestReadData_FileNotFound(t *testing.T) {
	l := &shmListener{topic: "internal/readdata-notfound"}
	_, err := l.readData()
	if err == nil {
		t.Error("expected error for non-existent data file")
	}
}

// TestReadData_EmptyFile verifies that readData returns an error when the
// data file exists but has no content (covers the io.ReadFull lenBuf error).
func TestReadData_EmptyFile(t *testing.T) {
	topic := "internal/readdata-empty"
	dir := shmTopicDir(topic)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	path := filepath.Join(dir, "data.bin")
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	_ = f.Close()
	defer func() { _ = os.Remove(path) }()

	l := &shmListener{topic: topic}
	_, err = l.readData()
	if err == nil {
		t.Error("expected error for empty data file (no length header)")
	}
}

// TestLoop_ReadTimeout_Continue covers the conn.Read error (timeout) continue
// path in loop (shmem.go:340-342). When no socket notification arrives before
// the 100ms read deadline, Read returns an error and the loop continues.
func TestLoop_ReadTimeout_Continue(t *testing.T) {
	topic := "internal/loop-timeout"
	listener, err := newShmListener(topic, nil, 4)
	if err != nil {
		t.Skipf("newShmListener: %v", err)
	}
	// Wait longer than the 100ms read deadline so the loop's Read times out at
	// least once, entering the `if err != nil { continue }` branch.
	time.Sleep(150 * time.Millisecond)
	listener.close()
}

// TestLoop_ReadDataError_Continue covers the readData-error continue path in
// loop: send a socket notification without creating a data file so readData
// returns an error and the loop continues rather than delivering a sample.
func TestLoop_ReadDataError_Continue(t *testing.T) {
	topic := "internal/loop-readdata-err"
	// Remove any stale data file so readData fails.
	_ = os.Remove(shmDataPath(topic))

	listener, err := newShmListener(topic, nil, 4)
	if err != nil {
		t.Skipf("newShmListener: %v (socket setup may be unavailable)", err)
	}
	defer listener.close()
	time.Sleep(10 * time.Millisecond)

	// Signal the socket without writing a data file.
	conn, dialErr := net.Dial("unixgram", shmSocketPath(topic))
	if dialErr != nil {
		t.Skipf("dial socket: %v", dialErr)
	}
	_, _ = conn.Write([]byte{0})
	_ = conn.Close()

	// Give the loop goroutine time to process the signal and hit the readData error.
	time.Sleep(50 * time.Millisecond)

	// Nothing must appear in listener.ch because readData failed.
	select {
	case <-listener.ch:
		t.Error("expected no sample when data file is missing")
	default:
	}
}

// TestDeliverSub_DropOldest_FullChannel covers the outer default branch in the
// DropOldest case of deliverSub: channel is full, oldest item is drained, and
// the new sample is re-sent successfully.
func TestDeliverSub_DropOldest_FullChannel(t *testing.T) {
	b := &shmBroker{}
	ch := make(chan dds.Sample, 1)
	sub := shmSub{ch: ch, backPressure: dds.DropOldest}

	// Fill the channel with one item.
	ch <- dds.Sample{Payload: []byte("old")}

	// deliverSub with DropOldest on a full channel: drains "old" and inserts "new".
	b.deliverSub(sub, dds.Sample{Payload: []byte("new")}, &shmTopicCounter{})

	select {
	case s := <-ch:
		if string(s.Payload) != "new" {
			t.Errorf("DropOldest: expected new sample, got %q", s.Payload)
		}
	default:
		t.Error("DropOldest: channel empty after delivery")
	}
	if b.drops.Load() != 1 {
		t.Errorf("DropOldest: expected 1 drop (drained old), got %d", b.drops.Load())
	}
}

// TestLoop_DropOnFull covers the `default: // drop on full` branch in the
// listener loop's final select (shmem.go:353-355). We start a listener with
// depth 1 and publish two samples in quick succession — the second sample
// finds the channel full and is silently dropped.
func TestLoop_DropOnFull(t *testing.T) {
	topic := "internal/loop-drop-full"
	listener, err := newShmListener(topic, nil, 1) // depth 1
	if err != nil {
		t.Skipf("newShmListener: %v", err)
	}
	defer listener.close()
	time.Sleep(10 * time.Millisecond)

	// Publish first sample — loop forwards it to listener.ch (now full).
	shmPublish(topic, []byte("first"))
	time.Sleep(40 * time.Millisecond) // let loop process it

	// Publish second sample — listener.ch is full, loop takes default: drop.
	shmPublish(topic, []byte("second"))
	time.Sleep(40 * time.Millisecond)

	// Only the first sample must be in the channel.
	select {
	case s := <-listener.ch:
		if string(s.Payload) != "first" {
			t.Errorf("expected first sample, got %q", s.Payload)
		}
	default:
		t.Error("expected first sample in channel")
	}
}

// TestNewShmListener_ListenError covers the net.ListenUnixgram error return in
// newShmListener (shmem.go:314-315). By placing a non-empty directory at the
// socket path, os.Remove fails silently and ListenUnixgram can't bind there.
func TestNewShmListener_ListenError(t *testing.T) {
	topic := "internal/listen-err"
	sockPath := shmSocketPath(topic)
	// Ensure parent dir exists.
	if err := os.MkdirAll(filepath.Dir(sockPath), 0o700); err != nil {
		t.Fatalf("MkdirAll parent: %v", err)
	}
	// Create a non-empty directory at sockPath so os.Remove fails (non-empty)
	// and net.ListenUnixgram also fails because a directory is in the way.
	if err := os.MkdirAll(sockPath, 0o700); err != nil {
		t.Fatalf("mkdir at sockPath: %v", err)
	}
	blockFile := filepath.Join(sockPath, "block")
	if err := os.WriteFile(blockFile, []byte("x"), 0o600); err != nil {
		t.Fatalf("write block file: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(sockPath) })

	_, err := newShmListener(topic, nil, 4)
	if err == nil {
		t.Error("expected error when socket path is a directory")
	}
}

// TestShmPublish_CreateError covers the os.Create error return in shmPublish:
// placing a directory at the data.bin path makes os.Create fail and return
// without panicking or writing anything.
func TestShmPublish_CreateError(t *testing.T) {
	topic := "internal/shmPublish-create-err"
	dataPath := shmDataPath(topic)
	dir := filepath.Dir(dataPath)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	// Place a directory where data.bin would be so os.Create fails.
	if err := os.MkdirAll(dataPath, 0o700); err != nil {
		t.Fatalf("making dir at dataPath: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dataPath) })

	// Must not panic.
	shmPublish(topic, []byte("payload"))
}

// TestReadData_TruncatedBody verifies that readData returns an error when the
// file body is shorter than the declared length.
func TestReadData_TruncatedBody(t *testing.T) {
	topic := "internal/readdata-truncated"
	dir := shmTopicDir(topic)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	path := filepath.Join(dir, "data.bin")

	// Header says 100 bytes; write only 10 bytes of body.
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	var hdr [4]byte
	binary.LittleEndian.PutUint32(hdr[:], 100)
	writeN, writeErr := f.Write(hdr[:])
	_ = writeN
	if writeErr != nil {
		t.Fatalf("Write: %v", writeErr)
	}
	writeN, writeErr = f.Write(make([]byte, 10)) // only 10 bytes
	_ = writeN
	if writeErr != nil {
		t.Fatalf("Write: %v", writeErr)
	}
	_ = f.Close()
	defer func() { _ = os.Remove(path) }()

	l := &shmListener{topic: topic}
	ignoredRet, err := l.readData()
	_ = ignoredRet
	if err == nil {
		t.Error("expected error for truncated data body")
	}
}
