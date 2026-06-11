// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package wan

import (
	"errors"
	"net"
	"testing"
	"time"

	dds "github.com/SoundMatt/go-DDS"
	"github.com/SoundMatt/go-DDS/mock"
)

// failFirstWrite fails on the first Write call, mimicking a broken header send.
type failFirstWrite struct{ calls int }

func (w *failFirstWrite) Write(b []byte) (int, error) {
	w.calls++
	if w.calls == 1 {
		return 0, errors.New("injected write error")
	}
	return len(b), nil
}

func TestWriteFrame_HeaderWriteError(t *testing.T) {
	err := writeFrame(&failFirstWrite{}, &wireFrame{Topic: "x", Payload: []byte("y")})
	if err == nil {
		t.Fatal("expected header write error from writeFrame")
	}
}

func newMockPart(t *testing.T) dds.Participant {
	t.Helper()
	p, err := mock.New(0, mock.IsolatedBroker())
	if err != nil {
		t.Fatalf("mock.New: %v", err)
	}
	t.Cleanup(func() { _ = p.Close() })
	return p
}

// TestSendLoop_WriteFrameError covers the writeFrame error path in sendLoop.
// net.Pipe() gives a synchronous connection: writing to client after server is
// closed returns an error immediately, reliably triggering closeAllSubs+return.
func TestSendLoop_WriteFrameError(t *testing.T) {
	p := newMockPart(t)
	topic := "sendloop/write-err"

	sub, err := p.NewSubscriber(topic, dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewSubscriber: %v", err)
	}

	client, server := net.Pipe()
	server.Close() // client writes fail immediately

	b := &Bridge{
		p:     p,
		done:  make(chan struct{}),
		conns: make(map[net.Conn]struct{}),
	}
	b.addConn(client)

	scs := []subChan{{sub: sub, ch: sub.C(), topic: topic}}
	b.wg.Add(1)
	go b.sendLoop(client, scs)

	pub, err := p.NewPublisher(topic, dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
	defer pub.Close()
	_ = pub.Write([]byte("trigger"))

	// sendLoop exits when writeFrame fails; wg reaches zero.
	done := make(chan struct{})
	go func() { b.wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("sendLoop did not exit after writeFrame error")
	}
}

// TestSendLoop_SubscriberChannelClosed covers the !ok branch in sendLoop: the
// subscriber channel is closed while sendLoop is blocking on the select.
func TestSendLoop_SubscriberChannelClosed(t *testing.T) {
	p := newMockPart(t)
	topic := "sendloop/sub-closed"

	sub, err := p.NewSubscriber(topic, dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewSubscriber: %v", err)
	}

	client, server := net.Pipe()
	defer server.Close()

	b := &Bridge{
		p:     p,
		done:  make(chan struct{}),
		conns: make(map[net.Conn]struct{}),
	}
	b.addConn(client)

	scs := []subChan{{sub: sub, ch: sub.C(), topic: topic}}
	b.wg.Add(1)
	go b.sendLoop(client, scs)

	// Close the subscriber after a brief pause — this closes sc.ch, triggering !ok.
	go func() {
		time.Sleep(20 * time.Millisecond)
		_ = sub.Close()
	}()

	done := make(chan struct{})
	go func() { b.wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("sendLoop did not exit after subscriber channel closed")
	}
}
