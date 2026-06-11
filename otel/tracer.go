// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Package otel bridges the dds.Tracer interface to an OpenTelemetry
// TracerProvider.
//
// Usage:
//
//	import (
//	    ddsotel "github.com/SoundMatt/go-DDS/otel"
//	    "github.com/SoundMatt/go-DDS/rtps"
//	    "go.opentelemetry.io/otel"
//	)
//
//	p, err := rtps.New(dds.Domain(0),
//	    rtps.WithTracer(ddsotel.NewTracer(otel.GetTracerProvider())),
//	)
package otel

import (
	"context"

	dds "github.com/SoundMatt/go-DDS"
	oteltrace "go.opentelemetry.io/otel/trace"

	"go.opentelemetry.io/otel/attribute"
)

const instrScope = "github.com/SoundMatt/go-DDS"

// NewTracer wraps an OpenTelemetry TracerProvider as a dds.Tracer.
// Pass otel.GetTracerProvider() to use the globally-registered provider.
func NewTracer(tp oteltrace.TracerProvider) dds.Tracer {
	return &otelTracer{tr: tp.Tracer(instrScope)}
}

type otelTracer struct {
	tr oteltrace.Tracer
}

func (t *otelTracer) Start(ctx context.Context, spanName string, attrs ...dds.SpanAttribute) (context.Context, dds.Span) {
	kvs := make([]attribute.KeyValue, len(attrs))
	for i, a := range attrs {
		kvs[i] = attribute.String(a.Key, a.Value)
	}
	ctx, span := t.tr.Start(ctx, spanName, oteltrace.WithAttributes(kvs...))
	return ctx, &otelSpan{span: span}
}

type otelSpan struct {
	span oteltrace.Span
}

func (s *otelSpan) SetAttribute(key, value string) {
	s.span.SetAttributes(attribute.String(key, value))
}

func (s *otelSpan) End() { s.span.End() }
