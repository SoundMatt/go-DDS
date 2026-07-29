module github.com/SoundMatt/go-DDS/observability

go 1.25.0

require (
	github.com/SoundMatt/go-DDS v0.53.0
	github.com/SoundMatt/go-DDS/safety v0.1.0
	go.opentelemetry.io/otel v1.44.0
	go.opentelemetry.io/otel/trace v1.44.0
)

require (
	github.com/SoundMatt/RELAY v1.14.0 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/pion/dtls/v3 v3.1.5 // indirect
	github.com/pion/logging v0.2.4 // indirect
	github.com/pion/transport/v4 v4.0.2 // indirect
	github.com/quic-go/quic-go v0.61.0 // indirect
	golang.org/x/crypto v0.54.0 // indirect
	golang.org/x/net v0.56.0 // indirect
	golang.org/x/sys v0.47.0 // indirect
	google.golang.org/protobuf v1.36.11 // indirect
)

// In-repo development: build/test against the sibling root module's working
// tree instead of a published tag. See ROADMAP.md, "Architecture Initiative"
// (#71), Phase D. Deliberately NOT applied to the
// github.com/SoundMatt/go-DDS/safety require above — that dependency is
// taken from the tagged safety/v0.1.0 module (per ROADMAP.md Phase D), not
// a relative-path replace, since Phase A already shipped a stable tagged
// safety module for this module to depend on.
replace github.com/SoundMatt/go-DDS => ../
