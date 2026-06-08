// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

//go:build !linux

package tsn

// Apply returns ErrNotSupported on non-Linux platforms.
// TAPRIO qdisc configuration requires Linux RTM_NEWQDISC via NETLINK_ROUTE.
func (c *TAPRIOConfig) Apply() error {
	return ErrNotSupported
}
