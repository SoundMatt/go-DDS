// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package tsn

import "github.com/SoundMatt/go-DDS/rtps"

// WithStreamConfig returns an rtps.Option that registers cfg as the
// participant's TSN stream configuration, equivalent to the pre-Phase-0
// rtps.WithTSNConfig(cfg *tsn.StreamConfig). rtps itself never imports
// package tsn (see ROADMAP.md, "Architecture Initiative" §Phase 0 — core
// must remain a dependency leaf), so this adapter is the supported wiring
// point for TSN-aware participants:
//
//	p, err := rtps.New(0, tsn.WithStreamConfig(cfg))
func WithStreamConfig(cfg *StreamConfig) rtps.Option {
	return rtps.WithTSNConfig(streamConfigAdapter{cfg})
}

// streamConfigAdapter adapts *StreamConfig to rtps.TSNStreamConfig.
type streamConfigAdapter struct{ cfg *StreamConfig }

// StreamForTopic implements rtps.TSNStreamConfig.
func (a streamConfigAdapter) StreamForTopic(topic string) (rtps.TSNParams, bool) {
	s := a.cfg.StreamForTopic(topic)
	if s == nil {
		return rtps.TSNParams{}, false
	}
	return rtps.TSNParams{
		Priority:       s.PCP,
		DSCP:           s.DSCP,
		Interval:       s.Interval(),
		TxOffset:       s.TxOffset(),
		MaxFragPayload: s.MaxFragPayload(),
	}, true
}
