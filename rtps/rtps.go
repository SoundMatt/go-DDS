// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Package rtps provides a pure-Go RTPS/UDP implementation of the dds
// interfaces. It does not use CGo and runs on any platform that supports
// UDP sockets.
//
// Two participants in the same process or on different hosts in the same LAN
// can exchange samples via standard RTPS wire protocol (OMG spec, version 2.3).
// Use the mock package for in-process testing; use this package when real
// network transport or interoperability with other DDS implementations is
// required.
//
// # Port Assignment
//
// Ports follow the RTPS 2.3 §9.6.1 formula:
//
//	metaMulticast(domain) = 7400 + 250*domain
//	metaUnicast(domain,i) = 7400 + 250*domain + 10 + 2*i
//	dataUnicast(domain,i) = 7400 + 250*domain + 11 + 2*i
//
// Discovery uses the multicast group 239.255.0.1.
//
// # Limitations (Phase 2)
//
//   - BestEffort QoS only (no HEARTBEAT/ACKNACK retransmission).
//   - IPv4 only.
//   - Remote subscription announcement (SEDP outbound reader) is not yet sent;
//     remote writers will not push data to local readers across process boundaries
//     without both sides also having local writers/readers for the same topic.
//     Intra-process pub/sub works without SEDP matching.
package rtps
