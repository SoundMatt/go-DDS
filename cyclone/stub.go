//go:build !cyclone

// Package cyclone provides a CycloneDDS-backed implementation of the dds
// interfaces via CGo. This stub is compiled when the cyclone build tag is
// absent; it returns an error at runtime so that callers can fall back to
// the mock implementation.
//
// To build the real CGo implementation:
//
//	apt-get install -y libcyclonedds-dev   # Debian/Ubuntu
//	brew install cyclonedds                # macOS
//	go build -tags cyclone ./...
package cyclone

import (
	"fmt"

	dds "github.com/SoundMatt/go-DDS"
)

// New returns an error when the cyclone build tag is absent. Import the
// mock package instead, or rebuild with -tags cyclone and CycloneDDS installed.
func New(_ dds.Domain) (dds.Participant, error) {
	return nil, fmt.Errorf("cyclone: not built; rebuild with -tags cyclone and CycloneDDS installed (apt install libcyclonedds-dev / brew install cyclonedds)")
}
