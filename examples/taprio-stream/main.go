// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// taprio-stream demonstrates how to configure a TSN stream for bounded-latency
// DDS publishing using tsn.StreamConfig and tsn.TAPRIOConfig.
//
// The example:
//  1. Loads a TSN stream config describing the "vehicle/speed" topic.
//  2. Derives a TAPRIO gate control list from the stream descriptors.
//  3. Prints the tc(8) command that would program the schedule on a real NIC
//     (requires Linux + CAP_NET_ADMIN for the actual Apply call).
//  4. Simulates SO_TXTIME-aligned publishing: computes the per-frame transmit
//     timestamp based on the TSN interval and offset, then publishes 8 frames
//     at that cadence using the in-process mock transport.
//
// Run with:
//
//	go run .
package main

import (
	"context"
	"fmt"
	"log"
	"time"

	dds "github.com/SoundMatt/go-DDS"
	"github.com/SoundMatt/go-DDS/mock"
	"github.com/SoundMatt/go-DDS/tsn"
)

// VehicleSpeed is the telemetry payload for the "vehicle/speed" topic.
type VehicleSpeed struct {
	VehicleID string
	KPH       float64
	TxTimeNS  int64 // scheduled transmit time (SO_TXTIME, CLOCK_TAI nanoseconds)
}

const (
	speedTopic = "vehicle/speed"
	steerTopic = "vehicle/steering"
	numFrames  = 8
)

func main() {
	// ── 1. Define TSN streams ──────────────────────────────────────────────────
	cfg := &tsn.StreamConfig{
		Streams: []tsn.Stream{
			{
				Topic:             speedTopic,
				VID:               100,
				PCP:               5,
				DSCP:              46,
				MaxFrameSize:      1500,
				MaxIntervalFrames: 1,
				IntervalUS:        1000, // 1 ms cycle (1000 fps)
				TxOffsetUS:        0,
				TalkerID:          "vehicle-ecu-1",
			},
			{
				Topic:             steerTopic,
				VID:               100,
				PCP:               4,
				DSCP:              34,
				MaxFrameSize:      1500,
				MaxIntervalFrames: 1,
				IntervalUS:        2000, // 2 ms cycle (500 fps)
				TxOffsetUS:        1000, // transmit at the halfway point
				TalkerID:          "vehicle-ecu-1",
			},
		},
	}

	speedStream := cfg.StreamForTopic(speedTopic)
	fmt.Printf("Stream: %s  interval=%v  offset=%v  PCP=%d  DSCP=%d\n",
		speedTopic,
		speedStream.Interval(),
		speedStream.TxOffset(),
		speedStream.PCP,
		speedStream.DSCP,
	)

	// ── 2. Derive TAPRIO gate schedule ────────────────────────────────────────
	taprioCfg, err := tsn.TAPRIOFromStreams(cfg)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("\nTAPRIO schedule: cycle=%v  entries=%d\n",
		taprioCfg.CycleTime, len(taprioCfg.Entries))
	for i, e := range taprioCfg.Entries {
		fmt.Printf("  entry[%d]: gate_mask=0x%02x  interval=%v\n",
			i, e.GateMask, e.Interval)
	}

	// ── 3. Print the tc(8) command for reference ───────────────────────────────
	taprioCfg.Interface = "eth0"
	fmt.Printf("\ntc command (for use on Linux with CAP_NET_ADMIN):\n  %s\n\n",
		taprioCfg.TCCommand("eth0", 0))

	// On Linux you could call:
	//   if err := taprioCfg.Apply(); err != nil { log.Fatal(err) }

	// ── 4. Simulate SO_TXTIME-aligned publishing ───────────────────────────────
	p, err := mock.New(dds.Domain(0))
	if err != nil {
		log.Fatal(err)
	}
	defer p.Close()

	pub, err := p.NewPublisher(speedTopic, dds.DefaultQoS)
	if err != nil {
		log.Fatal(err)
	}
	defer pub.Close()
	tpub := dds.NewTypedPublisher(pub, dds.JSONCodec[VehicleSpeed]{})

	sub, err := p.NewSubscriber(speedTopic, dds.DefaultQoS)
	if err != nil {
		log.Fatal(err)
	}
	defer sub.Close()
	tsub := dds.NewTypedSubscriber(sub, dds.JSONCodec[VehicleSpeed]{})
	defer tsub.Close()

	// Receiver goroutine
	done := make(chan struct{})
	go func() {
		defer close(done)
		for s := range tsub.C() {
			fmt.Printf("  [recv] kph=%.1f  sched_tx=%dns\n",
				s.Value.KPH, s.Value.TxTimeNS)
		}
	}()

	ctx := context.Background()
	fmt.Printf("Publishing %d frames at %v intervals:\n", numFrames, speedStream.Interval())
	for i := 0; i < numFrames; i++ {
		txTime := nextTxTime(speedStream)
		v := VehicleSpeed{
			VehicleID: "ECU-1",
			KPH:       float64(80 + i*5),
			TxTimeNS:  txTime.UnixNano(),
		}
		if err := tpub.WriteCtx(ctx, v); err != nil {
			log.Fatal(err)
		}
		fmt.Printf("  [send] kph=%.1f  sched_tx=%dns\n", v.KPH, v.TxTimeNS)
		time.Sleep(speedStream.Interval())
	}

	time.Sleep(50 * time.Millisecond)
	tsub.Close()
	<-done
}

// nextTxTime computes the next TSN transmit timestamp for s: the upcoming
// interval boundary plus the configured transmit offset.
func nextTxTime(s *tsn.Stream) time.Time {
	now := time.Now()
	interval := s.Interval()
	if interval == 0 {
		return now
	}
	// Align to next interval boundary then add offset.
	boundary := now.Truncate(interval).Add(interval)
	return boundary.Add(s.TxOffset())
}
