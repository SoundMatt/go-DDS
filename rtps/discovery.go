// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Discovery introspection surface (Milestone 17, "ROS 2 / rmw
// Compatibility"). NewPublisher/NewSubscriber/SEDP already track every local
// and remote endpoint internally (see sedp.go); this file exposes that
// table — plus this participant's own GUID — to other in-tree packages
// (like ros2) and to end users, the same way DiscoveryMetrics/TopicMetrics
// already expose aggregate counters rather than raw per-endpoint state.

package rtps

// DiscoveredEndpoint describes a single publication or subscription known to
// this participant — either local (registered on this process via
// NewPublisher/NewSubscriber/*WithType) or remote (learned via SEDP from a
// peer's announcement, RTPS 2.3 §8.5.4). TypeName is the opaque
// "CDR_BLOB" sentinel unless the endpoint was created with
// NewPublisherWithType/NewSubscriberWithType (or the remote peer announced
// a real type name of its own).
type DiscoveredEndpoint struct {
	GUID       GUID
	Topic      string
	TypeName   string
	IsWriter   bool
	Local      bool
	Partitions []string
}

// EndpointDiscoveryProvider is implemented by participants that expose the
// full set of publications/subscriptions they know about — their own and
// every remote peer's. rtps implements it; other backends (mock, shmem,
// cyclone) do not, since they have no wire-level discovery protocol to
// introspect.
type EndpointDiscoveryProvider interface {
	// DiscoveredEndpoints returns every known local and remote endpoint.
	// The order is unspecified.
	DiscoveredEndpoints() []DiscoveredEndpoint
}

// DiscoveredEndpoints implements EndpointDiscoveryProvider.
func (p *participant) DiscoveredEndpoints() []DiscoveredEndpoint {
	s := p.sedp
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]DiscoveredEndpoint, 0, len(s.localWriters)+len(s.localReaders)+len(s.remoteWriters)+len(s.remoteReaders))
	appendInfo := func(info *endpointInfo, local bool) {
		out = append(out, DiscoveredEndpoint{
			GUID:       info.guid,
			Topic:      info.topicName,
			TypeName:   info.typeName,
			IsWriter:   info.isWriter,
			Local:      local,
			Partitions: append([]string(nil), info.partitions...),
		})
	}
	for _, info := range s.localWriters {
		appendInfo(info, true)
	}
	for _, info := range s.localReaders {
		appendInfo(info, true)
	}
	for _, info := range s.remoteWriters {
		appendInfo(info, false)
	}
	for _, info := range s.remoteReaders {
		appendInfo(info, false)
	}
	return out
}

// GUIDProvider is implemented by any rtps value — a Participant, or a
// Publisher/Subscriber it created — that can identify itself by a stable
// wire-level GUID (RTPS 2.3 §9.3.1). Useful to callers (like the ros2
// package) that need to correlate a specific endpoint with its GUID, e.g.
// to report it in a ROS 2 graph message.
type GUIDProvider interface {
	// GUID returns this value's own GUID: the built-in-participant GUID for
	// a Participant, or the endpoint GUID for a Publisher/Subscriber.
	GUID() GUID
}

// GUID implements GUIDProvider for a Participant.
func (p *participant) GUID() GUID {
	return GUID{Prefix: p.guidPrefix, Entity: EntityIdParticipant}
}

// GUID implements GUIDProvider for a Publisher created via NewPublisher /
// NewPublisherWithType.
func (w *rtpsWriter) GUID() GUID {
	return GUID{Prefix: w.p.guidPrefix, Entity: w.eid}
}

// GUID implements GUIDProvider for a Subscriber created via NewSubscriber /
// NewSubscriberWithType / NewFilteredSubscriber.
func (r *rtpsReader) GUID() GUID {
	return GUID{Prefix: r.p.guidPrefix, Entity: r.eid}
}
