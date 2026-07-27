// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package rtps

import "time"

// TSNParams is the subset of a TSN stream descriptor's fields rtps needs to
// mark outbound traffic for a topic with the correct VLAN priority, DSCP
// value, fragmentation bound, and (optionally) a scheduled transmit offset.
// It holds only plain values so that rtps never needs to import package tsn
// to describe them (see ROADMAP.md, "Architecture Initiative" §Phase 0 —
// core must remain a dependency leaf).
type TSNParams struct {
	Priority       uint8         // VLAN Priority Code Point / SO_PRIORITY (0-7)
	DSCP           uint8         // IP Differentiated Services Code Point (0-63)
	Interval       time.Duration // TSN transmit interval
	TxOffset       time.Duration // transmit offset within Interval; 0 = no SO_TXTIME scheduling
	MaxFragPayload int           // max RTPS fragment payload bytes; 0 = unbounded
}

// TSNStreamConfig resolves a DDS topic name to its configured TSN transmit
// parameters. Pass an implementation to WithTSNConfig to enable per-topic
// traffic-class marking. Package tsn provides tsn.WithStreamConfig, which
// adapts a *tsn.StreamConfig to this interface without rtps importing tsn.
type TSNStreamConfig interface {
	// StreamForTopic returns the TSN parameters for topic and ok=true, or
	// ok=false when topic has no configured TSN stream.
	StreamForTopic(topic string) (params TSNParams, ok bool)
}
