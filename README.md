# go-DDS

A generic Go library for [DDS](https://www.omg.org/omg-dds-portal/) (Data Distribution Service) publish/subscribe. Works in any domain — IoT, robotics, industrial control, vehicle networks, simulation, and more.

The API is a stable Go interface. Implementations are swappable without changing application code.

[![CI](https://github.com/SoundMatt/go-DDS/actions/workflows/ci.yml/badge.svg)](https://github.com/SoundMatt/go-DDS/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/SoundMatt/go-DDS.svg)](https://pkg.go.dev/github.com/SoundMatt/go-DDS)

## Packages

| Package | Description | Requires |
|---|---|---|
| `mock` | In-process broker. Zero dependencies. Default for development and testing. | Nothing |
| `rtps` | Pure-Go RTPS/UDP wire protocol. Real DDS across processes and hosts. | Nothing |
| `cyclone` | [CycloneDDS](https://cyclonedds.io) via CGo. Full wire interop with non-Go participants. | `libcyclonedds-dev` + `-tags cyclone` |
| `shmem` | Shared-memory transport. Zero UDP overhead for same-host pub/sub. | Nothing |
| `security` | Pluggable payload security — NullPlugin, HMAC-SHA-256, AES-256-GCM, CertPlugin (X.509/ECDSA), AccessPolicy (topic ACL), ReplayGuard (anti-replay); `HMACDiscoveryPlugin` for SPDP-layer peer authentication. | Nothing |
| `tools/xtypes` | Dynamic Data / XTypes — TypeDescriptor, TypeIdentifier, DynamicData, TypeRegistry, CheckCompatibility. | `go get github.com/SoundMatt/go-DDS/tools` |
| `config` | JSON/YAML participant configuration with validation. | Nothing |
| `observability/monitor` | Real-time web dashboard. `/health`, `/api/topics`, `/api/diagnostics`, SSE discovery events. | `go get github.com/SoundMatt/go-DDS/observability` |
| `tsn` | TSN stream model, TAPRIO scheduling, stream health tracking. | Nothing |
| `bridge/wan` | WAN bridge — forward DDS samples between domains over TCP (length-framed JSON, 16 MiB cap); optional TLS + shared-token auth. | Nothing |
| `observability/admin` | HTTP admin API — `/admin/health`, `/admin/topics`, `/admin/discovery`, `/admin/publish`; bearer-token auth. | `go get github.com/SoundMatt/go-DDS/observability` |
| `observability/services` | Managed service lifecycle — RecorderService, ReplayService (loop + seek), MonitorService. | `go get github.com/SoundMatt/go-DDS/observability` |
| `observability/record` | Topic recording to JSONL, deterministic replay (real-time or scaled), fault injection. | `go get github.com/SoundMatt/go-DDS/observability` |
| `pool` | Allocation-free byte-slice recycling and fixed-capacity sample ring buffer. | Nothing |
| `safety` | E2E protection header (CRC-16, sequence counter, freshness) and deterministic queue. | Nothing |
| `testutil` | Test harness helpers: `NewParticipant`, `AssertSample`, `TopicRecorder`, `BurstPublish`. | Nothing |
| `tools/cmd/ddstool` | CLI tool: `pub`, `sub`, `discover` subcommands. | `go get github.com/SoundMatt/go-DDS/tools` |

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

Application code only ever references the `dds` interface package. Swap at the call site:

```go
// Development / tests — no system library needed:
import "github.com/SoundMatt/go-DDS/mock"
p, err := mock.New(dds.Domain(0))

// Production — pure-Go UDP transport, no native deps:
import "github.com/SoundMatt/go-DDS/rtps"
p, err := rtps.New(dds.Domain(0))

// Same host, zero UDP overhead:
import "github.com/SoundMatt/go-DDS/shmem"
p, err := shmem.New(dds.Domain(0))

// Interop with non-Go DDS participants:
// go build -tags cyclone ./...
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
for {
    sample, sub, err := ws.Wait(ctx)
    if err != nil { break }
    switch sub {
    case subTemp:  fmt.Println("temp:", string(sample.Payload))
    case subSpeed: fmt.Println("speed:", string(sample.Payload))
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

All peers on a topic must use the same plugin and key.

**Enterprise security** (v0.9): X.509/ECDSA certificate authentication, topic ACL, and anti-replay protection:

```go
// CertPlugin: mutual authentication + payload encryption via X.509/ECDSA.
// Implements the rtps.SecurityPlugin (Seal/Open) interface, so it wires into
// the data path with WithSecurity. Argument order: cert, key, CA.
certPlugin, _ := security.NewCertPlugin(myCertPEM, myKeyPEM, caPEM)
p, _ := rtps.New(dds.Domain(0), rtps.WithSecurity(certPlugin))

// Secure Discovery: HMAC-SHA-256 authentication of SPDP announcements.
// Peers without the same key are silently ignored at the discovery layer.
discPlugin := security.NewHMACDiscoveryPlugin([]byte("shared-discovery-key"))
p, _ = rtps.New(dds.Domain(0), rtps.WithDiscoverySecurity(discPlugin))
```

`AccessPolicy` (topic ACL) and `ReplayGuard` (anti-replay) are **enforced in the
data path** when configured, and compose with the encryption plugin above. Both
are opt-in: with neither set, all topics are permitted and no samples are dropped.

```go
// AccessPolicy: per-topic read/write ACL with path.Match patterns.
// NewPublisher fails with dds.ErrAccessDenied on a topic that fails CanWrite;
// NewSubscriber fails on a topic that fails CanRead.
policy := security.NewAccessPolicy(
    security.Rule{Pattern: "vehicle/speed", Allow: security.PermWrite},
    security.Rule{Pattern: "vehicle/*", Allow: security.PermRead},
)

// ReplayGuard: inbound samples whose sequence number repeats within the window
// are dropped before delivery.
guard := security.NewReplayGuard(5 * time.Second)

p, _ := rtps.New(dds.Domain(0),
    rtps.WithSecurity(certPlugin),     // encryption + authentication
    rtps.WithAccessControl(policy),    // topic ACL, enforced at endpoint creation
    rtps.WithAntiReplay(guard),        // anti-replay, enforced on the receive path
)
```

## Context API

All three core operations support context cancellation:

```go
// Publisher.WriteCtx — returns ctx.Err() immediately if context is done.
ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
defer cancel()
err := pub.WriteCtx(ctx, payload)

// Subscriber.Unsubscribe — stops delivery without closing the channel.
// Use when you want to stop receiving but keep the channel readable for
// any already-buffered samples.
sub.Unsubscribe()   // no more samples delivered
<-sub.C()           // drain buffered samples (channel still open)
sub.Close()         // close channel when done

// Participant.Domain — inspect which domain a participant joined.
fmt.Println(p.Domain()) // dds.Domain(0)
```

## Configuration

Load participant settings from a JSON or YAML file:

```go
import "github.com/SoundMatt/go-DDS/config"

cfg, err := config.LoadConfig("dds.json")
// or:  cfg, err := config.ParseConfig([]byte(`{...}`))
if err != nil { log.Fatal(err) }

p, err := rtps.New(dds.Domain(0), rtps.WithConfig(cfg))
```

## Runtime monitoring

The `monitor` package serves a live web dashboard with no external dependencies:

```go
import "github.com/SoundMatt/go-DDS/observability/monitor"

mon := monitor.New(p) // p implements MetricsProvider, HealthProvider, etc.
log.Fatal(http.ListenAndServe(":8080", mon))
```

Endpoints:

| Path | Description |
|---|---|
| `GET /health` | JSON health status (`ok` / `degraded` / `down`) |
| `GET /api/topics` | Per-topic metrics (write/deliver/drop counts, bytes) |
| `GET /api/diagnostics` | Discovery metrics and transport statistics |
| `GET /api/events` | SSE stream of discovery events (peer joined/left) |

## Recording and replay

Record live traffic to a JSONL file and replay it deterministically:

```go
import "github.com/SoundMatt/go-DDS/observability/record"

// Record
sub, _ := p.NewSubscriber("vehicle/speed", dds.DefaultQoS)
f, _ := os.Create("capture.jsonl")
rec := record.NewRecorder(f).AddTopic(sub).Start()
time.Sleep(10 * time.Second)
rec.Stop()

// Replay (real-time)
f2, _ := os.Open("capture.jsonl")
pl := record.NewPlayer(f2, p)
pl.Play(ctx)

// Replay at 4× speed, only one topic
pl.PlayScaled(ctx, 4.0)
pl.PlayFiltered(ctx, []string{"vehicle/speed"})
```

## Fault injection

Stress-test consumers by wrapping any publisher with configurable faults:

```go
import "github.com/SoundMatt/go-DDS/observability/record"

pub, _ := p.NewPublisher("vehicle/speed", dds.DefaultQoS)
faulty := record.NewFaultPublisher(pub, record.FaultOptions{
    LossRate:      0.05,               // 5% packet loss
    DelayMin:      2 * time.Millisecond,
    DelayMax:      10 * time.Millisecond,
    CorruptRate:   0.01,               // 1% payload corruption
    DuplicateRate: 0.02,               // 2% duplicate delivery
}, 0 /* seed: 0 = time-seeded */)
defer faulty.Close()

faulty.Write(payload) // faults applied transparently
```

## Safety communication

Protect topics with an 18-byte E2E header (DataID, SourceID, sequence counter, timestamp, CRC-16):

```go
import "github.com/SoundMatt/go-DDS/safety"

cfg := safety.E2EConfig{
    DataID:   42,
    SourceID: 1,
    MaxAge:   100 * time.Millisecond, // freshness check
}

// Publisher side — header added automatically
rawPub, _ := p.NewPublisher("safety/speed", dds.DefaultQoS)
pub := safety.NewE2EPublisher(rawPub, cfg)

// Subscriber side — header stripped, checks run
rawSub, _ := p.NewSubscriber("safety/speed", dds.DefaultQoS)
sub := safety.NewE2ESubscriber(rawSub, cfg)

go func() {
    for e := range sub.Errors() {
        log.Printf("E2E fault: %v", e) // ErrCRCMismatch, ErrSequenceGap, ErrStaleSample
    }
}()

for s := range sub.C() {
    // s.Payload is the original payload, header already removed
}
```

Decouple writes from transport using a bounded, panic-containing queue:

```go
q := safety.NewDeterministicQueue(pub, 128 /* depth */).Start()
defer q.Stop()

if err := q.Enqueue(payload); errors.Is(err, safety.ErrQueueFull) {
    // back-pressure: queue is full
}
```

## Edge performance

Recycle payload buffers on high-throughput paths:

```go
import "github.com/SoundMatt/go-DDS/pool"

bp := pool.New(1500) // capacity in bytes
buf := bp.Get()
buf = append(buf, payload...)
pub.Write(buf)
bp.Put(buf) // returned to pool; no allocation next cycle
```

Fixed-capacity ring buffer for decoupling a subscriber from a processing loop:

```go
sb := pool.NewSampleBuffer(256)

// producer goroutine
go func() {
    for s := range sub.C() {
        if !sb.Push(s) { /* drop or handle back-pressure */ }
    }
}()

// consumer goroutine at its own rate
s, ok := sb.Pop()
```

## Testing

The `testutil` package provides ready-made fixtures for unit and integration tests:

```go
import "github.com/SoundMatt/go-DDS/testutil"

p := testutil.NewParticipant(t, dds.Domain(0)) // auto-closed at test end

sub, _ := p.NewSubscriber("test/topic", dds.DefaultQoS)
pub, _ := p.NewPublisher("test/topic", dds.DefaultQoS)

pub.Write([]byte("ping"))
testutil.AssertSample(t, sub, []byte("ping"), time.Second)

rec := testutil.NewTopicRecorder(sub).Start()
testutil.BurstPublish(pub, 10, []byte("burst"))
rec.WaitFor(10, 2*time.Second)
```

The `ddstool` CLI inspects a live domain without writing application code:

```bash
go run github.com/SoundMatt/go-DDS/tools/cmd/ddstool pub -topic vehicle/speed -payload '{"kmh":80}' -count 10
go run github.com/SoundMatt/go-DDS/tools/cmd/ddstool sub -topic vehicle/speed -count 5
go run github.com/SoundMatt/go-DDS/tools/cmd/ddstool discover -wait 5s
```

## RELAY conformance

go-DDS conforms to the [RELAY](https://github.com/SoundMatt/RELAY) cross-protocol
spec (currently **v1.11**). `dds.SpecVersion` tracks `relay.SpecVersion`
automatically; the RELAY adapter module (`adapt.go`, spec §13.7) maps
`Sample.ToMessage()` / `FromMessage()` to the canonical `relay.Message` envelope
(spec §15.7).

The `cmd/go-dds` binary is a RELAY-conformant CLI. It emits the §12 `version`,
`capabilities`, and `status` documents, implements the §11.2 `convert` interop
driver, and acts as a `relay crossbar` spoke via streaming `subscribe`/`send`:

```bash
go build -o go-dds ./tools/cmd/go-dds       # run from the repo root
./go-dds capabilities                       # §12.2 capabilities document (JSON)

# convert: canonical dds.Sample (stdin) → relay.Message (stdout)
echo '{"topic":"rt/chatter","payload":"aGVsbG8=","timestamp":"0001-01-01T00:00:00Z","seq":7,"writer_guid":[1,2,3,4,5,6,7,8,9,10,11,12,13,14,15,16]}' \
  | ./go-dds convert --protocol DDS

# crossbar spoke: subscribe writes relay.Message NDJSON; send reads it back
./go-dds subscribe --topic rt/chatter --format json   # NDJSON source (stdout)
echo '{"protocol":2,"id":"rt/chatter","payload":"aGVsbG8="}' \
  | ./go-dds send --format json                        # NDJSON sink (stdin)
```

CI enforces conformance with the upstream `relay` checker (`relay conform
--strict` and `relay interop --protocol DDS --strict`), so drift from the
RELAY schemas fails the build.

## TSN (Time-Sensitive Networking)

Map DDS topics directly to IEEE 802.1Qbv TAPRIO streams:

```go
import "github.com/SoundMatt/go-DDS/tsn"

cfg, _ := tsn.LoadConfig("streams.json")
p, _ := rtps.New(dds.Domain(0), rtps.WithConfig(cfg.ParticipantConfig))

// Monitor write timing health per stream
stream := cfg.Streams[0]
tracker := tsn.NewHealthTracker(stream, 100 /* window */)
tracker.Record(time.Now())
health := tracker.Health() // Healthy=true when <5% of writes are late

// Generate tc-qdisc command for a TAPRIO gate
taprioCfg, _ := tsn.TAPRIOFromStreams(cfg)
fmt.Println(taprioCfg.TCCommand("eth0", 0))
// tc qdisc replace dev eth0 parent root handle 100 taprio ...
```

## Dynamic Data (XTypes)

Inspect and construct DDS samples at runtime without compile-time types:

```go
import "github.com/SoundMatt/go-DDS/tools/xtypes"

// Describe the type once
td := xtypes.TypeDescriptor{
    Kind: xtypes.KindStruct,
    Fields: []xtypes.FieldDescriptor{
        {Name: "kmh",    Kind: xtypes.KindFloat64},
        {Name: "source", Kind: xtypes.KindString},
    },
}

// Build and validate a sample against the descriptor
d := xtypes.NewDynamicData(&td)
_ = d.Set("kmh", 80.0)
_ = d.Set("source", "gps")

payload, _ := d.ToJSON()   // serialize
d2 := xtypes.NewDynamicData(&td)
_ = d2.FromJSON(payload)   // deserialize with schema validation

// Forward/backward compatibility check
compatible, err := xtypes.CheckCompatibility(&writerDesc, &readerDesc)
```

## Cross-domain / cross-protocol routing

Same-protocol cross-domain forwarding and cross-protocol bridging are handled
generically by the RELAY router rather than bespoke bridges: run two go-dds
spokes and connect them with `relay crossbar` (or compose `dds.Adapt()` nodes
with `relay/router`). The former `bridge/domain` and `bridge/mqtt` packages were
removed in favour of this (see the [RELAY conformance](#relay-conformance)
section); use `relay crossbar` with an `identity` route for DDS→DDS, or a
re-tag converter for DDS↔other protocols.

## WAN Bridge

Forward DDS samples between domains over a TCP connection (e.g. edge → cloud):

```go
import "github.com/SoundMatt/go-DDS/bridge/wan"

// Cloud side — receive frames and publish to the cloud participant
srv, err := wan.Serve(cloudParticipant, ":9000", wan.Options{})
defer srv.Close()

// Edge side — subscribe to topics and stream to the cloud
cli, err := wan.Connect(edgeParticipant, "cloud.example.com:9000", wan.Options{
    Topics: []string{"vehicle/speed", "vehicle/status"},
})
defer cli.Close()
```

Wire format: 4-byte big-endian length prefix + JSON `{"t":"<topic>","p":"<base64>"}`. Hard cap: 16 MiB per frame. For bidirectional bridging, create one pair in each direction.

## Admin API

HTTP endpoints for runtime inspection and publishing without a DDS client:

```go
import "github.com/SoundMatt/go-DDS/observability/admin"

srv, err := admin.New(p, admin.Options{
    Addr:  ":8081",
    Token: "secret",   // Bearer token; empty = no auth
})
defer srv.Close()
```

| Endpoint | Method | Description |
|---|---|---|
| `/admin/health` | GET | Participant health status |
| `/admin/topics` | GET | Per-topic metrics |
| `/admin/discovery` | GET | Discovery metrics |
| `/admin/publish` | POST | Publish a payload to a topic |

## Services

Managed lifecycle wrappers for recorder, replay, and monitor:

```go
import "github.com/SoundMatt/go-DDS/observability/services"

// Record to file with managed start/stop
recSvc := services.NewRecorderService(p, services.RecorderOptions{
    Output: f,
    Topics: []string{"vehicle/speed"},
})
recSvc.Start()
time.Sleep(10 * time.Second)
recSvc.Stop()

// Replay from file (loop until stopped)
replSvc := services.NewReplayService(p, services.ReplayOptions{
    Input: f2,
    Loop:  true,
    Speed: 2.0,   // 2× real-time
})
replSvc.Start()
defer replSvc.Stop()

// Run the monitor as a managed service
monSvc := services.NewMonitorService(p, ":8080")
monSvc.Start()
defer monSvc.Stop()
```

## RTPS interoperability testing

Wire-compatibility tests against a live CycloneDDS peer (gated behind the `interop` build tag):

```bash
docker compose -f interop/docker-compose.yml up -d cyclone-peer
go test -tags interop -v -timeout 60s ./interop/...
docker compose -f interop/docker-compose.yml down
```

## Using CycloneDDS

```bash
# Linux
apt-get install -y libcyclonedds-dev

# macOS
brew install cyclonedds

# Build + test
go build -tags cyclone ./...
go test -tags cyclone ./cyclone/...
```

## CI

| Job | Platforms | Notes |
|---|---|---|
| `test-mock` | ubuntu, macOS, Windows × Go 1.25/1.26 | race detector, full coverage |
| `test-rtps` | ubuntu | `-short` |
| `test-cyclone` | ubuntu-22.04 | skips cleanly if `libcyclonedds-dev` absent |
| `benchmark-smoke` | ubuntu | 1 iteration each, catches panics/deadlocks |
| `fuzz-short` | ubuntu | 10 s per fuzz target |
| `lint` | ubuntu | golangci-lint v2 |
| `dco` | PR only | Signed-off-by check |

## Roadmap

See [ROADMAP.md](ROADMAP.md) for per-milestone goals, sub-items, and success criteria.
See [CHANGELOG.md](CHANGELOG.md) for the release history and
[docs/MATURITY.md](docs/MATURITY.md) for per-package maturity (Stable /
Beta / Experimental / Reference).

**Released — v0.1 – v0.3**

- [x] Go interface (`Participant`, `Publisher`, `Subscriber`, `QoS`, `WaitSet`)
- [x] In-process mock broker — 100% statement coverage
- [x] CycloneDDS CGo implementation (`-tags cyclone`)
- [x] Pure-Go RTPS/UDP — no CGo, all platforms
- [x] Reliable QoS retransmission (HEARTBEAT / ACKNACK)
- [x] DDS-Security plugin interface (NullPlugin, HMAC-SHA-256, AES-256-GCM)
- [x] TransientLocal durability — late-joiner last-value cache
- [x] IPv6 multicast transport
- [x] RTPS interop testing with CycloneDDS (Docker Compose)
- [x] Sentinel errors, unicast discovery, content filters, deadline QoS
- [x] Large-payload fragmentation (DATA_FRAG), topic wildcards, metrics
- [x] Persistent history, real-time web monitor

**Released — v0.4**

- [x] Configurable channel depth and back-pressure (`DropNewest` / `DropOldest` / `Block`)
- [x] Structured logging (`WithLogger(*slog.Logger)`)
- [x] Participant liveliness detection (`WithLivelinessCallback`)
- [x] Graceful shutdown with reliable-ACK drain (`CloseWithDrain`)
- [x] Multicast data delivery
- [x] Shared-memory transport (`shmem/`)
- [x] INFO_TS submessage — source timestamps in `Sample.Timestamp`
- [x] Typed generics (`TypedPublisher[T]`, `TypedSubscriber[T]`, `JSONCodec[T]`)
- [x] OpenTelemetry-compatible tracing

**Released — v0.5 — TSN**

- [x] TSN-extended QoS fields (`TransportPriority`, `LatencyBudget`, `Lifespan`, `PublishPeriod`)
- [x] DDS-to-TSN stream model (`tsn.Stream`, `tsn.StreamConfig`, `tsn.LoadConfig`)
- [x] VLAN, PCP, and DSCP socket marking (Linux)
- [x] Scheduled transmit time (`SO_TXTIME` + `CLOCK_TAI` + ETF/TAPRIO, Linux)
- [x] Per-PCP traffic-class sockets, TSN-safe discovery, fragmentation bounds

**Released — v0.6 — Production Runtime + Observability**

- [x] JSON/YAML participant configuration (`config/`)
- [x] `DiscoveryMetrics`, `TopicMetrics`, `Health` interfaces
- [x] Per-topic atomic counters in `rtps` and `mock`
- [x] `WithHeartbeatPeriod`, `WithConfig` RTPS options
- [x] Monitor `/health`, `/api/topics`, `/api/diagnostics`, SSE discovery events

**Released — v0.7 — Developer Experience + Deterministic Networking**

- [x] Test harness helpers (`testutil/`) — `NewParticipant`, `AssertSample`, `TopicRecorder`, `BurstPublish`, `PeriodicPublish`
- [x] CLI tool (`cmd/ddstool`) — `pub`, `sub`, `discover` subcommands
- [x] TSN diagnostics (`tsn.HealthTracker`, `tsn.TAPRIOConfig`, `TCCommand`)
- [x] CI upgraded to Node.js 24 and golangci-lint v2

**Released — v0.8 — Verification, Edge Performance, Safety**

- [x] Topic recording to JSONL (`record.Recorder`) and deterministic replay (`record.Player`) — real-time, time-scaled, topic-filtered
- [x] Fault injection wrapper (`record.FaultPublisher`) — packet loss, delay, corruption, duplication, reordering
- [x] Allocation-free buffer recycling (`pool.BytePool`) and sample ring buffer (`pool.SampleBuffer`)
- [x] E2E protection header (`safety.E2EPublisher` / `safety.E2ESubscriber`) — CRC-16/CCITT, sequence counter, configurable freshness window
- [x] Deterministic queue with panic containment (`safety.DeterministicQueue`)

**Released — v0.9 — Enterprise Security, Dynamic Data, Services, Context API**

- [x] CertPlugin (X.509/ECDSA mutual auth), AccessPolicy (topic ACL), ReplayGuard (anti-replay) — `security/`
- [x] XTypes dynamic data — TypeDescriptor, TypeIdentifier (content hash), DynamicData, TypeRegistry, CheckCompatibility — `xtypes/`
- [x] WAN bridge — TCP forwarding with length-framed JSON wire format, 16 MiB cap — `bridge/wan/`
- [x] HTTP admin API — health, metrics, discovery, publish; bearer-token auth — `admin/`
- [x] Managed service lifecycle — RecorderService, ReplayService (loop + seek + speed), MonitorService — `services/`
- [x] `Participant.Domain()` accessor — `dds.Participant` interface
- [x] `Publisher.WriteCtx(ctx, payload)` — context-aware writes across all transports
- [x] `Subscriber.Unsubscribe()` — non-destructive deregistration (channel stays open)
- [x] `mock.IsolatedBroker()` — per-test broker isolation, eliminates cross-test echo loops
- [x] `HMACDiscoveryPlugin` + `rtps.WithDiscoverySecurity()` — HMAC-SHA-256 authenticated SPDP announcements

**Released — v0.9.1 — Spec Completeness and Go Idioms**

- [x] `Sample.SequenceNumber` and `Sample.WriterGUID` — per-writer monotonic counter and endpoint identity on all transports (mock, shmem, rtps)
- [x] `Subscriber.TryRead()` — non-blocking read on all transports (mock, shmem, rtps, cyclone)
- [x] Active subscriber Deadline QoS enforcement — `WithDeadlineMissed(fn)` fires callback when no sample arrives within `QoS.Deadline` period
- [x] Wildcard subscriptions in rtps and shmem transports (MQTT-style `+` and `#`)
- [x] `rpc` package — OMG DDS-RPC style request-reply via `Requester[Req,Rep]` and `Replier[Req,Rep]`
- [x] `GobCodec[T]` — stdlib binary codec complementing `JSONCodec[T]`
- [x] `ErrQoSMismatch`, `ErrDeadlineMissed`, `ErrSampleRejected`, `ErrResourceLimits` sentinel errors
- [x] `TypedSample[T]` gains `SequenceNumber` and `WriterGUID` fields

**Released — v0.9.2 — ProtoCodec, Reorder Fault Injection, WithContext, Fuzz Coverage**

- [x] `ProtoCodec[T proto.Message]` — Protocol Buffers codec complementing `JSONCodec[T]` and `GobCodec[T]`
- [x] `FaultPublisher.ReorderWindow` — shuffle a window of N samples on emit, simulating out-of-order delivery
- [x] `FaultPublisher.WriteCtx(ctx, payload)` — context-aware fault writes honouring deadline cancellation during delay
- [x] `mock.WithContext(ctx)` / `rtps.WithContext(ctx)` — tie participant lifetime to a `context.Context`
- [x] Fuzz targets for `security` (HMAC + AES-GCM round-trip and arbitrary-input safety)
- [x] Fuzz targets for `rpc` wire format (reply/request dispatch robustness, round-trip)
- [x] Fuzz targets for `ProtoCodec` (round-trip and arbitrary Unmarshal)

**Released — v0.10 — Dynamic WaitSet, REST/SSE Bridge, Secure SEDP, TypeRegistry, Docker Quickstart**

- [x] `WaitSet.Attach(subs...)` / `WaitSet.Detach(subs...)` — dynamically add/remove subscribers from an in-flight WaitSet (snapshot-safe, race-free)
- [x] `bridge/rest` — HTTP/SSE gateway: `GET /topics` (list), `GET /topics/{t}` (SSE stream, base64 payload), `POST /topics/{t}` (publish); Bearer token auth; keepalive pings
- [x] `rtps.EndpointPlugin` + `HMACDiscoveryPlugin.SignEndpoint`/`VerifyEndpoint` — HMAC-SHA-256 endpoint tags embedded in SEDP announcements (vendor PID 0x8002); unauthenticated endpoints are silently rejected
- [x] `xtypes.TopicTypeRegistry` + `RegisterTopicCodec[T]` + `GlobalTopicRegistry` — map topic names to Go type names for codec autodiscovery
- [x] Docker Quickstart — `cmd/monitor`, `examples/quickstart/{pub,sub}`, multi-stage `docker/Dockerfile`, `docker/docker-compose.yml` (`network_mode: host` for RTPS multicast)

**Released — v0.11 — gRPC Bridge, Key Rotation, Bridge Networking, Interop, DevContainer, GHCR**

- [x] `bridge/grpc` — gRPC gateway (JSON codec): `Subscribe` server-streaming, `Publish` unary, `StreamPublish` client-streaming; Bearer token auth interceptors; per-topic filter and transform hooks; YAML config (`LoadConfig`/`ApplyConfig`)
- [x] `HMACDiscoveryPlugin.Rekey(newKey)` — atomic key rotation; RWMutex-safe; old tags are immediately invalidated for both SPDP and SEDP
- [x] Docker bridge networking — `DDS_PEERS` + `WithNoMulticast` across all quickstart binaries; `docker/docker-compose.yml` updated to bridge network (works on macOS/Windows Docker Desktop); `docker/docker-compose.host.yml` overlay for Linux host networking
- [x] `docker/compose.interop.yml` — CycloneDDS peer containers on the same bridge network for cross-implementation wire-compat testing
- [x] `.devcontainer/devcontainer.json` — Go 1.25 dev container with golangci-lint, Docker-in-Docker, and VS Code Go extension; works in GitHub Codespaces
- [x] `.github/workflows/docker-publish.yml` — multi-arch (`linux/amd64`, `linux/arm64`) GHCR publish on push to main and version tags

**Released — v0.12 — Examples, Safety Completeness, IDL/CDR Compiler, TAPRIO**

- [x] `examples/sensor-pipeline`, `examples/command-response`, `examples/secure-topic`, `examples/taprio-stream` — self-contained runnable examples with `go run .`
- [x] `E2ESubscriber` schema validation — per-topic payload-length and schema-hash checks with `SafetyEventKindSchemaViolation`
- [x] `safety.Metrics` per-topic violation counters (CRC, sequence gap, stale, schema) via `SafetyMetrics()` snapshot
- [x] Monitor SSE `safety` event type — `WatchSafety()` broadcasts `SafetyEvent` to browser clients
- [x] `idl/` — `.idl` → Go struct + `Codec[T]` code generation; CDR/XCDR1 encode/decode
- [x] `tsn.TAPRIOConfig.Apply()` — TAPRIO qdisc configuration via netlink
- [x] go-FuSa: 280+ requirements, all `[traced+tested]`

**Released — v0.13.x — IDL Factory Codegen, RateMonitor, TSN Dashboard**

- [x] IDL `NewXxxPublisher`/`NewXxxSubscriber` typed factory wrappers
- [x] `safety.RateMonitor` — threshold-based violation-rate alerting (events/sec per topic per kind)
- [x] TSN health dashboard in monitor — `/api/tsn`, `tsn_health` SSE events, `tsn.HealthTracker`
- [x] IDL nested struct CDR encode/decode (v0.13.1)
- [x] IDL array (`T name[N]`), enum, qualified type names, `ddstool idl` CLI (v0.13.2)

**Released — v0.14.x — IDL Completeness, go-FuSa Compliance, Safety Certification Package**

- [x] IDL `go/format` output, `@key` annotation, `typedef`, end-to-end roundtrip harness, `--package` flag (v0.14.0)
- [x] IDL fuzz targets (`FuzzIDLParse`, `FuzzIDLGenerate`) + parseEnum infinite-loop fix (v0.14.1)
- [x] go-FuSa v0.19.0 → v0.21.0 compliance: LINT001/ANA007/CYBER017 fixes; cycle detection in IDL generator (v0.14.1–v0.14.3)
- [x] CI: pinned `gofusa` safety gate (0 errors required to merge) (v0.14.2)
- [x] Release workflow — auto-regenerates FMEA, safety case, SBOM, provenance on every tag (v0.14.3)
- [x] `docs/HARA.md` — tabletop ISO 26262-3 HARA; H-01 late delivery → ASIL-B
- [x] `docs/GC_LATENCY.md` — measured STW pause MAX 146µs, E2E latency MAX 305µs; formal GSN argument
- [x] `cmd/latmon` — continuous rolling-window latency monitor with JSON output
- [x] `cert/` — complete certification package: PSAC, SDP, SVP, SCMP, SQAP, LLR, SCR, DCA, TQPs, PRP, DEVIATIONS, RELEASE_LOG
- [x] `SAFETY_PLAN.md`, `docs/CODING_STANDARD.md`, `docs/STANDARDS_GAP.md` — ISO 26262 / IEC 61508 / DO-178C gap analysis
- [x] 238 requirements, all traced + tested; `gofusa check` 0 errors

See [ROADMAP.md](ROADMAP.md) for goals and sub-items.

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

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md). All commits require a DCO sign-off (`Signed-off-by:`).

## Support

See [SUPPORT.md](SUPPORT.md) for how to get help, report bugs, or report a
security vulnerability.

## License

Mozilla Public License v2.0 — see [LICENSE](LICENSE).  
Copyright (c) 2026 Matt Jones.
