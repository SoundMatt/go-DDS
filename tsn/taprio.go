// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package tsn

//fusa:req REQ-TSN-005
//fusa:req REQ-TSN-006

import (
	"errors"
	"time"
)

// ErrNotSupported is returned by TAPRIOConfig.Apply on platforms other than Linux.
var ErrNotSupported = errors.New("tsn: TAPRIO qdisc requires Linux")

// CycleDuration returns the effective cycle time: TAPRIOConfig.CycleTime if
// set, otherwise the sum of all entry intervals.
func (c *TAPRIOConfig) CycleDuration() time.Duration {
	if c.CycleTime > 0 {
		return c.CycleTime
	}
	var total time.Duration
	for _, e := range c.Entries {
		total += e.Interval
	}
	return total
}

// Validate checks that c is well-formed for a call to Apply.
func (c *TAPRIOConfig) Validate() error {
	if c.Interface == "" {
		return errors.New("tsn: TAPRIOConfig: Interface must not be empty")
	}
	if len(c.Entries) == 0 {
		return errors.New("tsn: TAPRIOConfig: Entries must not be empty")
	}
	for i, e := range c.Entries {
		if e.Interval <= 0 {
			return errors.New("tsn: TAPRIOConfig: entry[" + itoa(i) + "].Interval must be > 0")
		}
	}
	return nil
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	buf := [20]byte{}
	pos := len(buf)
	for n > 0 {
		pos--
		buf[pos] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[pos:])
}
