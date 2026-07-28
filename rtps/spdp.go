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
	// relayID is this peer's RTPS-over-Relay registration ID (Milestone 15,
	// "NAT Traversal / Cloud Gateway"). Populated either by decoding
	// pidRelayID from an SPDP announcement, or directly by
	// participant.dispatchRelayPacket whenever ANY message (not just SPDP)
	// arrives via the relay from this peer — the latter is more reliable
	// for a relay-only peer, since it needs no successful SPDP round trip
	// first and isn't subject to the usual "unknown/unreachable IP" concerns
	// that make tcpUnicast/dtlsPeers matching IP-based. Empty when the peer
	// doesn't use the relay transport at all.
	relayID string
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

// peerByPrefix returns the known participantProxy for prefix, if any.
func (s *spdpService) peerByPrefix(prefix GuidPrefix) (*participantProxy, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	proxy, ok := s.peers[prefix]
	return proxy, ok
}

// recordRelayID sets relayID on the known proxy for prefix (Milestone 15,
// "NAT Traversal / Cloud Gateway" — see participant.dispatchRelayPacket). A
// no-op if prefix has no known proxy yet (e.g. this relayed message arrived
// before any SPDP announcement from that peer was ever seen).
func (s *spdpService) recordRelayID(prefix GuidPrefix, relayID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	old, ok := s.peers[prefix]
	if !ok || old.relayID == relayID {
		return
	}
	// Callers elsewhere (allPeers, peerByPrefix) hand out the
	// *participantProxy pointer itself and read its fields without holding
	// s.mu for the duration of that read — safe only because every other
	// writer treats a published proxy as immutable and replaces the whole
	// map entry with a new value (see storePeer) rather than mutating
	// fields of a proxy another goroutine may already hold a reference to.
	// Follow that same copy-on-write discipline here instead of assigning
	// old.relayID directly.
	updated := *old
	updated.relayID = relayID
	s.peers[prefix] = &updated
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

	// Discovery over Relay (Milestone 15, "NAT Traversal / Cloud Gateway"):
	// additionally send the identical announcement, over the relay, to
	// every statically configured relay peer ID (see WithRelayPeers). This
	// is how two participants that each only know the relay's address (not
	// each other's — the very case a NAT/firewall boundary that blocks
	// direct connectivity entirely creates) first discover one another.
	if p.relaySock != nil {
		for _, id := range p.relayPeers {
			_ = p.relaySock.send(id, msg)
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

	// Relay registration ID (Milestone 15, "NAT Traversal / Cloud Gateway"),
	// when the RTPS-over-Relay transport is enabled. Peers use this to
	// reach us via the relay when no other transport is reachable — see
	// pidRelayID's doc comment for why this carries an opaque ID rather
	// than a Locator.
	if p.relaySock != nil {
		enc.addString(pidRelayID, p.relaySock.id)
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
	old, existed := s.peers[proxy.guid.Prefix]
	// A fresh SPDP announcement replaces the whole proxy object. Normally
	// this new announcement re-advertises pidRelayID itself (see
	// buildParticipantData) whenever the sender has the relay transport
	// enabled, so proxy.relayID is already correct here — but preserve the
	// previous proxy's relayID (Milestone 15) as a fallback whenever it
	// isn't, so a relayID learned out-of-band (participant.
	// dispatchRelayPacket's recordRelayID) or from an earlier announcement
	// survives a later one that happens to omit it.
	if proxy.relayID == "" && existed {
		proxy.relayID = old.relayID
	}
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
		case pidRelayID:
			if id, ok := decodeString(p.value); ok && id != "" {
				proxy.relayID = id
			}
		}
	}
	return proxy
}
