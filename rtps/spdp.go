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

import (
	"encoding/binary"
	"net"
	"sync"
	"time"
)

const spdpAnnouncePeriod = 2 * time.Second

// participantProxy stores the addresses needed to exchange SEDP traffic
// with a remote participant.
type participantProxy struct {
	guid                  GUID
	metatrafficUnicast    Locator
	metatrafficMulticast  Locator
	defaultUnicast        Locator
	builtinEndpoints      uint32
}

// spdpService manages discovery announcements and the known-peers table.
type spdpService struct {
	p     *participant
	mu    sync.RWMutex
	peers map[GuidPrefix]*participantProxy
	stop  chan struct{}
}

func newSPDPService(p *participant) *spdpService {
	return &spdpService{
		p:     p,
		peers: make(map[GuidPrefix]*participantProxy),
		stop:  make(chan struct{}),
	}
}

// start launches the SPDP announce and receive goroutines.
func (s *spdpService) start() {
	go s.announceLoop()
	go s.receiveLoop()
}

func (s *spdpService) close() {
	close(s.stop)
}

// peer returns the proxy for a known participant, or nil.
func (s *spdpService) peer(prefix GuidPrefix) *participantProxy {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.peers[prefix]
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
	ticker := time.NewTicker(spdpAnnouncePeriod)
	defer ticker.Stop()
	for {
		select {
		case <-s.stop:
			return
		case <-ticker.C:
			s.sendAnnouncement()
		}
	}
}

func (s *spdpService) sendAnnouncement() {
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

	// Participant lease duration: 10 seconds (uint32 sec + uint32 frac = 8 bytes).
	duration := make([]byte, 8)
	binary.LittleEndian.PutUint32(duration[0:], 10) // 10 seconds
	binary.LittleEndian.PutUint32(duration[4:], 0)
	enc.addParam(pidParticipantLeaseDuration, duration)

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
	s.mu.Lock()
	s.peers[proxy.guid.Prefix] = proxy
	s.mu.Unlock()
}

// parseParticipantData decodes PL_CDR_LE participant proxy data.
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
		case pidParticipantGUID:
			if g, ok := decodeGUID(p.value); ok {
				proxy.guid = g
			}
		}
	}
	return proxy
}
