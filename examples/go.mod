module github.com/SoundMatt/go-DDS/examples

go 1.25.0

require (
	github.com/SoundMatt/go-DDS v0.54.0
	github.com/SoundMatt/go-DDS/observability v0.1.0
	go.opentelemetry.io/otel v1.44.0
	go.opentelemetry.io/otel/exporters/stdout/stdouttrace v1.44.0
	go.opentelemetry.io/otel/sdk v1.44.0
)

require (
	github.com/SoundMatt/RELAY v1.14.0 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/go-logr/logr v1.4.3 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/google/uuid v1.6.0 // indirect
	go.opentelemetry.io/auto/sdk v1.2.1 // indirect
	go.opentelemetry.io/otel/metric v1.44.0 // indirect
	go.opentelemetry.io/otel/trace v1.44.0 // indirect
	golang.org/x/sys v0.47.0 // indirect
	google.golang.org/protobuf v1.36.11 // indirect
)

// In-repo development: build/test against the sibling root module's working
// tree instead of a published tag. See ROADMAP.md, "Architecture Initiative"
// (#71), Phase E. Deliberately NOT applied to the
// github.com/SoundMatt/go-DDS/observability require above (only
// examples/otel-tracing needs it) — that dependency is taken from the
// tagged observability/v0.1.0 module, not a relative-path replace, per this
// phase's whole point: examples exercise the *released* module boundaries
// as an implicit integration test of the split (see ROADMAP.md Target
// Layout: "depends on tagged versions of the above, never same-repo
// relative imports").
replace github.com/SoundMatt/go-DDS => ../
