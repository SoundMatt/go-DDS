//go:build cyclone

// Package cyclone provides a CycloneDDS-backed implementation of the dds
// interfaces via CGo. Build with -tags cyclone and CycloneDDS installed.
//
// Topic wire format: each DDS sample payload is a raw JSON byte sequence.
// The VissMessage IDL type (single bounded string field, no key) maps
// directly to a char[65536] C struct, avoiding IDL compiler dependency.
package cyclone

/*
#cgo CFLAGS: -I/usr/local/include
#cgo LDFLAGS: -lcyclonedds

#include <string.h>
#include <stdint.h>
#include "dds/dds.h"

// VissMessage holds a single JSON payload up to 64 KiB.
// Using a fixed-size array avoids heap allocation in the C layer and
// simplifies the DDS type descriptor (no pointer/string ops needed).
#define VISS_PAYLOAD_MAX 65536

typedef struct {
    char payload[VISS_PAYLOAD_MAX];
} VissMessage;

// CDR serialization opcodes for VissMessage.
// DDS_OP_ADR | DDS_OP_TYPE_4BY covers the 4-byte-aligned fixed char array;
// the array length is encoded as the second word per CycloneDDS convention.
static const uint32_t VissMessage_ops[] = {
    DDS_OP_ADR | DDS_OP_TYPE_ARR | DDS_OP_SUBTYPE_1BY,
    offsetof(VissMessage, payload),
    VISS_PAYLOAD_MAX,
    DDS_OP_RTS
};

static const dds_topic_descriptor_t VissMessage_desc = {
    sizeof(VissMessage),
    1,
    DDS_TOPIC_NO_OPTIMIZE,
    0,
    "VissMessage",
    NULL,
    sizeof(VissMessage_ops) / sizeof(VissMessage_ops[0]),
    VissMessage_ops,
    ""
};

// viss_dds_create_participant wraps dds_create_participant with a cast so
// that the Go CGo bridge sees a plain int32_t return value.
static int32_t viss_create_participant(int32_t domain) {
    return (int32_t)dds_create_participant((dds_domainid_t)domain, NULL, NULL);
}

static int32_t viss_create_topic(int32_t participant, const char *name) {
    return (int32_t)dds_create_topic(
        (dds_entity_t)participant, &VissMessage_desc, name, NULL, NULL);
}

static int32_t viss_create_publisher(int32_t participant) {
    return (int32_t)dds_create_publisher((dds_entity_t)participant, NULL, NULL);
}

static int32_t viss_create_writer(int32_t publisher, int32_t topic) {
    return (int32_t)dds_create_writer(
        (dds_entity_t)publisher, (dds_entity_t)topic, NULL, NULL);
}

static int32_t viss_create_subscriber(int32_t participant) {
    return (int32_t)dds_create_subscriber((dds_entity_t)participant, NULL, NULL);
}

static int32_t viss_create_reader(int32_t subscriber, int32_t topic) {
    return (int32_t)dds_create_reader(
        (dds_entity_t)subscriber, (dds_entity_t)topic, NULL, NULL);
}

static int32_t viss_create_waitset(int32_t participant) {
    return (int32_t)dds_create_waitset((dds_entity_t)participant);
}

static int32_t viss_write(int32_t writer, const char *payload) {
    VissMessage msg;
    memset(&msg, 0, sizeof(msg));
    strncpy(msg.payload, payload, VISS_PAYLOAD_MAX - 1);
    return (int32_t)dds_write((dds_entity_t)writer, &msg);
}

// viss_take reads one sample from reader into buf (caller-allocated,
// VISS_PAYLOAD_MAX bytes). Returns 1 if a sample was read, 0 if none,
// negative on error.
static int32_t viss_take(int32_t reader, char *buf) {
    void *samples[1];
    dds_sample_info_t infos[1];
    VissMessage msg;
    samples[0] = &msg;
    int32_t n = (int32_t)dds_take((dds_entity_t)reader, samples, infos, 1, 1);
    if (n > 0 && infos[0].valid_data) {
        strncpy(buf, msg.payload, VISS_PAYLOAD_MAX - 1);
        buf[VISS_PAYLOAD_MAX - 1] = '\0';
    }
    return n;
}

static int32_t viss_payload_max() { return VISS_PAYLOAD_MAX; }
*/
import "C"

import (
	"fmt"
	"sync"
	"time"
	"unsafe"

	dds "github.com/SoundMatt/go-DDS"
)

// New creates a CycloneDDS Participant for the given domain.
// Requires CycloneDDS system libraries; build with -tags cyclone.
func New(domain dds.Domain) (dds.Participant, error) {
	pid := C.viss_create_participant(C.int32_t(domain))
	if pid < 0 {
		return nil, fmt.Errorf("cyclone: dds_create_participant failed: %d", int(pid))
	}
	return &participant{pid: pid}, nil
}

type participant struct {
	mu     sync.Mutex
	pid    C.int32_t
	closed bool
}

func (p *participant) NewPublisher(topic string, _ dds.QoS) (dds.Publisher, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return nil, fmt.Errorf("cyclone: participant closed")
	}
	cname := C.CString(topic)
	defer C.free(unsafe.Pointer(cname))

	tid := C.viss_create_topic(p.pid, cname)
	if tid < 0 {
		return nil, fmt.Errorf("cyclone: create topic %q failed: %d", topic, int(tid))
	}
	pubid := C.viss_create_publisher(p.pid)
	if pubid < 0 {
		return nil, fmt.Errorf("cyclone: create publisher failed: %d", int(pubid))
	}
	wid := C.viss_create_writer(pubid, tid)
	if wid < 0 {
		return nil, fmt.Errorf("cyclone: create writer failed: %d", int(wid))
	}
	return &publisher{topic: topic, wid: wid}, nil
}

func (p *participant) NewSubscriber(topic string, _ dds.QoS) (dds.Subscriber, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return nil, fmt.Errorf("cyclone: participant closed")
	}
	cname := C.CString(topic)
	defer C.free(unsafe.Pointer(cname))

	tid := C.viss_create_topic(p.pid, cname)
	if tid < 0 {
		return nil, fmt.Errorf("cyclone: create topic %q failed: %d", topic, int(tid))
	}
	subid := C.viss_create_subscriber(p.pid)
	if subid < 0 {
		return nil, fmt.Errorf("cyclone: create subscriber failed: %d", int(subid))
	}
	rid := C.viss_create_reader(subid, tid)
	if rid < 0 {
		return nil, fmt.Errorf("cyclone: create reader failed: %d", int(rid))
	}

	s := &subscriber{
		topic: topic,
		rid:   rid,
		ch:    make(chan dds.Sample, 64),
		stop:  make(chan struct{}),
	}
	go s.poll()
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
	rc := C.viss_write(pub.wid, cs)
	if rc < 0 {
		return fmt.Errorf("cyclone: dds_write failed: %d", int(rc))
	}
	return nil
}

func (pub *publisher) Close() error {
	pub.mu.Lock()
	defer pub.mu.Unlock()
	pub.closed = true
	C.dds_delete(C.dds_entity_t(pub.wid))
	return nil
}

type subscriber struct {
	topic string
	rid   C.int32_t
	ch    chan dds.Sample
	stop  chan struct{}
	once  sync.Once
}

// poll spins on dds_take at 5ms intervals. This is sufficient for VISS
// request/response latencies and avoids the CGo callback complexity of
// DDS listeners (which require //export functions and runtime.LockOSThread).
// Replace with a waitset-based approach when submillisecond latency matters.
func (s *subscriber) poll() {
	payloadMax := int(C.viss_payload_max())
	buf := (*C.char)(C.malloc(C.size_t(payloadMax)))
	defer C.free(unsafe.Pointer(buf))

	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-s.stop:
			return
		case <-ticker.C:
			for {
				n := C.viss_take(s.rid, buf)
				if n <= 0 {
					break
				}
				payload := C.GoString(buf)
				select {
				case s.ch <- dds.Sample{Topic: s.topic, Payload: []byte(payload)}:
				default:
				}
			}
		}
	}
}

func (s *subscriber) C() <-chan dds.Sample { return s.ch }

func (s *subscriber) Close() error {
	s.once.Do(func() {
		close(s.stop)
		C.dds_delete(C.dds_entity_t(s.rid))
		close(s.ch)
	})
	return nil
}
