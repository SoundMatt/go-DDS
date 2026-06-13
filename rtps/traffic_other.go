// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

//go:build !linux

// Non-Linux stub implementations of TSN socket-marking helpers.
// SO_PRIORITY, IP_TOS, SO_TXTIME, and CLOCK_TAI are Linux-specific features;
// on Darwin, Windows, and other platforms these functions are no-ops that
// allow the rest of the code to compile and run without TSN scheduling.

package rtps

import (
	"net"
	"time"
)

func setSockPriority(_ *net.UDPConn, _ int) error { return nil }
func setSockTOS(_ *net.UDPConn, _ uint8) error    { return nil }
func enableTxTime(_ *net.UDPConn) error           { return nil }
func clockTAINow() (time.Time, error)             { return time.Now(), nil }

func scheduledSend(conn *net.UDPConn, dst *net.UDPAddr, data []byte, _ uint64) error {
	_, err := conn.WriteToUDP(data, dst)
	return err
}
