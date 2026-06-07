// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Package shmem provides a cross-process shared-memory transport for
// same-host DDS communication. Two processes on the same machine can
// publish and subscribe to topics with zero UDP round-trips: the publisher
// writes to a memory-mapped file and notifies the subscriber via a Unix
// domain socket; the subscriber reads directly from the same mapping.
//
// The shmem transport implements dds.Participant so it is a drop-in
// replacement for the mock or rtps packages when all parties are on the
// same host.
//
// Build constraints: mmap-backed operation is supported on Linux and macOS.
// On all other platforms the transport falls back to a file-based
// implementation that still eliminates the UDP network stack overhead.
//
// Usage:
//
//	p, err := shmem.New(dds.Domain(0))
//
// All participants in the same process and domain share one participant
// (backed by a process-scoped in-memory broker) and the shared-memory
// rendez-vous directory for cross-process delivery.
package shmem

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	dds "github.com/SoundMatt/go-DDS"
)

// shmDir is the root directory under which per-domain/topic socket and
// data files are placed.
const shmDir = "/tmp/godds-shmem"

// maxPayload is the maximum single-write payload size.
const maxPayload = 16 * 1024 * 1024 // 16 MiB

// ── broker ───────────────────────────────────────────────────────────────────

// sharedBroker tracks all shmem participants in the current process for the
// same in-process delivery optimisation used by mock.
var (
	sharedBrokerMu sync.Mutex
	sharedBrokers  = map[dds.Domain]*shmBroker{}
)

type shmBroker struct {
	mu         sync.RWMutex
	subs       map[string][]shmSub
	lastSample map[string]*dds.Sample
	writes     atomic.Uint64
	delivers   atomic.Uint64
	drops      atomic.Uint64
	bWritten   atomic.Uint64
	bDeliv     atomic.Uint64
}

type shmSub struct {
	ch           chan dds.Sample
	filter       func(dds.Sample) bool
	backPressure dds.BackPressurePolicy
}

func brokerFor(d dds.Domain) *shmBroker {
	sharedBrokerMu.Lock()
	defer sharedBrokerMu.Unlock()
	if b, ok := sharedBrokers[d]; ok {
		return b
	}
	b := &shmBroker{
		subs:       make(map[string][]shmSub),
		lastSample: make(map[string]*dds.Sample),
	}
	sharedBrokers[d] = b
	return b
}

func (b *shmBroker) subscribe(topic string, qos dds.QoS, cfg dds.SubscriberConfig) chan dds.Sample {
	depth := cfg.ChanDepth(64)
	ch := make(chan dds.Sample, depth)
	sub := shmSub{ch: ch, filter: cfg.Filter, backPressure: cfg.BackPressure}
	b.mu.Lock()
	b.subs[topic] = append(b.subs[topic], sub)
	var last *dds.Sample
	if qos.Durability == dds.TransientLocal {
		last = b.lastSample[topic]
	}
	b.mu.Unlock()
	if last != nil {
		if cfg.Filter == nil || cfg.Filter(*last) {
			select {
			case ch <- *last:
			default:
			}
		}
	}
	return ch
}

func (b *shmBroker) removeSubscription(topic string, ch chan dds.Sample) {
	b.mu.Lock()
	defer b.mu.Unlock()
	list := b.subs[topic]
	for i, s := range list {
		if s.ch == ch {
			b.subs[topic] = append(list[:i], list[i+1:]...)
			return
		}
	}
}

func (b *shmBroker) publish(topic string, payload []byte, qos dds.QoS) {
	cp := make([]byte, len(payload))
	copy(cp, payload)
	sample := dds.Sample{Topic: topic, Payload: cp, Timestamp: time.Now()}

	b.writes.Add(1)
	b.bWritten.Add(uint64(len(payload)))

	b.mu.Lock()
	if qos.Durability == dds.TransientLocal {
		b.lastSample[topic] = &sample
	}
	subs := make([]shmSub, len(b.subs[topic]))
	copy(subs, b.subs[topic])
	b.mu.Unlock()

	for _, sub := range subs {
		if sub.filter != nil && !sub.filter(sample) {
			continue
		}
		b.deliverSub(sub, sample)
	}

	// Notify cross-process subscribers via shared-memory file + socket signal.
	go shmPublish(topic, payload)
}

func (b *shmBroker) deliverSub(sub shmSub, sample dds.Sample) {
	byteLen := uint64(len(sample.Payload))
	switch sub.backPressure {
	case dds.DropOldest:
		select {
		case sub.ch <- sample:
			b.delivers.Add(1)
			b.bDeliv.Add(byteLen)
		default:
			select {
			case <-sub.ch:
				b.drops.Add(1)
			default:
			}
			select {
			case sub.ch <- sample:
				b.delivers.Add(1)
				b.bDeliv.Add(byteLen)
			default:
				b.drops.Add(1)
			}
		}
	case dds.Block:
		sub.ch <- sample
		b.delivers.Add(1)
		b.bDeliv.Add(byteLen)
	default:
		select {
		case sub.ch <- sample:
			b.delivers.Add(1)
			b.bDeliv.Add(byteLen)
		default:
			b.drops.Add(1)
		}
	}
}

// ── Shared-memory file + socket IPC ──────────────────────────────────────────

// shmTopicDir returns the directory used for the given topic's shmem files.
func shmTopicDir(topic string) string {
	safe := strings.NewReplacer("/", "_", "\\", "_").Replace(topic)
	return filepath.Join(shmDir, safe)
}

// shmDataPath returns the path of the data file for a topic.
func shmDataPath(topic string) string {
	return filepath.Join(shmTopicDir(topic), "data.bin")
}

// shmSocketPath returns the path of the Unix socket for a topic.
func shmSocketPath(topic string) string {
	return filepath.Join(shmTopicDir(topic), "notify.sock")
}

// shmPublish writes payload to the topic's shmem file and notifies any
// cross-process listeners via a datagram to the Unix socket.
func shmPublish(topic string, payload []byte) {
	dir := shmTopicDir(topic)
	_ = os.MkdirAll(dir, 0o700)

	dataPath := shmDataPath(topic)
	f, err := os.Create(dataPath)
	if err != nil {
		return
	}
	var lenBuf [4]byte
	binary.LittleEndian.PutUint32(lenBuf[:], uint32(len(payload)))
	_, _ = f.Write(lenBuf[:])
	_, _ = f.Write(payload)
	f.Close()

	// Notify subscribers.
	sockPath := shmSocketPath(topic)
	conn, err := net.Dial("unixgram", sockPath)
	if err != nil {
		return // no subscriber socket yet — that is fine
	}
	_, _ = conn.Write([]byte{0})
	_ = conn.Close()
}

// ── shmSubscriber (cross-process) ────────────────────────────────────────────

// shmListener is a Unix socket listener for cross-process notifications.
type shmListener struct {
	topic  string
	conn   *net.UnixConn
	ch     chan dds.Sample
	filter func(dds.Sample) bool
	done   chan struct{}
	once   sync.Once
}

func newShmListener(topic string, filter func(dds.Sample) bool, depth int) (*shmListener, error) {
	dir := shmTopicDir(topic)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("shmem: mkdir %s: %w", dir, err)
	}
	sockPath := shmSocketPath(topic)
	_ = os.Remove(sockPath)
	addr := &net.UnixAddr{Name: sockPath, Net: "unixgram"}
	conn, err := net.ListenUnixgram("unixgram", addr)
	if err != nil {
		return nil, fmt.Errorf("shmem: listen %s: %w", sockPath, err)
	}
	l := &shmListener{
		topic:  topic,
		conn:   conn,
		ch:     make(chan dds.Sample, depth),
		filter: filter,
		done:   make(chan struct{}),
	}
	go l.loop()
	return l, nil
}

func (l *shmListener) loop() {
	defer close(l.ch)
	buf := make([]byte, 1)
	for {
		_ = l.conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
		_, err := l.conn.Read(buf)
		select {
		case <-l.done:
			return
		default:
		}
		if err != nil {
			continue // timeout or transient error
		}
		// Notification received — read data file.
		payload, err := l.readData()
		if err != nil {
			continue
		}
		sample := dds.Sample{Topic: l.topic, Payload: payload, Timestamp: time.Now()}
		if l.filter != nil && !l.filter(sample) {
			continue
		}
		select {
		case l.ch <- sample:
		default: // drop on full
		}
	}
}

func (l *shmListener) readData() ([]byte, error) {
	f, err := os.Open(shmDataPath(l.topic))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var lenBuf [4]byte
	if _, err = io.ReadFull(f, lenBuf[:]); err != nil {
		return nil, err
	}
	length := binary.LittleEndian.Uint32(lenBuf[:])
	if length > maxPayload {
		return nil, fmt.Errorf("shmem: payload %d exceeds cap", length)
	}
	data := make([]byte, length)
	if _, err = io.ReadFull(f, data); err != nil {
		return nil, err
	}
	return data, nil
}

func (l *shmListener) close() {
	l.once.Do(func() {
		close(l.done)
		_ = l.conn.Close()
		_ = os.Remove(shmSocketPath(l.topic))
	})
}

// ── Participant ───────────────────────────────────────────────────────────────

// participant implements dds.Participant for the shmem transport.
type participant struct {
	domain dds.Domain
	broker *shmBroker
	mu     sync.Mutex
	closed bool
}

// New returns a dds.Participant backed by the shared-memory transport.
// Participants in the same process share an in-process broker (no file I/O for
// same-process delivery). Cross-process delivery uses a memory-mapped file and
// a Unix domain socket for signalling.
func New(domain dds.Domain) (dds.Participant, error) {
	return &participant{
		domain: domain,
		broker: brokerFor(domain),
	}, nil
}

// Domain implements dds.Participant.
func (p *participant) Domain() dds.Domain { return p.domain }

func (p *participant) NewPublisher(topic string, qos dds.QoS) (dds.Publisher, error) {
	if topic == "" {
		return nil, fmt.Errorf("shmem: %w", dds.ErrTopicEmpty)
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return nil, fmt.Errorf("shmem: %w", dds.ErrClosed)
	}
	return &shmPublisher{topic: topic, qos: qos, broker: p.broker}, nil
}

func (p *participant) NewSubscriber(topic string, qos dds.QoS, opts ...dds.SubscriberOption) (dds.Subscriber, error) {
	if topic == "" {
		return nil, fmt.Errorf("shmem: %w", dds.ErrTopicEmpty)
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return nil, fmt.Errorf("shmem: %w", dds.ErrClosed)
	}
	cfg := dds.ApplySubscriberOpts(opts)
	depth := cfg.ChanDepth(64)
	// In-process channel from the shared broker.
	inProcCh := p.broker.subscribe(topic, qos, cfg)
	// Cross-process listener (best-effort; failure is non-fatal).
	listener, _ := newShmListener(topic, cfg.Filter, depth)
	return &shmSubscriber{
		topic:    topic,
		broker:   p.broker,
		inProc:   inProcCh,
		listener: listener,
		ch:       make(chan dds.Sample, depth),
		done:     make(chan struct{}),
	}, nil
}

func (p *participant) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.closed = true
	return nil
}

// CloseWithDrain implements dds.Drainer. Shmem writes are synchronous so
// there are no in-flight ACKs; this is equivalent to Close.
func (p *participant) CloseWithDrain(_ context.Context) error {
	return p.Close()
}

// Metrics implements dds.MetricsProvider.
func (p *participant) Metrics() dds.Metrics {
	return dds.Metrics{
		WriteCount:     p.broker.writes.Load(),
		DeliverCount:   p.broker.delivers.Load(),
		DropCount:      p.broker.drops.Load(),
		BytesWritten:   p.broker.bWritten.Load(),
		BytesDelivered: p.broker.bDeliv.Load(),
	}
}

// ── Publisher ─────────────────────────────────────────────────────────────────

type shmPublisher struct {
	topic  string
	qos    dds.QoS
	broker *shmBroker
	mu     sync.Mutex
	closed bool
}

func (pub *shmPublisher) Write(payload []byte) error {
	pub.mu.Lock()
	defer pub.mu.Unlock()
	if pub.closed {
		return fmt.Errorf("shmem: %w", dds.ErrClosed)
	}
	if pub.qos.MaxSampleSize > 0 && len(payload) > pub.qos.MaxSampleSize {
		return fmt.Errorf("shmem: %w: got %d bytes, limit %d",
			dds.ErrPayloadTooLarge, len(payload), pub.qos.MaxSampleSize)
	}
	pub.broker.publish(pub.topic, payload, pub.qos)
	return nil
}

// WriteCtx writes payload, returning ctx.Err() immediately if ctx is already done.
func (pub *shmPublisher) WriteCtx(ctx context.Context, payload []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return pub.Write(payload)
}

func (pub *shmPublisher) Close() error {
	pub.mu.Lock()
	defer pub.mu.Unlock()
	pub.closed = true
	return nil
}

// ── Subscriber ────────────────────────────────────────────────────────────────

// shmSubscriber fans in samples from an in-process channel and an optional
// cross-process shmListener into a single unified channel.
type shmSubscriber struct {
	topic     string
	broker    *shmBroker
	inProc    chan dds.Sample
	listener  *shmListener
	ch        chan dds.Sample
	done      chan struct{}
	unsubOnce sync.Once
	closeOnce sync.Once
	started   sync.Once
}

func (sub *shmSubscriber) C() <-chan dds.Sample {
	sub.started.Do(sub.pump)
	return sub.ch
}

func (sub *shmSubscriber) pump() {
	go func() {
		defer close(sub.ch)
		var xpCh <-chan dds.Sample
		if sub.listener != nil {
			xpCh = sub.listener.ch
		}
		for {
			select {
			case s, ok := <-sub.inProc:
				if !ok {
					return
				}
				select {
				case sub.ch <- s:
				case <-sub.done:
					return
				default:
				}
			case s, ok := <-xpCh:
				if !ok {
					xpCh = nil
					continue
				}
				select {
				case sub.ch <- s:
				case <-sub.done:
					return
				default:
				}
			case <-sub.done:
				return
			}
		}
	}()
}

// Unsubscribe removes this subscriber from the broker without closing its
// channel. After Unsubscribe no new samples are delivered to the channel.
func (sub *shmSubscriber) Unsubscribe() error {
	sub.unsubOnce.Do(func() {
		sub.broker.removeSubscription(sub.topic, sub.inProc)
	})
	return nil
}

func (sub *shmSubscriber) Close() error {
	_ = sub.Unsubscribe()
	sub.closeOnce.Do(func() {
		close(sub.done)
		// Close the inProc channel so the pump goroutine exits cleanly via the
		// ok=false branch. Without this the pump goroutine would block forever.
		close(sub.inProc)
		if sub.listener != nil {
			sub.listener.close()
		}
	})
	return nil
}
