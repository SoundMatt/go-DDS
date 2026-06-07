// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Package wan provides a WAN Bridge that forwards DDS samples between two
// Participant domains over a TCP connection (Milestone 10 — Routing: WAN bridge).
//
// A server Bridge receives frames from connected clients and publishes the
// samples to a local Participant. A client Bridge subscribes to configured DDS
// topics on a local Participant and streams the received samples to a server.
//
// Wire format: each frame is a 4-byte big-endian length prefix followed by a
// JSON object {"t":"<topic>","p":"<base64-payload>"}. The 16 MiB frame cap
// prevents unbounded buffer allocation on malformed or malicious input.
//
// For bidirectional bridging, create one server/client pair in each direction.
package wan

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"

	dds "github.com/SoundMatt/go-DDS"
)

const maxFrameBytes uint32 = 16 * 1024 * 1024 // 16 MiB hard limit per frame

// ErrFrameTooLarge is returned by a server when an incoming frame exceeds
// the 16 MiB limit.
var ErrFrameTooLarge = errors.New("wan: frame too large")

// wireFrame is the per-frame JSON payload. json.Marshal encodes []byte as base64.
type wireFrame struct {
	Topic   string `json:"t"`
	Payload []byte `json:"p"`
}

// subChan pairs a [dds.Subscriber] with its channel and topic name.
type subChan struct {
	sub   dds.Subscriber
	ch    <-chan dds.Sample
	topic string
}

// Options configures a WAN [Bridge].
type Options struct {
	// Topics lists the DDS topic names to forward.
	// Only used by client Bridges ([Connect]); server Bridges publish any topic
	// they receive and do not filter.
	Topics []string
	// QoS is applied to all bridged DDS endpoints. Zero value uses dds.DefaultQoS.
	QoS dds.QoS
}

// Bridge is a WAN bridge (server or client side).
// Create a server Bridge with [Serve] and a client Bridge with [Connect].
//
// Bridge is safe for concurrent use from multiple goroutines.
type Bridge struct {
	p    dds.Participant
	opts Options
	ln   net.Listener // server only; nil for client

	mu    sync.Mutex
	conns map[net.Conn]struct{} // active connections

	done chan struct{} // closed by Close() to interrupt sendLoop goroutines
	once sync.Once
	wg   sync.WaitGroup
}

// Serve creates a WAN bridge server that listens on addr.
// Samples received from connected clients are published to p.
// Use [Bridge.Addr] to discover the actual bound address when addr uses port 0.
func Serve(p dds.Participant, addr string, opts Options) (*Bridge, error) {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}
	b := &Bridge{p: p, opts: opts, ln: ln, conns: make(map[net.Conn]struct{}), done: make(chan struct{})}
	b.wg.Add(1)
	go b.acceptLoop()
	return b, nil
}

// Connect creates a WAN bridge client that connects to addr.
// Topic subscriptions are created synchronously before Connect returns, so no
// samples published after Connect returns are missed.
//
// Returns an error if any topic subscription fails (e.g. p is already closed)
// or if the TCP connection cannot be established.
func Connect(p dds.Participant, addr string, opts Options) (*Bridge, error) {
	var scs []subChan
	for _, topic := range opts.Topics {
		sub, err := p.NewSubscriber(topic, opts.QoS)
		if err != nil {
			for _, sc := range scs {
				_ = sc.sub.Close()
			}
			return nil, fmt.Errorf("wan bridge: NewSubscriber(%q): %w", topic, err)
		}
		scs = append(scs, subChan{sub: sub, ch: sub.C(), topic: topic})
	}

	conn, err := net.Dial("tcp", addr)
	if err != nil {
		for _, sc := range scs {
			_ = sc.sub.Close()
		}
		return nil, err
	}

	b := &Bridge{p: p, opts: opts, conns: make(map[net.Conn]struct{}), done: make(chan struct{})}
	b.addConn(conn)
	b.wg.Add(1)
	go b.sendLoop(conn, scs)
	return b, nil
}

// Addr returns the TCP address the server Bridge is listening on.
// Returns an empty string for client Bridges created by [Connect].
func (b *Bridge) Addr() string {
	if b.ln == nil {
		return ""
	}
	return b.ln.Addr().String()
}

// Close stops the bridge and waits for all goroutines to exit.
// Safe to call multiple times.
func (b *Bridge) Close() error {
	b.once.Do(func() {
		close(b.done) // interrupt sendLoop goroutines blocked on subscriber channels
		if b.ln != nil {
			_ = b.ln.Close()
		}
		b.closeAllConns()
	})
	b.wg.Wait()
	return nil
}

func (b *Bridge) addConn(c net.Conn) {
	b.mu.Lock()
	b.conns[c] = struct{}{}
	b.mu.Unlock()
}

func (b *Bridge) removeConn(c net.Conn) {
	b.mu.Lock()
	delete(b.conns, c)
	b.mu.Unlock()
}

func (b *Bridge) closeAllConns() {
	b.mu.Lock()
	for c := range b.conns {
		_ = c.Close()
	}
	b.conns = make(map[net.Conn]struct{})
	b.mu.Unlock()
}

func (b *Bridge) acceptLoop() {
	defer b.wg.Done()
	for {
		conn, err := b.ln.Accept()
		if err != nil {
			return // listener closed by Close()
		}
		b.addConn(conn)
		b.wg.Add(1)
		go b.receiveLoop(conn)
	}
}

func (b *Bridge) receiveLoop(conn net.Conn) {
	defer b.wg.Done()
	defer b.removeConn(conn)
	defer func() { _ = conn.Close() }()

	pubs := make(map[string]dds.Publisher)
	defer func() {
		for _, pub := range pubs {
			_ = pub.Close()
		}
	}()

	for {
		frame, err := readFrame(conn)
		if err != nil {
			return
		}
		pub, ok := pubs[frame.Topic]
		if !ok {
			pub, err = b.p.NewPublisher(frame.Topic, b.opts.QoS)
			if err != nil {
				return
			}
			pubs[frame.Topic] = pub
		}
		if err := pub.Write(frame.Payload); err != nil {
			return
		}
	}
}

func (b *Bridge) sendLoop(conn net.Conn, scs []subChan) {
	defer b.wg.Done()
	defer b.removeConn(conn)
	defer func() { _ = conn.Close() }()

	// closeAllSubs interrupts goroutines blocked on sc.ch by closing each subscriber.
	closeAllSubs := func() {
		for _, sc := range scs {
			_ = sc.sub.Close()
		}
	}
	defer closeAllSubs()

	var mu sync.Mutex // protects concurrent writes to conn
	var innerWg sync.WaitGroup

	for _, sc := range scs {
		innerWg.Add(1)
		go func() {
			defer innerWg.Done()
			for {
				select {
				case s, ok := <-sc.ch:
					if !ok {
						return
					}
					mu.Lock()
					err := writeFrame(conn, &wireFrame{Topic: sc.topic, Payload: s.Payload})
					mu.Unlock()
					if err != nil {
						closeAllSubs()
						return
					}
				case <-b.done:
					return
				}
			}
		}()
	}
	innerWg.Wait()
}

// ── frame encoding ────────────────────────────────────────────────────────────

func writeFrame(w io.Writer, f *wireFrame) error {
	data, err := json.Marshal(f)
	if err != nil {
		return err
	}
	var hdr [4]byte
	binary.BigEndian.PutUint32(hdr[:], uint32(len(data)))
	if _, werr := w.Write(hdr[:]); werr != nil {
		return werr
	}
	_, err = w.Write(data)
	return err
}

func readFrame(r io.Reader) (*wireFrame, error) {
	var hdr [4]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return nil, err
	}
	size := binary.BigEndian.Uint32(hdr[:])
	if size > maxFrameBytes {
		return nil, fmt.Errorf("%w: %d bytes", ErrFrameTooLarge, size)
	}
	buf := make([]byte, size)
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, err
	}
	var frame wireFrame
	if err := json.Unmarshal(buf, &frame); err != nil {
		return nil, fmt.Errorf("wan: decode frame: %w", err)
	}
	return &frame, nil
}
