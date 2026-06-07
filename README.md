# go-DDS

A generic Go library for [DDS](https://www.omg.org/omg-dds-portal/) (Data Distribution Service) publish/subscribe. Works in any domain — IoT, robotics, industrial control, vehicle networks, simulation, and more.

The API is a stable Go interface. Implementations are swappable without changing application code:

| Package | Description | Requires |
|---|---|---|
| `mock` | In-process, pure Go. Zero dependencies. Default for development and testing. | Nothing |
| `cyclone` | [CycloneDDS](https://cyclonedds.io) via CGo. Real multi-process, multi-host DDS. | `libcyclonedds-dev` + `-tags cyclone` |
| `rtps` _(planned)_ | Pure-Go RTPS/UDP wire protocol. Real DDS, zero native dependencies. | Nothing |

## Install

```bash
go get github.com/SoundMatt/go-DDS
```

## Quick start

```go
import (
    dds "github.com/SoundMatt/go-DDS"
    "github.com/SoundMatt/go-DDS/mock"
)

p, _ := mock.New(dds.Domain(0))
defer p.Close()

sub, _ := p.NewSubscriber("sensors/temperature", dds.DefaultQoS)
pub, _ := p.NewPublisher("sensors/temperature", dds.DefaultQoS)

pub.Write([]byte(`{"value": 21.5, "unit": "celsius"}`))

sample := <-sub.C()
fmt.Println(string(sample.Payload)) // {"value": 21.5, "unit": "celsius"}
```

## Switching implementations

Application code only ever references the `dds` interface package. Swap implementations at the call site:

```go
// Development / tests — no system library needed:
import "github.com/SoundMatt/go-DDS/mock"
p, err := mock.New(dds.Domain(0))

// Production — real DDS domain, multi-host:
// (rebuild with: go build -tags cyclone ./...)
import "github.com/SoundMatt/go-DDS/cyclone"
p, err := cyclone.New(dds.Domain(0))

// Advanced — custom poll interval:
p, err := cyclone.NewWithOptions(dds.Domain(0), cyclone.Options{
    PollInterval: 1 * time.Millisecond,
})
```

## QoS

```go
// Live data — best-effort, volatile (default)
pub, _ := p.NewPublisher("robot/joint/angles", dds.DefaultQoS)

// Commands — reliable delivery, late joiners see current state
cmd, _ := p.NewPublisher("robot/joint/target", dds.ReliableQoS)
```

## Example use cases

| Domain | Topic example | QoS |
|---|---|---|
| Robotics | `robot/arm/joint_states` | BestEffort (100 Hz sensor) |
| Industrial | `plc/conveyor/speed` | Reliable (actuator command) |
| Vehicle networks | `vehicle/speed` | BestEffort |
| Simulation | `sim/entity/pose` | BestEffort |
| IoT | `building/floor3/temp` | Reliable + TransientLocal |

## Wire format

Each DDS sample payload is raw bytes. The application chooses the encoding — JSON, Protobuf, MessagePack, plain text, or anything else. go-DDS does not impose a schema.

The CycloneDDS implementation uses a single opaque byte-array DDS type (`RawMessage`) on the wire. This avoids IDL compiler dependency while remaining interoperable with any DDS participant that uses the same type descriptor.

## Using CycloneDDS (production)

```bash
# Linux
apt-get install -y libcyclonedds-dev

# macOS
brew install cyclonedds

# Build
go build -tags cyclone ./...
go test -tags cyclone ./cyclone/...
```

## CI status

[![CI](https://github.com/SoundMatt/go-DDS/actions/workflows/ci.yml/badge.svg)](https://github.com/SoundMatt/go-DDS/actions/workflows/ci.yml)

- Mock suite: ubuntu, macOS, Windows × Go 1.22, 1.23 (race detector on)
- CycloneDDS suite: ubuntu with `libcyclonedds-dev` (-tags cyclone)
- Lint: golangci-lint

## Roadmap

- [x] Go interface (`Participant`, `Publisher`, `Subscriber`, `QoS`)
- [x] In-process mock — 100% statement coverage
- [x] CycloneDDS CGo implementation (`-tags cyclone`)
- [x] Configurable poll interval (`cyclone.Options`)
- [ ] Pure-Go RTPS/UDP (phase 2 — no CGo, any platform)
- [ ] Reliable QoS retransmission
- [ ] Waitset-based subscriber (sub-millisecond latency, replaces polling)
- [ ] DDS-Security (DTLS / auth plugins)

## License

Mozilla Public License v2.0 — see [LICENSE](LICENSE).  
Copyright (c) 2026 Matt Jones.
