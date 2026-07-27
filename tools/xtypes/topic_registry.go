// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package xtypes

//fusa:req REQ-TREG-001
//fusa:req REQ-TREG-002
//fusa:req REQ-TREG-003

import (
	"reflect"
	"sort"
	"sync"

	dds "github.com/SoundMatt/go-DDS"
)

// TopicCodecInfo describes the codec registration for a topic.
type TopicCodecInfo struct {
	// Topic is the DDS topic name.
	Topic string
	// TypeName is the Go reflect type name of the value type (e.g. "main.Reading").
	TypeName string
}

// TopicTypeRegistry maps topic names to Go value types for codec autodiscovery.
// It enables runtime inspection of which Go type is associated with each topic,
// without requiring a concrete Codec[T] value at lookup time.
//
// Use RegisterTopicCodec to register a topic at startup, then LookupTopicType
// to retrieve the association anywhere in the program.
type TopicTypeRegistry struct {
	mu     sync.RWMutex
	topics map[string]string // topic -> Go type name
}

// NewTopicTypeRegistry returns an empty TopicTypeRegistry.
func NewTopicTypeRegistry() *TopicTypeRegistry {
	return &TopicTypeRegistry{topics: make(map[string]string)}
}

// GlobalTopicRegistry is the process-wide default TopicTypeRegistry.
// RegisterTopicCodec and LookupTopicType use this registry by default.
var GlobalTopicRegistry = NewTopicTypeRegistry()

// RegisterTopicCodec records that topic is encoded with codec[T]. The
// association between topic and the Go type T is stored in r (pass
// GlobalTopicRegistry for the process-wide default). The codec parameter is
// used only for type inference; it is not stored.
func RegisterTopicCodec[T any](r *TopicTypeRegistry, topic string, _ dds.Codec[T]) {
	var zero T
	rt := reflect.TypeOf(zero)
	var name string
	if rt == nil {
		name = "interface{}"
	} else {
		name = rt.String()
	}
	r.mu.Lock()
	r.topics[topic] = name
	r.mu.Unlock()
}

// LookupTopicType returns the Go type name registered for topic, or ("", false)
// if the topic has not been registered.
func (r *TopicTypeRegistry) LookupTopicType(topic string) (string, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	name, ok := r.topics[topic]
	return name, ok
}

// All returns all registered topic→type associations, sorted by topic name.
func (r *TopicTypeRegistry) All() []TopicCodecInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]TopicCodecInfo, 0, len(r.topics))
	for topic, typeName := range r.topics {
		out = append(out, TopicCodecInfo{Topic: topic, TypeName: typeName})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Topic < out[j].Topic })
	return out
}

// Deregister removes the registration for topic. It is a no-op if topic is not
// registered.
func (r *TopicTypeRegistry) Deregister(topic string) {
	r.mu.Lock()
	delete(r.topics, topic)
	r.mu.Unlock()
}
