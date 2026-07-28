// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package ros2

import (
	"errors"
	"fmt"
	"sync"

	dds "github.com/SoundMatt/go-DDS"
	"github.com/SoundMatt/go-DDS/rtps"
)

// ErrInvalidName is returned by NewROS2Participant when the given node name
// or namespace does not satisfy ValidNodeName / ValidNamespace.
var ErrInvalidName = errors.New("ros2: invalid node name or namespace")

// graphTypeName is the DDS type name rmw_fastrtps/rmw_cyclonedds register
// for rmw_dds_common/msg/ParticipantEntitiesInfo — the message exchanged on
// DiscoveryTopicName.
var graphTypeName = TypeSupportName("rmw_dds_common", "msg", "ParticipantEntitiesInfo")

// discoveryQoS is the QoS every conformant rmw implementation uses for
// DiscoveryTopicName: RELIABLE + TRANSIENT_LOCAL (keep-last, depth 1) so a
// late-joining node still receives every currently-live participant's most
// recent graph snapshot.
var discoveryQoS = dds.QoS{
	Reliability:  dds.Reliable,
	Durability:   dds.TransientLocal,
	HistoryDepth: 1,
}

// NodeInfo describes one ROS 2 node visible from a Participant's graph view
// — either the Participant's own node (Local == true) or one hosted by a
// remote participant, learned from its ros_discovery_info sample.
type NodeInfo struct {
	Namespace          string
	Name               string
	FullyQualifiedName string
	ParticipantGid     Gid
	Local              bool
}

// TopicInfo describes one ROS 2 topic visible from a Participant's
// discovery state: its fully-qualified name, every distinct type name seen
// on it, and how many local+remote publishers/subscribers are matched to
// it (mirroring `ros2 topic list -t` / `ros2 topic info`).
type TopicInfo struct {
	Name            string
	Types           []string
	PublisherCount  int
	SubscriberCount int
}

// Participant is a go-DDS RTPS participant wearing ROS 2's graph
// conventions: it publishes and subscribes using ROS 2's topic/type naming
// (see ToDDSTopicName/TypeSupportName) and participates in the
// rmw_dds_common "ros_discovery_info" graph protocol, so real ROS 2 tooling
// (`ros2 node list`, `ros2 topic list`) — and this package's own
// Nodes/Topics — see the same graph regardless of which rmw (or go-DDS)
// produced it.
//
// A Participant hosts exactly one ROS 2 node, matching the common case (one
// process == one rclcpp::Node == one DDS participant); nothing here
// prevents running several Participants — several nodes — in one process,
// each on its own underlying rtps.Participant.
type Participant struct {
	domain    dds.Domain
	nodeName  string
	namespace string // normalized: "" becomes "/"
	fqn       string
	gid       Gid

	inner dds.Participant
	typed dds.TypedEndpointFactory
	disc  rtps.EndpointDiscoveryProvider

	discoveryPub dds.Publisher
	discoverySub dds.Subscriber

	mu           sync.Mutex
	nodeEntities NodeEntitiesInfo
	graph        map[Gid]ParticipantEntitiesInfo // keyed by remote participant Gid
	closed       bool

	done chan struct{}
	wg   sync.WaitGroup
}

// NewROS2Participant creates a go-DDS RTPS participant on domain and wraps
// it with ROS 2 node conventions. nodeName must satisfy ValidNodeName;
// namespace must satisfy ValidNamespace ("" is treated as the root
// namespace "/"). opts are forwarded to rtps.New unchanged, so every
// existing rtps.Option (transports, security, TSN, …) composes normally.
func NewROS2Participant(domain dds.Domain, nodeName, namespace string, opts ...rtps.Option) (*Participant, error) {
	if !ValidNodeName(nodeName) {
		return nil, fmt.Errorf("ros2: node name %q: %w", nodeName, ErrInvalidName)
	}
	if !ValidNamespace(namespace) {
		return nil, fmt.Errorf("ros2: namespace %q: %w", namespace, ErrInvalidName)
	}
	wireNamespace := namespace
	if wireNamespace == "" {
		wireNamespace = "/"
	}

	inner, err := rtps.New(domain, opts...)
	if err != nil {
		return nil, fmt.Errorf("ros2: %w", err)
	}
	typed, ok := inner.(dds.TypedEndpointFactory)
	if !ok {
		_ = inner.Close()
		return nil, fmt.Errorf("ros2: %w", dds.ErrTypeNameUnsupported)
	}
	disc, ok := inner.(rtps.EndpointDiscoveryProvider)
	if !ok {
		_ = inner.Close()
		return nil, errors.New("ros2: participant does not support endpoint discovery")
	}
	guidP, ok := inner.(rtps.GUIDProvider)
	if !ok {
		_ = inner.Close()
		return nil, errors.New("ros2: participant does not expose a GUID")
	}

	p := &Participant{
		domain:    domain,
		nodeName:  nodeName,
		namespace: wireNamespace,
		fqn:       FullyQualifiedName(wireNamespace, nodeName),
		gid:       GidFromRTPS(guidP.GUID().Prefix, guidP.GUID().Entity),
		inner:     inner,
		typed:     typed,
		disc:      disc,
		nodeEntities: NodeEntitiesInfo{
			NodeNamespace: wireNamespace,
			NodeName:      nodeName,
		},
		graph: make(map[Gid]ParticipantEntitiesInfo),
		done:  make(chan struct{}),
	}

	pub, err := typed.NewPublisherWithType(DiscoveryTopicName, graphTypeName, discoveryQoS)
	if err != nil {
		_ = inner.Close()
		return nil, fmt.Errorf("ros2: discovery publisher: %w", err)
	}
	p.discoveryPub = pub

	sub, err := typed.NewSubscriberWithType(DiscoveryTopicName, graphTypeName, discoveryQoS)
	if err != nil {
		_ = pub.Close()
		_ = inner.Close()
		return nil, fmt.Errorf("ros2: discovery subscriber: %w", err)
	}
	p.discoverySub = sub

	p.wg.Add(1)
	go p.receiveGraphLoop()

	// Announce this node to the graph immediately, even with zero
	// endpoints — real ROS 2 nodes appear in `ros2 node list` as soon as
	// they're constructed, before publishing or subscribing anything.
	_ = p.publishGraph()

	return p, nil
}

// Domain returns the DDS domain this participant joined.
func (p *Participant) Domain() dds.Domain { return p.domain }

// Gid returns this participant's rmw_dds_common Gid.
func (p *Participant) Gid() Gid { return p.gid }

// FullyQualifiedNodeName returns this node's fully-qualified name, e.g.
// "/robot1/camera_driver".
func (p *Participant) FullyQualifiedNodeName() string { return p.fqn }

// NewPublisher creates a publisher for a ROS 2 topic. rosTopic may be
// relative (resolved against this node's namespace) or absolute (starting
// with "/"). typeName should normally come from TypeSupportName. The
// underlying DDS writer is announced under the mangled wire name
// (ToDDSTopicName) and typeName, and this node's entry in the
// ros_discovery_info graph is updated and republished.
func (p *Participant) NewPublisher(rosTopic, typeName string, qos dds.QoS) (dds.Publisher, error) {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil, fmt.Errorf("ros2: %w", dds.ErrClosed)
	}
	p.mu.Unlock()

	ddsTopic := ToDDSTopicName(FullyQualifiedName(p.namespace, rosTopic))
	pub, err := p.typed.NewPublisherWithType(ddsTopic, typeName, qos)
	if err != nil {
		return nil, fmt.Errorf("ros2: %w", err)
	}
	if gp, ok := pub.(rtps.GUIDProvider); ok {
		g := gp.GUID()
		p.mu.Lock()
		p.nodeEntities.WriterGidSeq = append(p.nodeEntities.WriterGidSeq, GidFromRTPS(g.Prefix, g.Entity))
		p.mu.Unlock()
		_ = p.publishGraph()
	}
	return pub, nil
}

// NewSubscriber creates a subscriber for a ROS 2 topic. See NewPublisher for
// rosTopic/typeName conventions.
func (p *Participant) NewSubscriber(rosTopic, typeName string, qos dds.QoS, opts ...dds.SubscriberOption) (dds.Subscriber, error) {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil, fmt.Errorf("ros2: %w", dds.ErrClosed)
	}
	p.mu.Unlock()

	ddsTopic := ToDDSTopicName(FullyQualifiedName(p.namespace, rosTopic))
	sub, err := p.typed.NewSubscriberWithType(ddsTopic, typeName, qos, opts...)
	if err != nil {
		return nil, fmt.Errorf("ros2: %w", err)
	}
	if gp, ok := sub.(rtps.GUIDProvider); ok {
		g := gp.GUID()
		p.mu.Lock()
		p.nodeEntities.ReaderGidSeq = append(p.nodeEntities.ReaderGidSeq, GidFromRTPS(g.Prefix, g.Entity))
		p.mu.Unlock()
		_ = p.publishGraph()
	}
	return sub, nil
}

// Nodes returns every ROS 2 node visible to this participant: its own node
// (Local == true) plus every node this participant has learned about from a
// remote peer's ros_discovery_info sample. Order is unspecified.
func (p *Participant) Nodes() []NodeInfo {
	p.mu.Lock()
	defer p.mu.Unlock()

	out := []NodeInfo{{
		Namespace:          p.namespace,
		Name:               p.nodeName,
		FullyQualifiedName: p.fqn,
		ParticipantGid:     p.gid,
		Local:              true,
	}}
	for gid, info := range p.graph {
		for _, n := range info.NodeEntitiesInfoSeq {
			out = append(out, NodeInfo{
				Namespace:          n.NodeNamespace,
				Name:               n.NodeName,
				FullyQualifiedName: FullyQualifiedName(n.NodeNamespace, n.NodeName),
				ParticipantGid:     gid,
				Local:              false,
			})
		}
	}
	return out
}

// Topics returns every ROS 2 topic visible to this participant — both its
// own publications/subscriptions and every remote endpoint learned via
// discovery — demangled back to ROS 2 fully-qualified topic names (see
// FromDDSTopicName). Endpoints whose DDS topic name does not carry the ROS
// 2 "rt" wire prefix (e.g. DiscoveryTopicName itself, or a plain non-ROS
// go-DDS topic) are omitted, matching what `ros2 topic list` shows. Order
// is unspecified.
func (p *Participant) Topics() []TopicInfo {
	eps := p.disc.DiscoveredEndpoints()

	agg := make(map[string]*TopicInfo)
	seenType := make(map[string]map[string]bool)
	order := make([]string, 0, len(eps))

	for _, ep := range eps {
		rosName, ok := FromDDSTopicName(ep.Topic)
		if !ok {
			continue
		}
		ti, exists := agg[rosName]
		if !exists {
			ti = &TopicInfo{Name: rosName}
			agg[rosName] = ti
			seenType[rosName] = make(map[string]bool)
			order = append(order, rosName)
		}
		if ep.TypeName != "" && !seenType[rosName][ep.TypeName] {
			seenType[rosName][ep.TypeName] = true
			ti.Types = append(ti.Types, ep.TypeName)
		}
		if ep.IsWriter {
			ti.PublisherCount++
		} else {
			ti.SubscriberCount++
		}
	}

	out := make([]TopicInfo, 0, len(order))
	for _, name := range order {
		out = append(out, *agg[name])
	}
	return out
}

// Close releases every DDS resource held by this participant: the
// discovery publisher/subscriber, their background receive loop, and the
// wrapped rtps.Participant. Safe to call more than once.
func (p *Participant) Close() error {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil
	}
	p.closed = true
	p.mu.Unlock()

	close(p.done)
	p.wg.Wait()
	if p.discoveryPub != nil {
		_ = p.discoveryPub.Close()
	}
	if p.discoverySub != nil {
		_ = p.discoverySub.Close()
	}
	return p.inner.Close()
}

// publishGraph re-announces this node's current ParticipantEntitiesInfo
// snapshot on DiscoveryTopicName. Every conformant rmw implementation
// re-publishes wholesale (not incrementally) on every graph change, which
// is what lets a late-joining peer learn the complete current state from
// TRANSIENT_LOCAL history depth 1 alone.
func (p *Participant) publishGraph() error {
	p.mu.Lock()
	info := ParticipantEntitiesInfo{
		Gid:                 p.gid,
		NodeEntitiesInfoSeq: []NodeEntitiesInfo{p.nodeEntities},
	}
	p.mu.Unlock()
	return p.discoveryPub.Write(info.Encode())
}

// receiveGraphLoop decodes every incoming ros_discovery_info sample and
// stores it under its participant Gid, replacing whatever was stored
// before — each sample is a full snapshot, never a delta (see
// publishGraph). Malformed samples (a peer running an incompatible
// encoding) are silently ignored rather than tearing down the participant.
func (p *Participant) receiveGraphLoop() {
	defer p.wg.Done()
	for {
		select {
		case s, ok := <-p.discoverySub.C():
			if !ok {
				return
			}
			info, ok := DecodeParticipantEntitiesInfo(s.Payload)
			if !ok || info.Gid == p.gid {
				continue
			}
			p.mu.Lock()
			p.graph[info.Gid] = info
			p.mu.Unlock()
		case <-p.done:
			return
		}
	}
}
