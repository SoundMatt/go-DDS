// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package otel_test

import (
	"context"
	"testing"

	dds "github.com/SoundMatt/go-DDS"
	ddsotel "github.com/SoundMatt/go-DDS/otel"
	"go.opentelemetry.io/otel/trace/noop"
)

func TestNewTracer_start_end(t *testing.T) {
	tr := ddsotel.NewTracer(noop.NewTracerProvider())
	ctx, span := tr.Start(context.Background(), "test.span",
		dds.SpanAttribute{Key: "topic", Value: "sensor/temp"},
	)
	if ctx == nil {
		t.Fatal("nil context")
	}
	span.SetAttribute("extra", "val")
	span.End()
}
