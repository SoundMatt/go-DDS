// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package dds_test

import (
	"context"
	"fmt"
	"time"

	dds "github.com/SoundMatt/go-DDS"
	"github.com/SoundMatt/go-DDS/mock"
)

// ExampleNewWaitSet demonstrates multiplexing over two subscribers without a
// polling loop. The WaitSet blocks until any attached subscriber delivers a
// sample, then returns it together with the subscriber it arrived on.
func ExampleNewWaitSet() {
	p, err := mock.New(dds.Domain(0))
	if err != nil {
		panic("New: " + err.Error())
	}
	defer p.Close()

	subTemp, err := p.NewSubscriber("sensors/temp", dds.DefaultQoS)
	if err != nil {
		panic("NewSubscriber: " + err.Error())
	}
	subSpeed, err := p.NewSubscriber("vehicle/speed", dds.DefaultQoS)
	if err != nil {
		panic("NewSubscriber: " + err.Error())
	}
	defer func() { _ = subTemp.Close() }()
	defer func() { _ = subSpeed.Close() }()

	pubTemp, err := p.NewPublisher("sensors/temp", dds.DefaultQoS)
	if err != nil {
		panic("NewPublisher: " + err.Error())
	}
	defer func() { _ = pubTemp.Close() }()

	_ = pubTemp.Write([]byte("21.5"))

	ws := dds.NewWaitSet(subTemp, subSpeed)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	sample, sub, err := ws.Wait(ctx)
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	switch sub {
	case subTemp:
		fmt.Println("temp:", string(sample.Payload))
	case subSpeed:
		fmt.Println("speed:", string(sample.Payload))
	}
	// Output:
	// temp: 21.5
}
