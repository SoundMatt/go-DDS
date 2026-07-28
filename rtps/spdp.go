// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// SPDP — Simple Participant Discovery Protocol (RTPS 2.3 §8.5.3 / §9.6.1).
//
// On startup, each participant announces itself on the well-known multicast
// group (239.255.0.1) at two-second intervals. Incoming announcements are
// decoded and stored in the known-participants table so that SEDP can reach
// them over unicast.

package rtps

//fusa:req REQ-DISC-001
//fusa:req REQ-DISC-002
//fusa:req REQ-DISC-003
//fusa:req REQ-DISC-004
//fusa:req REQ-DISC-005
//fusa:req REQ-DISC-006
//fusa:req REQ-DISC-007
//fusa:req REQ-DISC-011
//fusa:req REQ-DISC-012
//fusa:req REQ-DISC-015
//fusa:req REQ-PART-001
//fusa:req REQ-PART-003
//fusa:req REQ-LOC-001
//fusa:req REQ-LOC-002
//fusa:req REQ-LOC-003

import (
	"encoding/binary"
	"math/rand"
	"net"
	"sync"
	"sync/atomic"
	"time"

	dds "github.com/SoundMatt/go-DDS"
)

const defaultLeaseDuration = 10 * time.Second

const spdpAnnouncePeriod = 2 * time.Second

// participantProxy stores the addresses needed to exchange SEDP traffic
// with a remote participant.
type participantProxy struct {
	guid               GUID
	metatrafficUnicast Locator
	defaultUnicast     Locator
	builtinEndpoints   uint32
	leaseDuration      time.Duration // from pidParticipantLeaseDuration; 0 → use defaultLeaseDuration
	lastSeen           time.Time     // updated on each received SPDP announcement
	// tcpUnicast is this peer's RTPS-over-TCP listen address ("host:port"),
	// decoded from pidTCPLocator. Empty when the peer didn't advertise one
	// (TCP transport disabled, or an older go-DDS version). Milestone 14.
	tcpUnicast string
}

// spdpService manages discovery announcements and the known-peers table.
type spdpService struct {
	p     *participant
	mu    sync.RWMutex
	peers map[GuidPrefix]*participantProxy
	stop  chan struct{}

	// Discovery metric counters — incremented atomically.
	announcesSent     atomic.Uint64
	announcesReceived atomic.Uint64
	peerEvictions     atomic.Uint64
}

func newSPDPService(p *participant) *spdpService {
	return &spdpService{
		p:     p,
		peers: make(map[GuidPrefix]*participantProxy),
		stop:  make(chan struct{}),
	}
}

// start launches the SPDP announce, receive, and lease-eviction goroutines.
func (s *spdpService) start() {
	go s.announceLoop()
	go s.receiveLoop()
	go s.evictLoop()
}

func (s *spdpService) close() {
	close(s.stop)
}

// allPeers returns a snapshot of all known participant proxies.
func (s *spdpService) allPeers() []*participantProxy {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*participantProxy, 0, len(s.peers))
	for _, v := range s.peers {
		out = append(out, v)
	}
	return out
}

func (s *spdpService) announceLoop() {
	s.sendAnnouncement()
	interval := s.p.spdpInterval
	if interval <= 0 {
		interval = spdpAnnouncePeriod
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-s.stop:
			return
		case <-ticker.C:
			// Add random jitter before each announcement to avoid synchronised
			// floods when many TSN participants start simultaneously.
			if s.p.spdpJitter > 0 {
				jitter := time.Duration(rand.Int63n(int64(s.p.spdpJitter)))
				time.Sleep(jitter)
			}
			s.sendAnnouncement()
		}
	}
}

func (s *spdpService) sendAnnouncement() {
	s.announcesSent.Add(1)
	p := s.p
	payload := s.buildParticipantData()
	submsg := marshalDataSubmessage(
		EntityIdSPDPWriter,
		EntityIdSPDPReader,
		SequenceNumber{High: 0, Low: p.nextSeqNum()},
		payload,
	)
	msg := wrapInRTPSMessage(p.guidPrefix, submsg)
	dst := &net.UDPAddr{
		IP:   spdpMulticastAddr,
		Port: metaMulticastPort(int(p.domain)),
	}
	_ = p.metaSock.send(dst, msg)

	// Discovery over TCP (Milestone 14): additionally unicast the identical
	// announcement to every statically configured TCP peer. This is how a
	// participant is discovered across a network boundary UDP multicast (or
	// UDP at all) cannot cross — e.g. a firewall or cloud NAT that only
	// permits outbound TCP.
	if p.tcpSock != nil {
		for _, addr := range p.tcpPeers {
			_ = p.tcpSock.send(addr, msg)
		}
	}
}

// buildParticipantData returns PL_CDR_LE-encoded ParticipantProxy data.
func (s *spdpService) buildParticipantData() []byte {
	p := s.p
	enc := newPLCDREncoder()

	// Protocol version {2, 3}.
	enc.addParam(pidProtocolVersion, []byte{2, 3, 0, 0})

	// Vendor ID.
	enc.addParam(pidVendorId, []byte{goVendorId[0], goVendorId[1], 0, 0})

	// Participant GUID.
	enc.addGUID(pidParticipantGUID, GUID{Prefix: p.guidPrefix, Entity: EntityIdParticipant})

	// Builtin endpoints supported.
	enc.addUint32(pidBuiltinEndpointSet,
		EndpointSPDPAnnouncer|EndpointSPDPDetector|
			EndpointSEDPPubAnnouncer|EndpointSEDPPubDetector|
			EndpointSEDPSubAnnouncer|EndpointSEDPSubDetector)

	// Metatraffic unicast locator (where SEDP traffic should be sent).
	metaLocator := locatorFromUDP(&net.UDPAddr{IP: net.IPv4zero}, p.metaSock.port)
	enc.addLocator(pidMetatrafficUnicastLocator, metaLocator)

	// Default unicast locator (where user DATA should be sent).
	userLocator := locatorFromUDP(&net.UDPAddr{IP: net.IPv4zero}, p.dataSock.port)
	enc.addLocator(pidDefaultUnicastLocator, userLocator)

	// RTPS-over-TCP listen locator (Milestone 14), when the TCP transport is
	// enabled. Peers use this to reach us over TCP as a UDP fallback.
	if p.tcpSock != nil {
		tcpLocator := locatorFromTCP(net.IPv4zero, p.tcpSock.port)
		enc.addLocator(pidTCPLocator, tcpLocator)
	}

	// Participant lease duration: 10 seconds (uint32 sec + uint32 frac = 8 bytes).
	duration := make([]byte, 8)
	binary.LittleEndian.PutUint32(duration[0:], 10) // 10 seconds
	binary.LittleEndian.PutUint32(duration[4:], 0)
	enc.addParam(pidParticipantLeaseDuration, duration)

	// Append discovery authentication token when a DiscoveryPlugin is configured.
	if p.discoveryPlugin != nil {
		tag := p.discoveryPlugin.SignDiscovery(p.guidPrefix[:])
		enc.addParam(pidDiscoveryToken, tag)
	}

	return enc.finish()
}

func (s *spdpService) receiveLoop() {
	for {
		select {
		case <-s.stop:
			return
		case pkt, ok := <-s.p.mcastSock.recv:
			if !ok {
				return
			}
			s.handlePacket(pkt.data, pkt.from)
		}
	}
}

func (s *spdpService) handlePacket(data []byte, from *net.UDPAddr) {
	hdr, ok := parseHeader(data)
	if !ok {
		return
	}
	// Ignore our own announcements.
	if hdr.GuidPrefix == s.p.guidPrefix {
		return
	}
	_ = parseSubmessages(data[20:], func(id, _ byte, body []byte) error {
		if id != submsgDATA {
			return nil
		}
		ds, ok := parseDataSubmessage(flagEndianness|flagData, body)
		if !ok || ds.Payload == nil {
			return nil
		}
		if ds.WriterEntityId != EntityIdSPDPWriter {
			return nil
		}
		// Verify discovery authentication token when a plugin is configured.
		if s.p.discoveryPlugin != nil {
			token := extractDiscoveryToken(ds.Payload)
			if !s.p.discoveryPlugin.VerifyDiscovery(hdr.GuidPrefix[:], token) {
				return nil // reject unauthenticated or wrongly-signed peer
			}
		}
		proxy := parseParticipantData(hdr.GuidPrefix, ds.Payload, from)
		if proxy != nil {
			s.storePeer(proxy)
			// Notify SEDP that a new peer is available.
			s.p.sedp.onNewPeer(proxy)
		}
		return nil
	})
}

func (s *spdpService) storePeer(proxy *participantProxy) {
	s.announcesReceived.Add(1)
	proxy.lastSeen = time.Now()
	if proxy.leaseDuration == 0 {
		proxy.leaseDuration = defaultLeaseDuration
	}
	s.mu.Lock()
	_, existed := s.peers[proxy.guid.Prefix]
	s.peers[proxy.guid.Prefix] = proxy
	s.mu.Unlock()
	if !existed && s.p.livelinessCb != nil {
		// Build full participant GUID: prefix + built-in participant entity 0x000001c1.
		var g dds.GUID
		copy(g[:12], proxy.guid.Prefix[:])
		g[12] = 0x00
		g[13] = 0x00
		g[14] = 0x01
		g[15] = 0xc1
		s.p.livelinessCb(g, dds.LivelinessGained)
	}
}

// evictLoop checks once per second for peers whose lease has expired and
// removes them from the known-peers table, notifying SEDP for each eviction.
func (s *spdpService) evictLoop() {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-s.stop:
			return
		case <-ticker.C:
			s.evictExpired()
		}
	}
}

func (s *spdpService) evictExpired() {
	now := time.Now()
	var evicted []GuidPrefix
	s.mu.Lock()
	for prefix, peer := range s.peers {
		d := peer.leaseDuration
		if d == 0 {
			d = defaultLeaseDuration
		}
		if now.Sub(peer.lastSeen) > d {
			delete(s.peers, prefix)
			evicted = append(evicted, prefix)
		}
	}
	s.mu.Unlock()
	s.peerEvictions.Add(uint64(len(evicted)))
	for _, prefix := range evicted {
		s.p.sedp.onPeerEvicted(prefix)
		if s.p.livelinessCb != nil {
			var g dds.GUID
			copy(g[:12], prefix[:])
			g[12] = 0x00
			g[13] = 0x00
			g[14] = 0x01
			g[15] = 0xc1
			s.p.livelinessCb(g, dds.LivelinessLost)
		}
	}
}

// parseParticipantData decodes PL_CDR_LE participant proxy data.
// extractDiscoveryToken scans a PL_CDR_LE payload for pidDiscoveryToken and
// returns a copy of its value, or nil if the PID is absent.
func extractDiscoveryToken(payload []byte) []byte {
	dec, ok := newPLCDRDecoder(payload)
	if !ok {
		return nil
	}
	for {
		p, ok := dec.next()
		if !ok {
			break
		}
		if p.pid == pidDiscoveryToken {
			v := make([]byte, len(p.value))
			copy(v, p.value)
			return v
		}
	}
	return nil
}

func parseParticipantData(prefix GuidPrefix, payload []byte, from *net.UDPAddr) *participantProxy {
	dec, ok := newPLCDRDecoder(payload)
	if !ok {
		return nil
	}
	proxy := &participantProxy{
		guid: GUID{Prefix: prefix, Entity: EntityIdParticipant},
	}
	for {
		p, ok := dec.next()
		if !ok {
			break
		}
		switch p.pid {
		case pidMetatrafficUnicastLocator:
			if l, ok := unmarshalLocator(p.value); ok {
				proxy.metatrafficUnicast = l
				// If the address is 0.0.0.0, fill in the sender's IP.
				if proxy.metatrafficUnicast.Address == ([16]byte{}) {
					ip4 := from.IP.To4()
					if ip4 != nil {
						copy(proxy.metatrafficUnicast.Address[12:], ip4)
					}
				}
			}
		case pidDefaultUnicastLocator:
			if l, ok := unmarshalLocator(p.value); ok {
				proxy.defaultUnicast = l
				// Same 0.0.0.0 fill-in for data delivery.
				if proxy.defaultUnicast.Address == ([16]byte{}) {
					ip4 := from.IP.To4()
					if ip4 != nil {
						copy(proxy.defaultUnicast.Address[12:], ip4)
					}
				}
			}
		case pidBuiltinEndpointSet:
			if len(p.value) >= 4 {
				proxy.builtinEndpoints = binary.LittleEndian.Uint32(p.value)
			}
		case pidParticipantLeaseDuration:
			if len(p.value) >= 4 {
				secs := binary.LittleEndian.Uint32(p.value[0:4])
				if secs > 0 {
					proxy.leaseDuration = time.Duration(secs) * time.Second
				}
			}
		case pidParticipantGUID:
			if g, ok := decodeGUID(p.value); ok {
				proxy.guid = g
			}
		case pidTCPLocator:
			if l, ok := unmarshalLocator(p.value); ok && l.Kind == LocatorKindTCPv4 {
				if l.Address == ([16]byte{}) {
					ip4 := from.IP.To4()
					if ip4 != nil {
						copy(l.Address[12:], ip4)
					}
				}
				if hp, ok2 := l.tcpHostPort(); ok2 {
					proxy.tcpUnicast = hp
				}
			}
		}
	}
	return proxy
}
