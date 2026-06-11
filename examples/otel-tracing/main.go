// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Command otel-tracing demonstrates the OpenTelemetry adapter for go-DDS.
//
// The otel package wraps any OpenTelemetry TracerProvider as a dds.Tracer.
// Pass it to rtps.WithTracer (or auto.WithRTPSOpts) to get OTLP spans on
// every publish/receive call.
//
// This example uses a stdout OTLP exporter so you can see traces without
// an external collector. In production, replace it with an OTLP gRPC/HTTP
// exporter pointing at your collector.
//
// Run:
//
//	go run ./examples/otel-tracing
package main

import (
	"context"
	"log"
	"time"

	dds "github.com/SoundMatt/go-DDS"
	ddsotel "github.com/SoundMatt/go-DDS/otel"
	"github.com/SoundMatt/go-DDS/rtps"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

func main() {
	// Set up a stdout trace exporter for local inspection.
	exp, err := stdouttrace.New(stdouttrace.WithPrettyPrint())
	if err != nil {
		log.Fatalf("stdouttrace.New: %v", err)
	}
	tp := sdktrace.NewTracerProvider(sdktrace.WithBatcher(exp))
	otel.SetTracerProvider(tp)
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if shutErr := tp.Shutdown(ctx); shutErr != nil {
			log.Printf("tracer shutdown: %v", shutErr)
		}
	}()

	// Wrap the OTel provider as a dds.Tracer and inject it into the participant.
	tracer := ddsotel.NewTracer(otel.GetTracerProvider())

	p, err := rtps.New(dds.Domain(0),
		rtps.WithTracer(tracer),
		rtps.WithNoMulticast(),
	)
	if err != nil {
		log.Fatalf("rtps.New: %v", err)
	}
	defer func() { _ = p.Close() }()

	sub, err := p.NewSubscriber("telemetry/events", dds.DefaultQoS)
	if err != nil {
		log.Fatalf("NewSubscriber: %v", err)
	}
	defer func() { _ = sub.Close() }()

	pub, err := p.NewPublisher("telemetry/events", dds.DefaultQoS)
	if err != nil {
		log.Fatalf("NewPublisher: %v", err)
	}
	defer func() { _ = pub.Close() }()

	if err := pub.Write([]byte(`{"event":"startup","version":"1.0"}`)); err != nil {
		log.Fatalf("Write: %v", err)
	}

	select {
	case s := <-sub.C():
		log.Printf("received: %s", s.Payload)
	case <-time.After(2 * time.Second):
		log.Fatal("timeout waiting for sample")
	}

	log.Println("trace spans written to stdout — look for 'dds.publish' and 'dds.receive'")
}
