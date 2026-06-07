# go-DDS

A generic Go library for [DDS](https://www.omg.org/omg-dds-portal/) (Data Distribution Service) publish/subscribe. Works in any domain — IoT, robotics, industrial control, vehicle networks, simulation, and more.

The API is a stable Go interface. Implementations are swappable without changing application code:

| Package | Description | Requires |
|---|---|---|
| `mock` | In-process, pure Go. Zero dependencies. Default for development and testing. | Nothing |
| `rtps` | Pure-Go RTPS/UDP wire protocol. Real DDS, zero native dependencies. | Nothing |
| `cyclone` | [CycloneDDS](https://cyclonedds.io) via CGo. Interoperates with non-Go DDS participants. | `libcyclonedds-dev` + `-tags cyclone` |
| `security` | Pluggable security — NullPlugin, HMAC-SHA-256, AES-256-GCM. | Nothing |

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

// Production — pure-Go UDP transport, no native deps:
import "github.com/SoundMatt/go-DDS/rtps"
p, err := rtps.New(dds.Domain(0))

// Interop — real CycloneDDS domain, multi-host:
// (rebuild with: go build -tags cyclone ./...)
import "github.com/SoundMatt/go-DDS/cyclone"
p, err := cyclone.New(dds.Domain(0))
```

## QoS

```go
// Live data — best-effort, volatile (default)
pub, _ := p.NewPublisher("robot/joint/angles", dds.DefaultQoS)

// Commands — reliable delivery, late joiners see current state
cmd, _ := p.NewPublisher("robot/joint/target", dds.ReliableQoS)
```

## WaitSet

`dds.WaitSet` multiplexes over multiple subscribers — no polling loop required:

```go
subTemp, _ := p.NewSubscriber("sensors/temp", dds.DefaultQoS)
subSpeed, _ := p.NewSubscriber("vehicle/speed", dds.DefaultQoS)

ws := dds.NewWaitSet(subTemp, subSpeed)
ctx := context.Background()

for {
    sample, sub, err := ws.Wait(ctx)
    if err != nil {
        break
    }
    switch sub {
    case subTemp:
        fmt.Println("temp:", string(sample.Payload))
    case subSpeed:
        fmt.Println("speed:", string(sample.Payload))
    }
}
```

## Security

Pluggable payload-level security via the `security` package:

```go
import (
    "github.com/SoundMatt/go-DDS/rtps"
    "github.com/SoundMatt/go-DDS/security"
)

key := security.NewRandomKey(32)

// AES-256-GCM: full encryption + authentication
aesPlugin, _ := security.NewAESGCMPlugin(key)
p, _ := rtps.New(dds.Domain(0), rtps.WithSecurity(aesPlugin))

// HMAC-SHA-256: integrity + authentication, no encryption
hmacPlugin := security.NewHMACPlugin(key)
p, _ = rtps.New(dds.Domain(0), rtps.WithSecurity(hmacPlugin))
```

All peers communicating on a topic must use the same plugin and key.

## Example use cases

| Domain | Topic example | QoS |
|---|---|---|
| Robotics | `robot/arm/joint_states` | BestEffort (100 Hz sensor) |
| Industrial | `plc/conveyor/speed` | Reliable (actuator command) |
| Vehicle networks | `vehicle/speed` | BestEffort |
| Simulation | `sim/entity/pose` | BestEffort |
| IoT | `building/floor3/temp` | Reliable |

## Wire format

Each DDS sample payload is raw bytes. The application chooses the encoding — JSON, Protobuf, MessagePack, plain text, or anything else. go-DDS does not impose a schema.

The RTPS transport encodes payloads as CDR_LE byte arrays, compatible with the RTPS 2.3 wire format. The CycloneDDS implementation uses an opaque `RawMessage` DDS type.

## RTPS interoperability testing

The `interop/` directory contains wire-compatibility tests against a live CycloneDDS peer. These tests are gated behind the `interop` build tag so they never run in normal CI.

```bash
# Start CycloneDDS peer in Docker
docker compose -f interop/docker-compose.yml up -d cyclone-peer

# Run interop tests
go test -tags interop -v -timeout 60s ./interop/...

# Tear down
docker compose -f interop/docker-compose.yml down
```

Three tests are provided:

| Test | Direction | What it verifies |
|---|---|---|
| `TestInterop_GoPublisher_CycloneSubscriber` | go-DDS → CycloneDDS | RTPS writer is interoperable |
| `TestInterop_CyclonePublisher_GoSubscriber` | CycloneDDS → go-DDS | RTPS reader is interoperable |
| `TestInterop_BidirectionalEcho` | both | End-to-end round-trip |

Set `INTEROP_DOMAIN` (default `0`) and `INTEROP_TIMEOUT` (default `15s`) to configure the tests.

## Using CycloneDDS (production interop)

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
[![Go Reference](https://pkg.go.dev/badge/github.com/SoundMatt/go-DDS.svg)](https://pkg.go.dev/github.com/SoundMatt/go-DDS)

| Job | Platforms | Notes |
|---|---|---|
| `test-mock` | ubuntu, macOS, Windows × Go 1.22/1.23 | race detector, full coverage |
| `test-rtps` | ubuntu | `-short` (skips 2.2 s two-participant test) |
| `test-cyclone` | ubuntu-22.04 | probe-and-flag — skips cleanly if `libcyclonedds-dev` is absent |
| `benchmark-smoke` | ubuntu | 1 iteration each, catches panics/deadlocks |
| `fuzz-short` | ubuntu | 10 s per fuzz target |
| `lint` | ubuntu | golangci-lint |
| `dco` | PR only | Signed-off-by check |

## Roadmap

See [ROADMAP.md](ROADMAP.md) for per-item context, release notes, and implementation guidance.

**Released — v0.1 – v0.3**

- [x] Go interface (`Participant`, `Publisher`, `Subscriber`, `QoS`)
- [x] In-process mock — 100% statement coverage
- [x] CycloneDDS CGo implementation (`-tags cyclone`)
- [x] Configurable poll interval (`cyclone.Options`)
- [x] Pure-Go RTPS/UDP — no CGo, all platforms
- [x] Reliable QoS retransmission (HEARTBEAT / ACKNACK)
- [x] WaitSet — sub-millisecond multi-topic blocking receive
- [x] DDS-Security plugin interface (NullPlugin, HMAC-SHA-256, AES-256-GCM)
- [x] TransientLocal durability (last-value cache for late joiners)
- [x] IPv6 multicast transport (`WithIPv6()` option, `LocatorKindUDPv6`)
- [x] RTPS interop testing with CycloneDDS (Docker Compose + CycloneDDS peer)
- [x] Typed sentinel errors (`ErrClosed`, `ErrTopicEmpty` — `errors.Is` support)
- [x] Unicast-only / no-multicast discovery mode (`WithNoMulticast`, `WithPeerLocators`)
- [x] Content-filtered subscriptions (`WithFilter`)
- [x] Deadline QoS (`WithDeadlineCallback`)
- [x] Large payload fragmentation (RTPS DATA_FRAG submessage)
- [x] Topic wildcards (`sensors/#`, `vehicle/+/speed`)
- [x] Metrics / statistics API (`Metrics()` / `MetricsProvider`)
- [x] RTPS persistent history (`WithPersistentHistory`)
- [x] Real-time web monitor (`monitor/` sub-package, SSE, no external dependencies)

**Released — v0.4**

- [x] Configurable subscriber channel depth (`WithChannelDepth`) and back-pressure policy (`DropNewest` / `DropOldest` / `Block`)
- [x] Structured logging (`WithLogger(*slog.Logger)`, zero-cost when unused)
- [x] Participant liveliness detection and callback (`WithLivelinessCallback`, `LivelinessGained` / `LivelinessLost`)
- [x] Graceful shutdown with reliable-ACK drain (`CloseWithDrain(ctx)`, `Drainer` interface)
- [x] Multicast data delivery (domain-scoped multicast group, one packet per write)
- [x] Shared memory transport (`shmem/` sub-package, cross-process same-host)
- [x] INFO_TS submessage — source timestamps in `Sample.Timestamp`
- [x] MQTT bridge (`bridge/mqtt/` — DDS ↔ MQTT bidirectional, QoS and topic mapping, no external dep)
- [x] Typed generics (`TypedPublisher[T]` / `TypedSubscriber[T]`, `Codec[T]`, `JSONCodec[T]`)
- [x] OpenTelemetry-compatible tracing (`WithTracer(dds.Tracer)`, per-deliver spans, zero dep)

**Released — v0.5 — TSN (Time-Sensitive Networking)**

- [x] TSN-extended QoS fields (`TransportPriority`, `LatencyBudget`, `Lifespan`, `PublishPeriod`, `MaxSampleSize`)
- [x] DDS-to-TSN stream model (`tsn.Stream` descriptor, `tsn.StreamConfig`, `tsn.LoadConfig`)
- [x] VLAN, PCP, and DSCP socket marking (`SO_PRIORITY`, `IP_TOS`, Linux-only build tag)
- [x] Scheduled transmit time (`SO_TXTIME` + `CLOCK_TAI` + ETF/taprio qdisc, Linux-only)
- [x] gPTP / IEEE 802.1AS time base (`CLOCK_TAI` via `syscall.ClockGettime`, no external dep)
- [x] Separate traffic-class sockets (per-PCP socket map, `tsnSocketForPCP`)
- [x] TSN-safe discovery (`WithSPDPInterval`, `WithSPDPJitter`, `WithStaticPeers`)
- [x] Fragmentation bounds for TSN streams (`MaxSampleSize` guard, `splitIntoFragmentsN`)
- [x] External TSN configuration (JSON file, `tsn.LoadConfig`, `WithTSNConfig`)

**Future — [10-milestone roadmap](ROADMAP.md) targeting v0.9**

| Version | Theme |
|---|---|
| v0.6 | Production Runtime + Observability |
| v0.7 | Developer Experience + Deterministic Networking |
| v0.8 | Verification, Edge Performance, Safety |
| v0.9 | Enterprise Security, Dynamic Data, Services |

See [ROADMAP.md](ROADMAP.md) for goals, sub-items, and success criteria per milestone.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md). All commits require a DCO sign-off.

## License

Mozilla Public License v2.0 — see [LICENSE](LICENSE).  
Copyright (c) 2026 Matt Jones.
