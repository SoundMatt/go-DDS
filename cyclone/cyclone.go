//go:build cyclone

// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Package cyclone provides a CycloneDDS-backed implementation of the dds
// interfaces via CGo. Build with -tags cyclone and CycloneDDS installed.
//
// Wire format: each DDS sample is a single opaque byte sequence (RawMessage).
// No IDL compiler is required — the type descriptor is constructed directly
// from CycloneDDS CDR serialization opcodes.  The maximum payload size
// defaults to 64 KiB and is configurable via [Options].
package cyclone

/*
#cgo CFLAGS: -I/usr/local/include
#cgo LDFLAGS: -lcyclonedds

#include <stdlib.h>
#include <string.h>
#include <stdint.h>
#include "dds/dds.h"

// RawMessage is the on-wire DDS type: a single fixed-size byte array.
// Using a fixed-size array avoids heap allocation in the C layer and
// produces a simple CDR type descriptor without IDL compilation.
// The size is set at build time; callers use go_dds_payload_max() to
// read it rather than repeating the constant.
#define GO_DDS_PAYLOAD_MAX 65536

typedef struct {
    char data[GO_DDS_PAYLOAD_MAX];
} RawMessage;

// CDR serialization opcodes for RawMessage (single byte-array field, no key).
static const uint32_t RawMessage_ops[] = {
    DDS_OP_ADR | DDS_OP_TYPE_ARR | DDS_OP_SUBTYPE_1BY,
    offsetof(RawMessage, data),
    GO_DDS_PAYLOAD_MAX,
    DDS_OP_RTS
};

static const dds_topic_descriptor_t RawMessage_desc = {
    sizeof(RawMessage),
    1,
    DDS_TOPIC_NO_OPTIMIZE,
    0,
    "RawMessage",
    NULL,
    sizeof(RawMessage_ops) / sizeof(RawMessage_ops[0]),
    RawMessage_ops,
    ""
};

static int32_t go_dds_create_participant(int32_t domain) {
    return (int32_t)dds_create_participant((dds_domainid_t)domain, NULL, NULL);
}

static int32_t go_dds_create_topic(int32_t participant, const char *name) {
    return (int32_t)dds_create_topic(
        (dds_entity_t)participant, &RawMessage_desc, name, NULL, NULL);
}

static int32_t go_dds_create_publisher(int32_t participant) {
    return (int32_t)dds_create_publisher((dds_entity_t)participant, NULL, NULL);
}

static int32_t go_dds_create_writer(int32_t publisher, int32_t topic) {
    return (int32_t)dds_create_writer(
        (dds_entity_t)publisher, (dds_entity_t)topic, NULL, NULL);
}

static int32_t go_dds_create_subscriber(int32_t participant) {
    return (int32_t)dds_create_subscriber((dds_entity_t)participant, NULL, NULL);
}

static int32_t go_dds_create_reader(int32_t subscriber, int32_t topic) {
    return (int32_t)dds_create_reader(
        (dds_entity_t)subscriber, (dds_entity_t)topic, NULL, NULL);
}

// go_dds_write publishes payload (null-terminated string) on writer.
static int32_t go_dds_write(int32_t writer, const char *payload) {
    RawMessage msg;
    memset(&msg, 0, sizeof(msg));
    strncpy(msg.data, payload, GO_DDS_PAYLOAD_MAX - 1);
    return (int32_t)dds_write((dds_entity_t)writer, &msg);
}

// go_dds_take reads one sample from reader into buf (caller-allocated,
// GO_DDS_PAYLOAD_MAX bytes). Returns 1 on success, 0 if no data, <0 on error.
static int32_t go_dds_take(int32_t reader, char *buf) {
    void *samples[1];
    dds_sample_info_t infos[1];
    RawMessage msg;
    samples[0] = &msg;
    int32_t n = (int32_t)dds_take((dds_entity_t)reader, samples, infos, 1, 1);
    if (n > 0 && infos[0].valid_data) {
        strncpy(buf, msg.data, GO_DDS_PAYLOAD_MAX - 1);
        buf[GO_DDS_PAYLOAD_MAX - 1] = '\0';
    }
    return n;
}

static int32_t go_dds_payload_max() { return GO_DDS_PAYLOAD_MAX; }
*/
import "C"

import (
	"context"
	"fmt"
	"sync"
	"time"
	"unsafe"

	dds "github.com/SoundMatt/go-DDS"
)

// Options configures a CycloneDDS participant. Zero value applies defaults.
type Options struct {
	// PollInterval controls how often the subscriber polls for new samples.
	// Default: 5ms. Lower values reduce latency at the cost of CPU.
	PollInterval time.Duration
}

func (o Options) pollInterval() time.Duration {
	if o.PollInterval <= 0 {
		return 5 * time.Millisecond
	}
	return o.PollInterval
}

// New creates a CycloneDDS Participant on the given domain using default options.
// Requires CycloneDDS system libraries; build with -tags cyclone.
func New(domain dds.Domain) (dds.Participant, error) {
	return NewWithOptions(domain, Options{})
}

// NewWithOptions creates a CycloneDDS Participant with explicit options.
func NewWithOptions(domain dds.Domain, opts Options) (dds.Participant, error) {
	pid := C.go_dds_create_participant(C.int32_t(domain))
	if pid < 0 {
		return nil, fmt.Errorf("cyclone: dds_create_participant failed (rc=%d)", int(pid))
	}
	return &participant{pid: pid, opts: opts, domain: domain}, nil
}

type participant struct {
	mu     sync.Mutex
	pid    C.int32_t
	opts   Options
	domain dds.Domain
	closed bool
}

// Domain implements dds.Participant.
func (p *participant) Domain() dds.Domain { return p.domain }

func (p *participant) NewPublisher(topic string, _ dds.QoS) (dds.Publisher, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return nil, fmt.Errorf("cyclone: participant closed")
	}
	cname := C.CString(topic)
	defer C.free(unsafe.Pointer(cname))

	tid := C.go_dds_create_topic(p.pid, cname)
	if tid < 0 {
		return nil, fmt.Errorf("cyclone: create topic %q failed (rc=%d)", topic, int(tid))
	}
	pubid := C.go_dds_create_publisher(p.pid)
	if pubid < 0 {
		return nil, fmt.Errorf("cyclone: create publisher failed (rc=%d)", int(pubid))
	}
	wid := C.go_dds_create_writer(pubid, tid)
	if wid < 0 {
		return nil, fmt.Errorf("cyclone: create writer failed (rc=%d)", int(wid))
	}
	return &publisher{topic: topic, wid: wid}, nil
}

func (p *participant) NewSubscriber(topic string, _ dds.QoS, _ ...dds.SubscriberOption) (dds.Subscriber, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return nil, fmt.Errorf("cyclone: participant closed")
	}
	cname := C.CString(topic)
	defer C.free(unsafe.Pointer(cname))

	tid := C.go_dds_create_topic(p.pid, cname)
	if tid < 0 {
		return nil, fmt.Errorf("cyclone: create topic %q failed (rc=%d)", topic, int(tid))
	}
	subid := C.go_dds_create_subscriber(p.pid)
	if subid < 0 {
		return nil, fmt.Errorf("cyclone: create subscriber failed (rc=%d)", int(subid))
	}
	rid := C.go_dds_create_reader(subid, tid)
	if rid < 0 {
		return nil, fmt.Errorf("cyclone: create reader failed (rc=%d)", int(rid))
	}

	s := &subscriber{
		topic: topic,
		rid:   rid,
		ch:    make(chan dds.Sample, 64),
		stop:  make(chan struct{}),
		poll:  p.opts.pollInterval(),
	}
	go s.pollLoop()
	return s, nil
}

func (p *participant) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return nil
	}
	p.closed = true
	C.dds_delete(C.dds_entity_t(p.pid))
	return nil
}

// publisher implements dds.Publisher.
type publisher struct {
	topic  string
	wid    C.int32_t
	mu     sync.Mutex
	closed bool
}

func (pub *publisher) Write(payload []byte) error {
	pub.mu.Lock()
	defer pub.mu.Unlock()
	if pub.closed {
		return fmt.Errorf("cyclone: publisher closed")
	}
	cs := C.CString(string(payload))
	defer C.free(unsafe.Pointer(cs))
	if rc := C.go_dds_write(pub.wid, cs); rc < 0 {
		return fmt.Errorf("cyclone: dds_write failed (rc=%d)", int(rc))
	}
	return nil
}

// WriteCtx writes payload, returning ctx.Err() immediately if ctx is already done.
func (pub *publisher) WriteCtx(ctx context.Context, payload []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return pub.Write(payload)
}

func (pub *publisher) Close() error {
	pub.mu.Lock()
	defer pub.mu.Unlock()
	pub.closed = true
	C.dds_delete(C.dds_entity_t(pub.wid))
	return nil
}

// subscriber implements dds.Subscriber using a polling goroutine.
// A waitset-based approach can replace this when sub-millisecond latency
// is required — polling avoids the //export CGo complexity of DDS listeners.
type subscriber struct {
	topic     string
	rid       C.int32_t
	ch        chan dds.Sample
	stop      chan struct{}
	unsubOnce sync.Once
	closeOnce sync.Once
	poll      time.Duration
}

func (s *subscriber) pollLoop() {
	payloadMax := int(C.go_dds_payload_max())
	buf := (*C.char)(C.malloc(C.size_t(payloadMax)))
	defer C.free(unsafe.Pointer(buf))

	ticker := time.NewTicker(s.poll)
	defer ticker.Stop()
	for {
		select {
		case <-s.stop:
			return
		case <-ticker.C:
			for {
				n := C.go_dds_take(s.rid, buf)
				if n <= 0 {
					break
				}
				payload := []byte(C.GoString(buf))
				select {
				case s.ch <- dds.Sample{Topic: s.topic, Payload: payload}:
				default:
					// Subscriber not reading; drop rather than block.
				}
			}
		}
	}
}

func (s *subscriber) C() <-chan dds.Sample { return s.ch }

// Unsubscribe stops the poll loop and deletes the CycloneDDS reader entity
// without closing the channel. No new samples are delivered after this call.
func (s *subscriber) Unsubscribe() error {
	s.unsubOnce.Do(func() {
		close(s.stop)
		C.dds_delete(C.dds_entity_t(s.rid))
	})
	return nil
}

func (s *subscriber) Close() error {
	_ = s.Unsubscribe()
	s.closeOnce.Do(func() { close(s.ch) })
	return nil
}
