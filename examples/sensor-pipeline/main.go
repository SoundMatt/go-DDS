// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// sensor-pipeline demonstrates periodic publishing and aggregating subscription
// using TypedPublisher/TypedSubscriber with the JSON codec.
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
)

type Reading struct {
	SensorID string
	Celsius  float64
}

const topic = "sensors/temperature"
const numReadings = 10

func main() {
	p, err := mock.New(dds.Domain(0))
	if err != nil {
		log.Fatal(err)
	}
	defer p.Close()

	pub, err := p.NewPublisher(topic, dds.DefaultQoS)
	if err != nil {
		log.Fatal(err)
	}
	defer pub.Close()
	tpub := dds.NewTypedPublisher(pub, dds.JSONCodec[Reading]{})

	sub, err := p.NewSubscriber(topic, dds.DefaultQoS)
	if err != nil {
		log.Fatal(err)
	}
	defer sub.Close()
	tsub := dds.NewTypedSubscriber(sub, dds.JSONCodec[Reading]{})
	defer tsub.Close()

	done := make(chan struct{})
	go aggregate(tsub, done)

	ctx := context.Background()
	sensors := []struct {
		id   string
		base float64
	}{
		{"sensor-A", 21.0},
		{"sensor-B", 19.5},
		{"sensor-C", 23.2},
	}

	for i := 0; i < numReadings; i++ {
		s := sensors[i%len(sensors)]
		reading := Reading{
			SensorID: s.id,
			Celsius:  s.base + float64(i)*0.3,
		}
		if err := tpub.WriteCtx(ctx, reading); err != nil {
			log.Fatal(err)
		}
		time.Sleep(200 * time.Millisecond)
	}

	time.Sleep(100 * time.Millisecond)
	close(done)
}

func aggregate(tsub *dds.TypedSubscriber[Reading], done <-chan struct{}) {
	var sum float64
	var count int
	for {
		select {
		case s, ok := <-tsub.C():
			if !ok {
				return
			}
			count++
			sum += s.Value.Celsius
			avg := sum / float64(count)
			fmt.Printf("[%s] %.1f°C   (running avg: %.2f°C, n=%d)\n",
				s.Value.SensorID, s.Value.Celsius, avg, count)
		case <-done:
			fmt.Printf("\nFinal average over %d readings: %.2f°C\n", count, sum/float64(count))
			return
		}
	}
}
