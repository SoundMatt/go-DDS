// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Package mqtt provides a bidirectional bridge between a DDS participant and
// an MQTT broker. Samples published on either side are forwarded to the other
// using a configurable topic-mapping and QoS-mapping policy.
//
// # Dependency-free design
//
// go-DDS does not import an MQTT client library. Instead this package defines
// the MQTTClient interface which callers implement using their preferred client
// (e.g., github.com/eclipse/paho.mqtt.golang). This keeps go-DDS's go.mod
// free of external dependencies while still letting callers use any
// standards-compliant MQTT 3.1.1 / 5.0 client.
//
// # Quick start
//
//	client := /* paho.NewClient(...) or any MQTTClient adapter */
//	ddsParticipant, _ := mock.New(dds.Domain(0))
//
//	bridge, err := mqtt.NewBridge(ddsParticipant, client, mqtt.Options{
//	    TopicMap: mqtt.PrefixMap("vehicles/", ""),
//	    QoSMap:   mqtt.DefaultQoSMap,
//	})
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer bridge.Close()
//
// # Topic mapping
//
// By default the bridge uses a 1:1 topic name mapping (the DDS topic and the
// MQTT topic are identical). Provide a TopicMapper to rename topics as they
// cross the boundary in either direction.
package mqtt

import (
	"fmt"
	"strings"
	"sync"

	dds "github.com/SoundMatt/go-DDS"
)

// ── MQTTClient interface ──────────────────────────────────────────────────────

// Token represents an asynchronous MQTT operation. Implement it to wrap your
// client library's token / future type.
type Token interface {
	// Wait blocks until the operation completes.
	Wait() bool
	// Error returns the error (if any) from the completed operation.
	Error() error
}

// MessageHandler is called for each message received from the MQTT broker.
// topic is the MQTT topic; payload is the raw message bytes.
type MessageHandler func(topic string, payload []byte)

// MQTTClient is the interface that must be implemented to connect this bridge
// to an MQTT broker. github.com/eclipse/paho.mqtt.golang satisfies this
// interface with a thin adapter (see the adapters example in the package docs).
type MQTTClient interface {
	// Publish sends payload to an MQTT topic.
	// qos: 0=at-most-once, 1=at-least-once, 2=exactly-once.
	// retained: true means the broker stores the last message for late joiners.
	Publish(topic string, qos byte, retained bool, payload []byte) Token

	// Subscribe registers handler for all messages on an MQTT topic pattern.
	// The pattern may use MQTT wildcards (+ and #).
	Subscribe(topic string, qos byte, handler MessageHandler) Token

	// Unsubscribe cancels a previous subscription.
	Unsubscribe(topics ...string) Token

	// IsConnected reports whether the client is currently connected.
	IsConnected() bool
}

// ── QoS mapping ───────────────────────────────────────────────────────────────

// QoSMapper converts between DDS QoS and MQTT QoS levels.
type QoSMapper interface {
	// DDSToMQTT returns the MQTT QoS byte (0/1/2) and the retained flag for a
	// given DDS QoS.
	DDSToMQTT(q dds.QoS) (qos byte, retained bool)
	// MQTTToDDS returns the DDS QoS appropriate for a message received with the
	// given MQTT QoS and retained flag.
	MQTTToDDS(qos byte, retained bool) dds.QoS
}

// defaultQoSMapper is the built-in QoS mapping rule.
type defaultQoSMapper struct{}

// DefaultQoSMap is the built-in QoSMapper:
//   - DDS BestEffort → MQTT QoS 0, not retained
//   - DDS Reliable / TransientLocal → MQTT QoS 1, retained
//   - MQTT QoS 0 → DDS BestEffort + Volatile
//   - MQTT QoS 1/2 → DDS Reliable + TransientLocal
var DefaultQoSMap QoSMapper = defaultQoSMapper{}

func (defaultQoSMapper) DDSToMQTT(q dds.QoS) (byte, bool) {
	if q.Reliability == dds.Reliable {
		return 1, q.Durability == dds.TransientLocal
	}
	return 0, false
}

func (defaultQoSMapper) MQTTToDDS(qos byte, retained bool) dds.QoS {
	if qos >= 1 {
		if retained {
			return dds.ReliableQoS
		}
		return dds.QoS{Reliability: dds.Reliable, Durability: dds.Volatile}
	}
	return dds.DefaultQoS
}

// ── Topic mapping ─────────────────────────────────────────────────────────────

// TopicMapper converts topic names as they cross the DDS↔MQTT boundary.
type TopicMapper interface {
	// DDSToMQTT translates a DDS topic name to an MQTT topic.
	DDSToMQTT(ddsTopic string) string
	// MQTTToDDS translates an MQTT topic to a DDS topic name.
	// Returns "", false to discard the message (no matching DDS topic).
	MQTTToDDS(mqttTopic string) (string, bool)
}

// identityMapper passes topic names through unchanged.
type identityMapper struct{}

func (identityMapper) DDSToMQTT(t string) string         { return t }
func (identityMapper) MQTTToDDS(t string) (string, bool) { return t, true }

// IdentityMap is the default TopicMapper: DDS and MQTT use the same topic names.
var IdentityMap TopicMapper = identityMapper{}

// prefixMapper strips/adds a fixed prefix on each side.
type prefixMapper struct{ ddsPrefix, mqttPrefix string }

// PrefixMap returns a TopicMapper that prepends ddsPrefix to DDS topics and
// mqttPrefix to MQTT topics. Stripping happens on the opposite side.
//
// Example — DDS "signals/rpm" ↔ MQTT "vehicle/signals/rpm":
//
//	PrefixMap("signals/", "vehicle/signals/")
func PrefixMap(ddsPrefix, mqttPrefix string) TopicMapper {
	return prefixMapper{ddsPrefix: ddsPrefix, mqttPrefix: mqttPrefix}
}

func (m prefixMapper) DDSToMQTT(t string) string {
	stripped := strings.TrimPrefix(t, m.ddsPrefix)
	return m.mqttPrefix + stripped
}

func (m prefixMapper) MQTTToDDS(t string) (string, bool) {
	if !strings.HasPrefix(t, m.mqttPrefix) {
		return "", false
	}
	stripped := strings.TrimPrefix(t, m.mqttPrefix)
	return m.ddsPrefix + stripped, true
}

// ── Options ───────────────────────────────────────────────────────────────────

// Options configures the bridge.
type Options struct {
	// TopicMap controls how topic names are translated. Default: IdentityMap.
	TopicMap TopicMapper

	// QoSMap controls how QoS policies are translated. Default: DefaultQoSMap.
	QoSMap QoSMapper

	// DDSTopics is the set of DDS topics the bridge subscribes to and forwards
	// to MQTT. An empty slice means the bridge listens on all topics via a
	// wildcard subscriber ("#" on the DDS side).
	//
	// Note: the DDS participant must support wildcard subscriptions if this
	// field is left empty.
	DDSTopics []string

	// MQTTTopics is the set of MQTT topic patterns the bridge subscribes to and
	// forwards to DDS. An empty slice defaults to "#" (all topics).
	MQTTTopics []string

	// DDSQoS is the QoS used when the bridge creates DDS publishers for
	// messages arriving from MQTT. Default: dds.DefaultQoS.
	DDSQoS dds.QoS
}

// ── Bridge ────────────────────────────────────────────────────────────────────

// Bridge forwards samples bidirectionally between a DDS participant and an
// MQTT client. It is safe for concurrent use.
type Bridge struct {
	participant dds.Participant
	client      MQTTClient
	opts        Options

	mu         sync.Mutex
	closed     bool
	subs       []dds.Subscriber
	publishers map[string]dds.Publisher // keyed by MQTT topic
	done       chan struct{}
}

// NewBridge creates a Bridge and starts forwarding between DDS and MQTT.
// The caller is responsible for closing the bridge when done.
func NewBridge(p dds.Participant, client MQTTClient, opts Options) (*Bridge, error) {
	if !client.IsConnected() {
		return nil, fmt.Errorf("mqtt bridge: client is not connected")
	}
	if opts.TopicMap == nil {
		opts.TopicMap = IdentityMap
	}
	if opts.QoSMap == nil {
		opts.QoSMap = DefaultQoSMap
	}
	if len(opts.MQTTTopics) == 0 {
		opts.MQTTTopics = []string{"#"}
	}

	b := &Bridge{
		participant: p,
		client:      client,
		opts:        opts,
		publishers:  make(map[string]dds.Publisher),
		done:        make(chan struct{}),
	}

	// Subscribe to MQTT topics and forward to DDS.
	for _, mqttTopic := range opts.MQTTTopics {
		mqttTopic := mqttTopic // capture
		tok := client.Subscribe(mqttTopic, 1, func(topic string, payload []byte) {
			b.fromMQTT(topic, payload)
		})
		if tok.Wait() && tok.Error() != nil {
			return nil, fmt.Errorf("mqtt bridge: subscribe %q: %w", mqttTopic, tok.Error())
		}
	}

	// Subscribe to DDS topics and forward to MQTT.
	ddsTopics := opts.DDSTopics
	if len(ddsTopics) == 0 {
		ddsTopics = []string{"#"}
	}
	for _, ddsTopic := range ddsTopics {
		sub, err := p.NewSubscriber(ddsTopic, opts.DDSQoS)
		if err != nil {
			_ = b.Close()
			return nil, fmt.Errorf("mqtt bridge: DDS subscriber %q: %w", ddsTopic, err)
		}
		b.mu.Lock()
		b.subs = append(b.subs, sub)
		b.mu.Unlock()
		go b.fromDDS(sub)
	}

	return b, nil
}

// fromDDS forwards samples received on sub to the MQTT broker.
func (b *Bridge) fromDDS(sub dds.Subscriber) {
	for {
		select {
		case s, ok := <-sub.C():
			if !ok {
				return
			}
			mqttTopic := b.opts.TopicMap.DDSToMQTT(s.Topic)
			mqttQoS, retained := b.opts.QoSMap.DDSToMQTT(b.opts.DDSQoS)
			tok := b.client.Publish(mqttTopic, mqttQoS, retained, s.Payload)
			tok.Wait()
		case <-b.done:
			return
		}
	}
}

// fromMQTT forwards messages received from MQTT to the DDS participant.
func (b *Bridge) fromMQTT(mqttTopic string, payload []byte) {
	ddsTopic, ok := b.opts.TopicMap.MQTTToDDS(mqttTopic)
	if !ok {
		return
	}
	qos := b.opts.QoSMap.MQTTToDDS(0, false)

	b.mu.Lock()
	pub, exists := b.publishers[ddsTopic]
	if !exists {
		var err error
		pub, err = b.participant.NewPublisher(ddsTopic, qos)
		if err != nil {
			b.mu.Unlock()
			return
		}
		b.publishers[ddsTopic] = pub
	}
	b.mu.Unlock()

	_ = pub.Write(payload)
}

// Close stops the bridge and releases all resources.
func (b *Bridge) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return nil
	}
	b.closed = true
	close(b.done)

	for _, sub := range b.subs {
		_ = sub.Close()
	}
	for _, pub := range b.publishers {
		_ = pub.Close()
	}
	for _, mqttTopic := range b.opts.MQTTTopics {
		b.client.Unsubscribe(mqttTopic).Wait()
	}
	return nil
}
