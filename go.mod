module github.com/SoundMatt/go-DDS

go 1.25.0

require (
	github.com/SoundMatt/RELAY v1.11.1
	github.com/SoundMatt/go-DDS/observability v0.1.0
	go.opentelemetry.io/otel v1.44.0
	go.opentelemetry.io/otel/exporters/stdout/stdouttrace v1.44.0
	go.opentelemetry.io/otel/sdk v1.44.0
	google.golang.org/protobuf v1.36.11
)

// In-repo development: build/test the observability submodule (ROADMAP.md,
// "Architecture Initiative" #71, Phase D) against its sibling working tree
// instead of a published tag. The root module needs this (unlike Phases B/C)
// because examples/otel-tracing imports the otel package, which moved into
// observability/ in this phase. The Phase A `safety` require/replace this
// directive used to sit next to was removed: after this phase, monitor (the
// only root-tree package that imported safety) moved into observability/
// too, so nothing in the root module imports safety directly any more —
// observability/go.mod now takes that dependency itself, from the tagged
// safety/v0.1.0 module.
replace github.com/SoundMatt/go-DDS/observability => ./observability

require (
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/go-logr/logr v1.4.3 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/google/uuid v1.6.0 // indirect
	go.opentelemetry.io/auto/sdk v1.2.1 // indirect
	go.opentelemetry.io/otel/metric v1.44.0 // indirect
	go.opentelemetry.io/otel/trace v1.44.0 // indirect
)

require golang.org/x/sys v0.46.0
