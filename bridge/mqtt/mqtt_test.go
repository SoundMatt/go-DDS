// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package mqtt_test

import (
	"sync"
	"testing"
	"time"

	dds "github.com/SoundMatt/go-DDS"
	mqttbridge "github.com/SoundMatt/go-DDS/bridge/mqtt"
	"github.com/SoundMatt/go-DDS/mock"
)

// ── stubMQTTClient ────────────────────────────────────────────────────────────

// stubToken is a synchronous Token that always succeeds.
type stubToken struct{}

func (stubToken) Wait() bool   { return true }
func (stubToken) Error() error { return nil }

// stubMQTTClient is an in-process fake MQTT broker for testing.
type stubMQTTClient struct {
	mu        sync.RWMutex
	handlers  map[string]mqttbridge.MessageHandler
	published []stubMsg
	connected bool
}

type stubMsg struct {
	topic   string
	payload []byte
}

func newStubClient() *stubMQTTClient {
	return &stubMQTTClient{
		handlers:  make(map[string]mqttbridge.MessageHandler),
		connected: true,
	}
}

func (c *stubMQTTClient) Publish(topic string, _ byte, _ bool, payload []byte) mqttbridge.Token {
	c.mu.Lock()
	c.published = append(c.published, stubMsg{topic: topic, payload: payload})
	c.mu.Unlock()
	return stubToken{}
}

func (c *stubMQTTClient) Subscribe(topic string, _ byte, handler mqttbridge.MessageHandler) mqttbridge.Token {
	c.mu.Lock()
	c.handlers[topic] = handler
	c.mu.Unlock()
	return stubToken{}
}

func (c *stubMQTTClient) Unsubscribe(topics ...string) mqttbridge.Token {
	c.mu.Lock()
	for _, t := range topics {
		delete(c.handlers, t)
	}
	c.mu.Unlock()
	return stubToken{}
}

func (c *stubMQTTClient) IsConnected() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.connected
}

func (c *stubMQTTClient) injectMQTT(topic string, payload []byte) {
	c.mu.RLock()
	h := c.handlers[topic]
	if h == nil {
		h = c.handlers["#"]
	}
	c.mu.RUnlock()
	if h != nil {
		h(topic, payload)
	}
}

func (c *stubMQTTClient) lastPublished() (string, []byte) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if len(c.published) == 0 {
		return "", nil
	}
	m := c.published[len(c.published)-1]
	return m.topic, m.payload
}

// ── tests ──────────────────────────────────────────────────────────────────────

func newMockPart(t *testing.T) dds.Participant {
	t.Helper()
	p, err := mock.New(0)
	if err != nil {
		t.Fatalf("mock.New: %v", err)
	}
	t.Cleanup(func() { p.Close() })
	return p
}

func TestNewBridge_ConnectedClient(t *testing.T) {
	p := newMockPart(t)
	client := newStubClient()
	b, err := mqttbridge.NewBridge(p, client, mqttbridge.Options{})
	if err != nil {
		t.Fatalf("NewBridge: %v", err)
	}
	defer func() { _ = b.Close() }()
}

func TestNewBridge_DisconnectedClient_Errors(t *testing.T) {
	p := newMockPart(t)
	client := newStubClient()
	client.connected = false
	_, err := mqttbridge.NewBridge(p, client, mqttbridge.Options{})
	if err == nil {
		t.Error("expected error for disconnected client")
	}
}

func TestBridge_DDSToMQTT(t *testing.T) {
	p := newMockPart(t)
	client := newStubClient()
	b, err := mqttbridge.NewBridge(p, client, mqttbridge.Options{
		DDSTopics: []string{"vehicle/speed"},
	})
	if err != nil {
		t.Fatalf("NewBridge: %v", err)
	}
	defer func() { _ = b.Close() }()

	pub, _ := p.NewPublisher("vehicle/speed", dds.DefaultQoS)
	defer pub.Close()
	_ = pub.Write([]byte(`{"kmh":120}`))

	// Allow the goroutine to forward.
	time.Sleep(50 * time.Millisecond)

	topic, payload := client.lastPublished()
	if topic != "vehicle/speed" {
		t.Errorf("MQTT topic: got %q, want vehicle/speed", topic)
	}
	if string(payload) != `{"kmh":120}` {
		t.Errorf("MQTT payload: got %q, want {\"kmh\":120}", payload)
	}
}

func TestBridge_MQTTToDDS(t *testing.T) {
	p := newMockPart(t)
	client := newStubClient()

	b, err := mqttbridge.NewBridge(p, client, mqttbridge.Options{})
	if err != nil {
		t.Fatalf("NewBridge: %v", err)
	}
	defer func() { _ = b.Close() }()

	sub, _ := p.NewSubscriber("sensor/temp", dds.DefaultQoS)
	defer sub.Close()

	client.injectMQTT("sensor/temp", []byte("22.5"))

	select {
	case s := <-sub.C():
		if string(s.Payload) != "22.5" {
			t.Errorf("DDS payload: got %q, want 22.5", s.Payload)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout: MQTT message not forwarded to DDS")
	}
}

func TestPrefixMap_DDSToMQTT(t *testing.T) {
	m := mqttbridge.PrefixMap("signals/", "vehicle/signals/")
	mqtt := m.DDSToMQTT("signals/rpm")
	if mqtt != "vehicle/signals/rpm" {
		t.Errorf("DDSToMQTT: got %q, want vehicle/signals/rpm", mqtt)
	}
}

func TestPrefixMap_MQTTToDDS(t *testing.T) {
	m := mqttbridge.PrefixMap("signals/", "vehicle/signals/")
	ddsT, ok := m.MQTTToDDS("vehicle/signals/speed")
	if !ok {
		t.Fatal("MQTTToDDS: expected match")
	}
	if ddsT != "signals/speed" {
		t.Errorf("MQTTToDDS: got %q, want signals/speed", ddsT)
	}
}

func TestPrefixMap_MQTTToDDS_NoMatch(t *testing.T) {
	m := mqttbridge.PrefixMap("signals/", "vehicle/signals/")
	_, ok := m.MQTTToDDS("other/topic")
	if ok {
		t.Error("expected no match for non-matching prefix")
	}
}

func TestIdentityMap(t *testing.T) {
	m := mqttbridge.IdentityMap
	if got := m.DDSToMQTT("foo/bar"); got != "foo/bar" {
		t.Errorf("DDSToMQTT: got %q", got)
	}
	dds, ok := m.MQTTToDDS("foo/bar")
	if !ok || dds != "foo/bar" {
		t.Errorf("MQTTToDDS: got %q, ok=%v", dds, ok)
	}
}

func TestDefaultQoSMap_DDSToMQTT(t *testing.T) {
	m := mqttbridge.DefaultQoSMap
	qos, retained := m.DDSToMQTT(dds.DefaultQoS) // BestEffort + Volatile
	if qos != 0 || retained {
		t.Errorf("BestEffort→MQTT: got qos=%d retained=%v, want 0/false", qos, retained)
	}
	qos, retained = m.DDSToMQTT(dds.ReliableQoS) // Reliable + TransientLocal
	if qos != 1 || !retained {
		t.Errorf("Reliable+TransientLocal→MQTT: got qos=%d retained=%v, want 1/true", qos, retained)
	}
}

func TestDefaultQoSMap_MQTTToDDS(t *testing.T) {
	m := mqttbridge.DefaultQoSMap
	q := m.MQTTToDDS(0, false)
	if q.Reliability != dds.BestEffort {
		t.Errorf("MQTT QoS 0 → DDS: got reliability %d, want BestEffort", q.Reliability)
	}
	q = m.MQTTToDDS(1, true)
	if q.Reliability != dds.Reliable || q.Durability != dds.TransientLocal {
		t.Errorf("MQTT QoS 1 retained → DDS: got %+v", q)
	}
}

func TestDefaultQoSMap_MQTTToDDS_ReliableNotRetained(t *testing.T) {
	m := mqttbridge.DefaultQoSMap
	// qos=1, retained=false → Reliable + Volatile (not TransientLocal)
	q := m.MQTTToDDS(1, false)
	if q.Reliability != dds.Reliable {
		t.Errorf("expected Reliable, got %v", q.Reliability)
	}
	if q.Durability != dds.Volatile {
		t.Errorf("expected Volatile (not retained), got %v", q.Durability)
	}
}

func TestBridge_FromMQTT_NoTopicMatch(t *testing.T) {
	p := newMockPart(t)
	client := newStubClient()
	// PrefixMap: MQTT topics must start with "vehicle/" to be forwarded to DDS.
	b, err := mqttbridge.NewBridge(p, client, mqttbridge.Options{
		TopicMap: mqttbridge.PrefixMap("signals/", "vehicle/"),
	})
	if err != nil {
		t.Fatalf("NewBridge: %v", err)
	}
	defer func() { _ = b.Close() }()

	sub, _ := p.NewSubscriber("signals/rpm", dds.DefaultQoS)
	defer sub.Close()

	// Inject a topic that does NOT match the mqtt prefix — should be discarded.
	client.injectMQTT("other/topic", []byte("ignored"))

	select {
	case <-sub.C():
		t.Error("non-matching MQTT topic should not be forwarded to DDS")
	case <-time.After(80 * time.Millisecond):
		// correct: nothing delivered
	}
}

func TestBridge_Close_Idempotent(t *testing.T) {
	p := newMockPart(t)
	client := newStubClient()
	b, _ := mqttbridge.NewBridge(p, client, mqttbridge.Options{})
	b.Close()
	b.Close() // must not panic
}
