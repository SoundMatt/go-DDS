# go-DDS Roadmap

## Vision

go-DDS is a modern, Go-native data distribution platform built on DDSI-RTPS.

The project focuses on:

- Reliable distributed communication
- Modern developer experience
- Strong observability
- Safety-oriented communication
- Deterministic networking
- Embedded, edge, cloud, and automotive deployments
- Interoperability with existing DDS ecosystems

go-DDS is not intended to replicate every OMG DDS API feature or vendor-specific extension.

Interoperability matters.
Developer productivity matters.
Operational simplicity matters.
API parity does not.

---

## Guiding Principles

1. Pure Go first
2. Standards where they provide value
3. Simplicity over completeness
4. Observability by default
5. Testability by default
6. Safety as a first-class concern
7. TSN as a first-class concern
8. Vehicle data model agnostic
9. Transport-focused rather than data-model focused

---

## Release Plan

| Version | Milestones | Theme |
|---|---|---|
| v0.1 – v0.5 | Released | Foundation through TSN |
| v0.6 | 1, 4 | Production Runtime + Observability ✅ |
| v0.7 | 2, 7 | Developer Experience + Deterministic Networking ✅ |
| v0.8 | 3, 5, 6 | Verification, Edge Performance, Safety ✅ |
| v0.9 | 8, 9, 10 | Enterprise Security, Dynamic Data, Services ✅ |
| v0.9.1 | — | Spec Completeness & Go Idioms ✅ |
| v0.9.2 | 11 (partial) | ProtoCodec, FaultPublisher reorder, fuzz targets, WithContext ✅ |
| v0.10 | 12 (partial), 13 (partial) | Dynamic WaitSet, REST/SSE bridge, Secure SEDP, TypeRegistry, Docker Quickstart ✅ |
| v0.11 | 12, 13 | gRPC bridge, key rotation, bridge networking, interop scene, dev container, GHCR ✅ |
| v0.12 | 2 (partial), 6, 7 (partial), 13 | Examples, Safety completeness (schema validation, metrics, monitor SSE), IDL/CDR, TAPRIO ✅ |
| v0.13.0 | 2, 6, 7 | IDL factory codegen, safety RateMonitor + SSE, TSN health dashboard ✅ |
| v0.13.1 | — | IDL nested struct CDR fix, stale rtps doc removed ✅ |
| v0.13.2 | 2 | IDL array + enum support, ParseFile tests, qualified names, `ddstool idl` CLI ✅ |
| v0.14.0 | 2 | IDL go/format output, @key annotation, typedef, end-to-end round-trip, `--package` flag, IDL fuzz targets ✅ |
| v0.14.1 | — | go-FuSa v0.19.0: LINT001/ANA007/CYBER017 fixes, parseEnum infinite-loop fix, cycle detection in IDL gen ✅ |
| v0.14.2 | — | CI: pinned gofusa v0.19.0 safety gate job ✅ |
| v0.14.3 | — | Release workflow; gitignore check-report.json; gofusa v0.21.0 ✅ |
| v0.14.4 | — | Fix GC latency test races; independence policy (no IV&V required); cert docs updated ✅ |
| v0.14.5 | — | Docker: golang:1.22→1.25 builder; Node.js 24 opt-in for docker/* actions ✅ |
| v0.15.0 | — | go-FuSa v0.25.1; OpenTelemetry adapter (`otel/`); roadmap ✅ audit ✅ |
| v0.16.0 | — | Loaned samples API (`dds.LoaningPublisher`); scenario test runner (`testutil/scenario`) ✅ |
| v0.17.0 | — | Automatic transport selection (`auto/` package — shmem → RTPS fallback) ✅ |
| v0.18.0 | — | Quality polish: rtps/scenario coverage, fusa:req traceability, CONTRIBUTING.md expansion, new examples ✅ |
| v0.19.0 | — | shmem.NewLoaningPublisher (API parity); cdr/xtypes fuzz targets; auto/ coverage 76.9% ✅ |
| v0.20.0 | — | shmem DiscoveryMetrics/TopicMetrics/Health parity; idl/roundtrip coverage 80% ✅ |
| v0.21.0 | — | cyclone optional interface parity; bridge/grpc coverage 75.6%→86.7% ✅ |
| v0.22.0 | — | bridge/rest coverage 78.8%→91.2%; cdr/ coverage 91.7%→94.2% ✅ |
| v0.23.0 | — | idl/ 87.2%→88.1%; idl/roundtrip 80%→85.3%; monitor/ 90.0%→90.5% ✅ |
| v0.24.0 | — | bridge/grpc 86.7%→91.1%; bridge/wan 92.8%→93.6% ✅ |
| v0.25.0 | — | cdr/ 94.2%→100%; bridge/rest 91.2%→98.2%; bridge/mqtt 97.5%→100% ✅ |
| v0.26.0 | — | rpc/ 86.1%→98.1%; cdr/ 99.2%→100%; bridge/rest 95.6%→98.2% ✅ |
| v0.27.0 | — | mock/ 96.4%→98.0%; testutil/scenario/ 86.7%→100%; bridge/wan 93.6%→99.2% ✅ |
| v0.28.0 | — | shmem/ 93.3%→95.2%; monitor/ 90.5%→91.0% ✅ |
| v0.29.0 | — | go-FuSa v0.25.1→v0.30.0 upgrade ✅ |
| v0.30.0 | — | idl/roundtrip 85.3%→96.0%; bridge/grpc 91.1%→92.8%; Docker golang:1.26 + alpine:3.21 ✅ |
| v0.31.0 | — | idl/ 88.4%→92.5%: parser error paths, typedef/struct sub-module resolution, camelCase/toGoName edge cases ✅ |
| v0.32.0 | — | rtps/ 86.8%→87.3%: CDR/locator/submessage error paths, TryRead closed channel, sendHeartbeatLocked, waitDrain ✅ |
| **main** | — | **next coverage target** |
| v1.0 | 14 | TCP/TLS + DTLS transport completeness |
| v1.1 | 15 | Cloud-native runtime (Prometheus, K8s, NAT traversal) |
| v1.2 | 16 | QUIC + WebSocket transports |
| v1.3 | 17 | Robotics (ROS 2 rmw, Zenoh federation) |
| v1.4 | 18 | Aerospace (DDS-FACE, DO-178C DAL-A, ARINC 664) |
| v1.5 | 19 | Platform (RTOS/bare-metal, WebAssembly) |
| v1.6 | 20 | Certification uplift (ASIL-D, IEC 62304, IEC 62443) |

### Released — v0.1 – v0.8

- **v0.1** — Core interfaces, mock runtime, CycloneDDS CGo, pure-Go RTPS/UDP
- **v0.2** — TransientLocal durability, IPv6, interop testing, RTPS protocol completeness
- **v0.3** — Sentinel errors, unicast discovery, content filters, deadline QoS, fragmentation, wildcards, metrics, persistent history, web monitor
- **v0.4** — Back-pressure policies, structured logging, liveliness callbacks, graceful drain, multicast data, shared memory, timestamps, MQTT bridge, typed generics, tracing
- **v0.5** — TSN QoS fields, stream model, PCP/DSCP marking, SO_TXTIME, CLOCK_TAI, per-PCP sockets, SPDP jitter, TSN fragmentation bounds, JSON stream config
- **v0.6** — JSON/YAML config (`config/`), `DiscoveryMetrics`/`TopicMetrics`/`Health` interfaces, per-topic counters in rtps+mock, `WithHeartbeatPeriod`/`WithConfig`, monitor `/health` + `/api/topics` + `/api/diagnostics` + SSE discovery events
- **v0.7** — Test harness helpers (`testutil/`), CLI tool (`cmd/ddstool` — pub/sub/discover), TSN TAPRIO diagnostics (`tsn.HealthTracker`, `tsn.TAPRIOConfig`), CI upgraded to Node.js 24 + golangci-lint v2
- **v0.8** — Topic recording/replay (`record.Recorder`/`Player`/`FaultPublisher`), allocation-free buffer recycling (`pool.BytePool`/`SampleBuffer`), E2E protection header + deterministic queue (`safety.E2EPublisher`/`E2ESubscriber`/`DeterministicQueue`)
- **v0.9** — Enterprise security (`security.CertPlugin`/`AccessPolicy`/`ReplayGuard`), dynamic data (`xtypes.TypeObject`/`DynamicData`/`TypeRegistry`), domain bridge, WAN bridge, admin HTTP API, managed services (`RecorderService`/`ReplayService`/`MonitorService`); `Participant.Domain()`, `Publisher.WriteCtx()`, `Subscriber.Unsubscribe()`, `mock.IsolatedBroker()`, HMAC-authenticated SPDP discovery
- **v0.9.1** — `Sample.SequenceNumber` + `Sample.WriterGUID` on all transports; `Subscriber.TryRead()` non-blocking read; active subscriber Deadline enforcement (`WithDeadlineMissed`); wildcard subscriptions in rtps+shmem; `rpc.Requester[Req,Rep]`/`Replier[Req,Rep]` (OMG DDS-RPC); `GobCodec[T]`; `ErrQoSMismatch`/`ErrDeadlineMissed`/`ErrSampleRejected`/`ErrResourceLimits` sentinels
- **v0.9.2** — `ProtoCodec[T proto.Message]` for Protobuf encoding; `FaultPublisher.ReorderWindow` fault injection mode (shuffles buffered window on fill and flush on Close); `FaultPublisher.WriteCtx` context-aware writes with cancellable delay; `mock.WithContext`/`rtps.WithContext` participant lifecycle tied to `context.Context`; fuzz targets for `security` (HMAC+AES-GCM round-trip and arbitrary-input safety), `rpc` wire format (reply/request dispatch + round-trip), and `ProtoCodec`
- **v0.10** — Dynamic WaitSet (`WaitSet.Attach`/`WaitSet.Detach` with snapshot-based `reflect.Select`); REST/SSE bridge (`bridge/rest` — `GET /topics`, `GET /topics/{t}` SSE, `POST /topics/{t}`, Bearer auth, keepalive); Secure SEDP (`rtps.EndpointPlugin`; `HMACDiscoveryPlugin.SignEndpoint`/`VerifyEndpoint` embeds per-endpoint HMAC-SHA-256 tag in vendor PID 0x8002); `TopicTypeRegistry` (`xtypes.RegisterTopicCodec[T]`, `LookupTopicType`, `GlobalTopicRegistry`); Docker Quickstart (`cmd/monitor`, `examples/quickstart/{pub,sub}`, `docker/Dockerfile`, `docker/docker-compose.yml`)
- **v0.11** — gRPC bridge (`bridge/grpc` — `Subscribe` server-streaming, `Publish` unary, `StreamPublish` client-streaming, Bearer auth interceptors, filter/transform hooks, YAML config via `LoadConfig`/`ApplyConfig`, `JSONCodec`); `HMACDiscoveryPlugin.Rekey` atomic key rotation (RWMutex-safe); Docker bridge networking (`DDS_PEERS`+`WithNoMulticast` across all quickstart binaries, bridge-network compose); `docker/docker-compose.host.yml` Linux override; `docker/compose.interop.yml` CycloneDDS interop scene; `.devcontainer/devcontainer.json` Codespaces/VS Code support; `.github/workflows/docker-publish.yml` multi-arch GHCR publish (linux/amd64 + linux/arm64)
- **v0.12** — Examples (`examples/sensor-pipeline/`, `examples/command-response/`, `examples/secure-topic/`, `examples/taprio-stream/` — each self-contained with `go run .` and its own README); Safety completeness: `E2ESubscriber` schema validation, `safety.Metrics` per-topic violation counters, monitor SSE `safety` event type; IDL compiler (`idl/` — `.idl` → Go struct + `Codec[T]` code generation, CDR/XCDR1 encoding); TAPRIO qdisc configuration (`tsn.TAPRIOConfig.Apply()` via netlink); go-FuSa coverage (280+ requirements, all `[traced+tested]`)
- **v0.13.0** — IDL factory codegen (`NewXxxPublisher`/`NewXxxSubscriber` typed wrappers); `safety.RateMonitor` (threshold-based violation-rate alerting, SSE `safety_metrics` events); TSN health dashboard in monitor (`/api/tsn` endpoint, `tsn_health` SSE events, `tsn.HealthTracker` + `tsn.TAPRIOConfig.VerifyApplied()`); monitor `RegisterSafetyMetrics` / `RegisterTSNHealth` registration APIs
- **v0.13.1** — IDL nested struct CDR encode/decode (recursive field inlining, no `// TODO:` stubs); removed two stale limitation bullets from rtps package doc
- **v0.13.2** — IDL array support (`T name[N]` → Go `[N]T`, CDR encode/decode with range loops); IDL enum support (Go `type E int32` + consts, int32 CDR codec, use as struct field types); `ParseFile` test coverage; qualified type names (`Module::Type`); `ddstool idl` CLI subcommand
- **v0.14.0** — IDL `go/format` output; `@key` annotation support; `typedef` declarations; end-to-end IDL roundtrip test harness; `--package` flag for `ddstool idl`; fuzz targets for `FuzzIDLParse` and `FuzzIDLGenerate`
- **v0.14.1** — go-FuSa v0.19.0 compliance: LINT001/ANA007/CYBER017 fixes across `idl/` and `cmd/ddstool`; parseEnum infinite-loop fix (fuzz corpus entry); cycle detection in IDL code generator (self-referential struct `A{A g}`); `idl/parser.go` refactor (`expectTok`/`expect` split)
- **v0.14.2** — CI: pinned `gofusa@v0.19.0` safety gate job added to `.github/workflows/ci.yml`
- **v0.14.3** — Release workflow (`.github/workflows/release.yml`) regenerates safety artifacts on every `v*` tag; `.gitignore` excludes `check-report.json`
- **v0.14.4** — Fix two data races in `gc_latency_test.go` (payload-reuse + close/send); drop IV&V requirement (structural CI independence documented in `SAFETY_PLAN.md §3.1`); close G-61508-02 and G-DO178-09 in `STANDARDS_GAP.md`
- **v0.14.5** — Docker builder bumped from golang:1.22 to golang:1.25; `FORCE_JAVASCRIPT_ACTIONS_TO_NODE24=true` for docker/* actions
- **v0.15.0** — go-FuSa v0.25.1; `otel/` OpenTelemetry adapter (`NewTracer(tp)` bridges `dds.Tracer` to any OTel provider); roadmap ✅ audit (all implemented items marked)
- **v0.16.0** — `dds.LoaningPublisher` interface with zero-copy loaned-sample API (`mock.NewLoaningPublisher`, `rtps.NewLoaningPublisher`); `testutil/scenario` declarative scenario test runner (Publish, Expect, ExpectNone, Wait, Assert steps); 241 requirements
- **v0.17.0** — `auto.NewParticipant` automatic transport selection (shmem → RTPS fallback, explicit override via `WithTransport`); 244 requirements (REQ-AUTO-001–003 added)
- **v0.18.0** — Quality polish: rtps/ coverage tests (loan, heartbeat, metrics, health, config, IPv6); testutil/scenario coverage tests (error paths, cancellation); `//fusa:req` traceability annotations on 10 foundational types in `dds.go`; `CONTRIBUTING.md` expanded to all 25 packages with contract notes; new examples (loaned-samples, auto-transport, scenario-dsl, otel-tracing); `nolint` explanatory comments
- **v0.19.0** — `shmem.NewLoaningPublisher` (LoaningPublisher parity across all transports); fuzz targets for `cdr/` (`FuzzCDRDecode`, `FuzzCDRRoundtrip`) and `xtypes/` (`FuzzDynamicDataFromJSON`, `FuzzTypeIdentifier`); `auto/` coverage 69.2%→76.9% via `WithRTPSOpts` test; shmem/ coverage at 93.8%
- **v0.20.0** — shmem `DiscoveryMetrics`/`TopicMetrics`/`Health` interface parity with mock/rtps (per-topic `sync.Map` counters, `shmTopicCounter`); `idl/roundtrip` typed-factory coverage 61.3%→80.0%; shmem/ coverage →96%+
- **v0.21.0** — cyclone `DiscoveryMetrics`/`TopicMetrics`/`Health` optional interface parity (stub returns zeros/nil/HealthOK); cyclone interface tests; `bridge/grpc` coverage 75.6%→86.7% (`ApplyConfig`, `qos()`, `NewClient`, `authStream` now tested)
- **v0.26.0** — `rpc/` 86.1%→98.1% (NewRequester/NewReplier publisher+subscriber error paths; Request encode/unmarshal/done-channel errors; Reply encode/write errors; demux/pump short+invalid payload continue branches); `cdr/` 99.2%→100% (ReadString n==0 defensive branch); `bridge/rest` 95.6%→98.2% (SSE channel-close, write-error-on-message, keepalive write error)
- **v0.27.0** — `mock/` 96.4%→98.0% (NewLoaningPublisher publisher error + non-mock participant; TryRead closed-subscriber); `testutil/scenario/` 86.7%→100% (publishStep write error via MaxSampleSize; expectStep timeout + subscriber-closed via pre-closed-channel participant wrapper; waitStep/expectNoneStep ctx-cancel-during-wait; expectNoneStep subscriber error); `bridge/wan/` 93.6%→99.2% (Connect multi-topic cleanup; ErrFrameTooLarge/body-read-error/invalid-JSON via server-side tests; sendLoop writeFrame error + subscriber-channel-closed + writeFrame header error via `net.Pipe` internal test)
- **v0.28.0** — `shmem/` 93.3%→95.2% (deliverSub resetDeadline call; TryRead closed-channel; "#" wildcard match; pattern-longer-than-topic mismatch; NewLoaningPublisher publisher-error path); `monitor/` 90.5%→91.0% (WatchSafety events-channel-closed; handleHealth nil-hp 501 path)
- **v0.29.0** — go-FuSa v0.25.1→v0.30.0 upgrade; `gofusa check ./...` clean at new version
- **v0.30.0** — `idl/roundtrip` 85.3%→96.0% (HeaderCodec/TelemetryCodec Unmarshal truncated-field error paths: string, int64, int32, float64 array, float32, bool, sequence length, sequence element); `bridge/grpc` 91.1%→92.8% (Subscribe channel-close !ok branch; Transform error drops-and-continues; StreamPublish empty-topic-in-stream; Publish/Subscribe closed-participant Internal error); Docker golang:1.25→1.26 + alpine:3.20→3.21

---

## Current State

### Core DDS

- Participant API ✅
- Publisher API ✅
- Subscriber API ✅
- Topic abstraction ✅
- WaitSets ✅
- QoS foundations ✅

### RTPS

- Native RTPS implementation ✅
- Discovery ✅
- Reliability ✅
- UDP transport ✅

### Backends

- Native RTPS ✅
- CycloneDDS ✅
- Mock runtime ✅
- Shared Memory ✅

### Integration

- MQTT bridge ✅
- TSN stream model ✅
- TCP/TLS transport
- DTLS transport
- QUIC transport
- WebSocket transport
- ROS 2 / rmw compatibility
- Zenoh federation
- OPC UA bridge
- SOME/IP bridge
- Prometheus metrics endpoint
- Kubernetes operator

### Platform

- Linux / macOS / Windows ✅
- RTOS / bare-metal (Zephyr, FreeRTOS, NuttX)
- WebAssembly (WASIP1, Cloudflare Workers)
- Android / iOS

### Quality

- Unit testing ✅
- Interoperability testing ✅
- Fuzzing ✅
- CI/CD ✅

### Certification

- ASIL-B SEOOC ✅
- ASIL-D (automotive, ISO 26262)
- DO-178C DAL-A (aerospace)
- IEC 62304 Class C (medical)
- IEC 62443 SL-2 (industrial)
- DDS-FACE TS (aerospace)

---

## Milestone 1 — Production Runtime `v0.6`

Goal:
Deliver a production-grade DDS runtime.

### Discovery

- Robust SPDP ✅
- Robust SEDP ✅
- Discovery diagnostics ✅
- Discovery metrics ✅ (`dds.DiscoveryMetrics`, `DiscoveryMetricsProvider`)
- Discovery health reporting ✅ (`dds.HealthProvider`)

### Reliability

- Complete RTPS reliability ✅
- DATA_FRAG ✅
- Fragmentation ✅
- Reassembly ✅
- Recovery tuning ✅ (`rtps.WithHeartbeatPeriod`)

### Configuration

- YAML configuration ✅ (`config/`)
- JSON configuration ✅ (`config/`)
- Configuration validation ✅

### Operations

- Runtime diagnostics ✅ (`monitor/` — SSE `/api/diagnostics`)
- Runtime inspection ✅ (`monitor/` — `/api/topics`, `/api/peers`)
- Runtime health ✅ (`dds.HealthProvider`, `/health` endpoint)

Success Criteria:
A deployment can be operated and debugged without custom tooling.

---

## Milestone 2 — Developer Experience `v0.7`

Goal:
Make go-DDS the easiest DDS implementation to develop against.

### Code Generation

- IDL parser ✅ (`idl/`)
- Go code generation ✅ (`idl/` — structs, enums, arrays, @key, typedef)
- Typed publishers ✅ (`dds.TypedPublisher[T]`)
- Typed subscribers ✅ (`dds.TypedSubscriber[T]`)

### Serialization

- CDR ✅ (`cdr/`)
- XCDR ✅ (XCDR2 subset via `cdr/`)

### Local Development

- Virtual DDS runtime ✅ (`mock/`)
- In-memory transport ✅ (`mock/`)
- Single-process simulation ✅ (`mock/`)

### Testing

- Mock participants ✅ (`mock/`, `testutil/`)
- Topic simulators ✅ (`testutil/scenario`)
- Test harnesses ✅ (`testutil/` — AssertSample, TopicRecorder, BurstPublish)

### CLI

- Topic inspection ✅ (`ddstool sub`)
- Publish tool ✅ (`ddstool pub`)
- Subscribe tool ✅ (`ddstool sub`)
- Discovery inspection ✅ (`ddstool peers`)

Success Criteria:
A developer can go from interface definition to running system in minutes.

---

## Milestone 3 — Verification and Validation `v0.8`

Goal:
Support the complete software lifecycle from unit testing through system validation.

### Recording

- Topic recording ✅
- RTPS recording _(not targeted — packet-level RTPS capture is out of scope for SEOOC)_
- Metadata recording ✅

### Replay

- Deterministic replay ✅
- Time-scaled replay ✅
- Filtered replay ✅

### Scenario Testing

- Scenario runner ✅ (`testutil/scenario` — Publish, Expect, ExpectNone, Wait, Assert steps)
- Automated test scripts ✅
- Expected behavior validation ✅

### Fault Injection

- Packet loss ✅
- Delay ✅
- Reordering ✅
- Duplication ✅
- Corruption ✅

### Test Reporting

- Coverage integration ✅
- Validation reports ✅ (`cert/SCR.md`, `gofusa safety-case`)
- Test artifacts ✅ (CI uploads coverage, safety-case, FMEA per release)

Success Criteria:
DDS deployments become fully testable and reproducible.

---

## Milestone 4 — Observability `v0.6`

Goal:
Provide first-class visibility into distributed systems.

### Metrics

- Participant metrics ✅
- Topic metrics ✅
- Discovery metrics ✅
- Transport metrics ✅

### Tracing

- OpenTelemetry support ✅ (`otel/` package — `NewTracer(tp)` bridges `dds.Tracer` to any OTel provider)
- Distributed tracing ✅ (`rtps.WithTracer(ddsotel.NewTracer(tp))` propagates spans through dispatch)

### Monitoring

- Health monitoring ✅
- Runtime statistics ✅
- Topology visualization ✅ (monitor SSE + `/api/topics`)

### Dashboard

- Web monitoring ✅
- Traffic inspection ✅
- Performance visualization ✅

Success Criteria:
Engineers can understand system behavior without packet captures.

---

## Milestone 5 — Edge Performance `v0.8`

Goal:
Support high-performance embedded and edge deployments.

### Shared Memory

- Cross-process transport ✅
- Automatic transport selection ✅ (`auto.NewParticipant` — tries shmem, falls back to RTPS/UDP; `WithTransport(TransportShmem|TransportRTPS)` overrides)

### Zero Copy

- Loaned samples ✅ (`dds.LoaningPublisher` interface; `mock.NewLoaningPublisher`, `rtps.NewLoaningPublisher` backed by `pool.BytePool`)
- Preallocated buffers ✅

### Resource Controls

- Bounded queues ✅
- Memory limits ✅ (via Go runtime `GOMEMLIMIT`; documented as AoU-02 in `HARA.md §5`)
- Flow control ✅ (`BackPressurePolicy`: `DropNewest`, `DropOldest`, `Block`)

### Benchmarking

- Latency benchmarks ✅ (`mock/mock_bench_test.go` — `BenchmarkPublish_RoundTrip`)
- Throughput benchmarks ✅ (`BenchmarkPublish_FireAndForget`, `BenchmarkPublish_Parallel`)
- Scalability testing ✅ (`BenchmarkPublish_FanOut`, `BenchmarkPublish_ManyTopics`)

Success Criteria:
High-volume data can be distributed efficiently on constrained hardware.

---

## Milestone 6 — Safety Communication `v0.8`

Goal:
Support safety-oriented communication architectures.

### E2E Protection

- Data IDs ✅
- Source IDs ✅
- Sequence counters ✅
- CRC protection ✅
- Freshness checking ✅
- Schema validation ✅

### Safety Runtime

- Deterministic queues ✅
- Panic containment ✅
- Runtime monitoring ✅ (`cmd/latmon` continuous GC+latency monitor; `safety.RateMonitor`)

### Safety Visibility

- Safety metrics ✅ (`safety.Metrics` per-topic violation counters)
- Safety events ✅
- Safety diagnostics ✅ (`HARA.md`, `GC_LATENCY.md`, `cert/DCA.md`)

### Schema Validation (v0.12)

- Payload type identity embedded in E2E header or out-of-band type check
- `E2ESubscriber` rejects samples whose decoded shape does not match the registered `TypeDescriptor`
- Raises `SafetyEvent` with kind `SchemaViolation`; sample is discarded

### Safety Metrics (v0.12)

- `safety.Metrics` struct: per-topic counters for CRC failures, sequence errors, freshness timeouts, source ID mismatches, schema violations, replay rejections
- `safety.MetricsProvider` interface implemented by `E2ESubscriber` and `ReplayGuard`
- Monitor SSE `safety` event type: streams violation events to the web dashboard in real time

### Documentation

- Safety manual ✅
- Assumptions of use ✅ (`SEOOC.md`)
- Integration guidance ✅ (`SEOOC.md`)

Success Criteria:
Applications can build black-channel safety architectures using go-DDS and observe safety-event rates in real time.

---

## Milestone 7 — Deterministic Networking `v0.7`

Goal:
Become the easiest DDS implementation to deploy on TSN networks.

### TSN Foundations

- VLAN support ✅
- PCP support ✅
- DSCP support ✅
- QoS mapping ✅

### TSN Streams

- Stream descriptors ✅
- Topic mapping ✅
- Configuration validation ✅

### TSN Scheduling

- SO_TXTIME ✅
- CLOCK_TAI ✅
- gPTP awareness ✅
- ETF integration ✅
- TAPRIO integration ✅ (`tsn.TAPRIOConfig.Apply()`, `TAPRIOFromStreams()`)

### TSN Diagnostics

- Timing validation ✅ (`tsn.HealthTracker`)
- Stream health ✅ (`tsn.StreamHealth`, monitor `/api/tsn`)
- Schedule monitoring ✅ (`tsn.TAPRIOConfig.VerifyApplied()`)

Success Criteria:
DDS topics map directly to deterministic TSN streams.

---

## Milestone 8 — Enterprise Security `v0.9`

Goal:
Provide secure communication for enterprise and automotive deployments.

### Authentication

- Certificate identity ✅ (`security.CertPlugin`)
- Mutual authentication ✅ (mTLS via `CertPlugin`)

### Authorization

- Topic permissions ✅ (`security.AccessPolicy`)
- Access policies ✅ (`security.AccessPolicy.Allow`/`Deny`)

### Protection

- Encryption ✅
- Signing ✅
- Replay protection ✅ (`security.ReplayGuard` sliding-window anti-replay)

### Secure Discovery

- Discovery protection ✅ (`HMACDiscoveryPlugin` — SPDP+SEDP signing)
- Identity validation ✅ (`HMACDiscoveryPlugin.VerifyEndpoint`)

Success Criteria:
Deployments can securely operate across untrusted networks.

---

## Milestone 9 — Dynamic Data `v0.9`

Goal:
Support evolving distributed systems.

### XTypes

- TypeObject ✅ (`xtypes.TypeObject`)
- TypeIdentifier ✅ (`xtypes.TypeIdentifier`)
- DynamicData ✅ (`xtypes.DynamicData`)

### Runtime Type Discovery

- Dynamic type inspection ✅ (`xtypes.TypeRegistry`, `LookupTopicType`)
- Compatibility validation ✅ (`xtypes.CheckCompatibility`)

### Evolution

- Forward compatibility ✅ (optional field additions tolerated)
- Backward compatibility ✅ (missing required fields detected)

Success Criteria:
Systems can evolve without lock-step software updates.

---

## Milestone 10 — Enterprise Services `v0.9`

Goal:
Provide operational services around the middleware.

### Routing

- Domain bridge ✅
- WAN bridge ✅
- Protocol bridge ✅ (`bridge/grpc`, `bridge/rest`)

### Administration

- Admin API ✅ (`admin/` HTTP API)
- Remote diagnostics ✅ (monitor `/api/diagnostics`)
- Remote inspection ✅ (monitor `/api/topics`, SSE discovery events)

### Service Framework

- Recorder service ✅ (`RecorderService`)
- Replay service ✅ (`ReplayService`)
- Monitoring service ✅ (`MonitorService`)

Success Criteria:
Large deployments can be operated without custom infrastructure.

---

## Milestone 11 — Spec Completeness & Go Idioms `v0.9.1`

Goal:
Fill the gaps between "works for demos" and "expected by any DDS user or Go developer." Every item here is either mandated by the OMG DDS spec or is a standard idiom in modern Go pub/sub libraries.

### Core DDS (spec-required)

- **Sample metadata** — sequence number, writer GUID, and source timestamp carried on every `Sample`; required by the core DDS data model
- **Transient-local durability** — late-joiner history delivery across transports (already live in mock/shmem; complete for rtps)
- **Deadline QoS enforcement** — active enforcement: publisher deadline-missed event + subscriber deadline-missed event; current callback registration is passive only
- **Content-filtered / wildcard subscriptions** — MQTT-style `+` / `#` wildcard topic matching (already live in mock; extend to rtps and shmem)
- **Request-reply (Requester/Replier)** — OMG DDS-RPC pattern: typed `Requester[Req, Rep]` and `Replier[Req, Rep]` built on two topics; the standard Go-native RPC-over-DDS API

### Go Idioms (expected in any modern Go library)

- **`Subscriber.TryRead()`** — non-blocking read; returns `(Sample, bool)` without blocking on the channel; standard complement to the blocking `C()` channel
- **Richer error sentinels** — `ErrQoSMismatch`, `ErrDeadlineMissed`, `ErrSampleRejected`, `ErrResourceLimits`; `errors.Is`-friendly throughout
- **Protobuf codec** — `proto.Codec[T]` alongside the existing `json.Codec[T]`; industry default for binary pub/sub

Success Criteria:
A developer familiar with the OMG DDS spec or with standard Go networking libraries finds no surprising omissions.

---

## Milestone 12 — Routing, Context API & Secure Discovery `v0.10`

Goal:
Harden the network boundary, make cancellation first-class, and finish the protocol-bridge story started in v0.9.

### Context API

- `context.Context` propagation through `WriteCtx`, `TryRead`, and `WaitSet.Wait`
- Cancellation and deadline respected at every blocking call site
- `WithContext` participant option for domain-scoped cancellation

### Secure Discovery

- SPDP/SEDP message signing and verification (extend the HMAC-authenticated SPDP from v0.9.1)
- Participant identity validation on discovery (reject unknown or untrusted participants)
- Key-rotation support: re-key without participant restart

### Protocol Bridge

- DDS ↔ gRPC gateway: expose any topic as a gRPC streaming RPC and vice-versa
- DDS ↔ REST/SSE gateway: HTTP GET (SSE stream) for subscribe, HTTP POST for publish
- Topic-level filter and transform hooks at the bridge boundary
- Minimal config: single YAML stanza maps a DDS topic to a gRPC service method

### Protobuf Codec

- `proto.Codec[T]` alongside `json.Codec[T]` and `GobCodec[T]`
- Wire format compatible with standard protobuf tooling
- Codec autodiscovery via `TypeRegistry` in `xtypes`

Success Criteria:
Every blocking API call respects `context.Context`; discovery is authenticated end-to-end; a go-DDS topic is reachable from a vanilla gRPC or HTTP client.

---

## Milestone 13 — Docker Quickstart `v0.11`

Goal:
Let a developer experience working multi-process DDS in under two minutes with no Go toolchain required.

### Containers

- Publisher container — runs `ddstool pub` on a configurable topic and payload
- Subscriber container — runs `ddstool sub`, prints arriving samples
- Monitor container — serves the web dashboard on a known port

### Networking

- Docker bridge network with static peer addresses (`WithStaticPeers`) for reliability across macOS/Windows Docker Desktop
- Optional `--network host` mode documented for Linux deployments

### Quickstart surface

- Single `docker compose up` launches all three containers
- Browser URL printed to stdout for the monitor dashboard
- `README` section with copy-paste commands

### Distribution

- Multi-arch images (linux/amd64, linux/arm64)
- Images published to GitHub Container Registry (`ghcr.io/soundmatt/go-dds`)

Success Criteria:
A developer can see DDS samples flowing in a browser within two minutes of cloning the repo.

### Dev Container

- `.devcontainer/devcontainer.json` with Go toolchain, golangci-lint, and recommended VS Code extensions pre-installed
- Works in GitHub Codespaces and VS Code Dev Containers with zero local setup
- Same container image re-used for CI to guarantee dev/CI parity

### Examples

- `examples/` directory with self-contained, runnable programs
- `examples/sensor-pipeline/` — periodic publisher + aggregating subscriber (temperature telemetry pattern)
- `examples/command-response/` — request/reply over two topics (RPC-over-DDS pattern)
- `examples/secure-topic/` — HMAC discovery + AES topic encryption end-to-end
- Each example has its own `README.md` with a `go run .` quickstart

### Interop Scene

- `compose.interop.yml` adds a FastDDS or CycloneDDS container to the quickstart compose file
- Proves wire compatibility: go-DDS publisher, third-party subscriber (and vice-versa) exchange samples on the same topic
- Documents any QoS or transport constraints required for cross-implementation discovery

---

## Milestone 14 — Transport Completeness `v1.0`

Goal:
Make go-DDS reachable across any network boundary — firewalls, NAT, cloud, and secure channels.

### TCP/TLS (RTPS over TCP)

- RTPS framing over TCP (`rtps/transport_tcp.go`) — length-prefixed submessage stream
- TLS 1.3 wrapping via `crypto/tls` — zero external deps
- Participant option `WithTCPAddr(addr)` alongside existing UDP
- Automatic fallback: prefer UDP multicast, fall back to TCP unicast when UDP unreachable
- Discovery over TCP — SPDP unicast to known peers via TCP

### DTLS (Encrypted UDP)

- DTLS 1.3 transport wrapping existing UDP sockets (`rtps/transport_dtls.go`)
- Certificate-based peer authentication (reuses `security.CertPlugin` identity)
- Satisfies OMG DDS Security spec §9.5 "Secure Transport" requirement
- `WithDTLS(tlsCfg)` participant option; compatible with existing shmem and mock backends

### QoS Enforcement — Active Policy

- **Liveliness**: publishers assert liveness on a heartbeat schedule; subscribers raise `ErrLivelinessLost` when a writer goes silent
- **Ownership**: `OwnershipStrength` selects the active writer; lower-strength writers are silenced until primary fails
- **Partition**: logical namespace isolation within a domain — topics only matched when partitions intersect
- **Time-Based Filter**: `MinSeparation` QoS drops samples arriving faster than the configured rate at the subscriber

Success Criteria:
go-DDS participants connect through corporate firewalls and across cloud VPCs without VPN or custom infrastructure.

---

## Milestone 15 — Cloud-Native Runtime `v1.1`

Goal:
First-class Kubernetes and cloud observability — deployable as a standard cloud service with zero custom tooling.

### Prometheus Metrics

- `/metrics` HTTP endpoint on the monitor (Prometheus text format)
- Gauges: active topics, matched readers/writers, participant count
- Counters: samples published/received/dropped, bytes in/out, CDR encode/decode errors
- Histograms: end-to-end latency percentiles (p50/p95/p99), queue depth over time
- `monitor.WithPrometheus(addr)` option; compatible with existing SSE dashboard

### Kubernetes Operator

- CRD `DDSParticipant` — declarative participant config (domain, QoS profile, transport)
- Operator discovers participants in annotated pods and injects domain/peer config via env
- `DDSDomain` CRD: domain-per-namespace isolation with network policy generation
- Helm chart for operator deployment

### NAT Traversal / Cloud Gateway

- TURN-style relay server (`bridge/relay/`) — participants behind NAT register with relay, relay forwards RTPS frames
- STUN-based peer discovery for cloud↔edge pairing
- `WithRelay(relayAddr)` participant option; transparent to application code
- TLS-secured relay channel; relay never decrypts DDS payload

### Content-Filtered Topics

- `NewFilteredSubscriber(topic, expr, params)` — server-side SQL-like predicate (`x > 42 AND status = 'active'`)
- Filter evaluated at publisher before transmission — reduces network load
- Compatible with existing wildcard subscriptions

Success Criteria:
A go-DDS deployment on Kubernetes is observable via standard Prometheus/Grafana and reachable from any cloud region without a VPN.

---

## Milestone 16 — QUIC + WebSocket Transports `v1.2`

Goal:
Connect browsers, edge compute, and congestion-sensitive cloud paths natively.

### QUIC Transport

- RTPS over QUIC streams (`rtps/transport_quic.go`) using `quic-go`
- Reliable streams for SEDP/SPDP discovery; unreliable datagrams for best-effort data
- 0-RTT reconnection — no head-of-line blocking on multi-stream RTPS sessions
- Interoperable with FastDDS QUIC extension (draft spec)
- `WithQUIC(tlsCfg)` participant option

### WebSocket Transport

- RTPS-over-WebSocket (`rtps/transport_ws.go`) — browser and Wasm participants
- JSON and binary (base64-CDR) framing modes
- `bridge/ws/` package: HTTP upgrade handler, participant bridge to RTPS domain
- JavaScript/TypeScript client library (`js/dds-client/`) — TypedPublisher and TypedSubscriber over WebSocket

### WebAssembly Target

- `GOOS=wasip1 GOARCH=wasm` build support for mock and WebSocket transports
- `examples/wasm-subscriber/` — in-browser DDS subscriber compiled with `tinygo` or standard `go build`
- Fastly/Cloudflare Workers deployment guide

Success Criteria:
A browser tab and a cloud function can join a DDS domain alongside embedded devices without a protocol bridge.

---

## Milestone 17 — Robotics Integration `v1.3`

Goal:
Make go-DDS a first-class participant in ROS 2 and modern robotics middleware ecosystems.

### ROS 2 / rmw Compatibility

- Wire-compatible RTPS discovery with ROS 2 participants (FastDDS / CycloneDDS rmw)
- ROS 2 topic naming convention (`/namespace/topic_name`) and type hash interop
- `ros2/` package: `NewROS2Participant` wraps RTPS participant with ROS 2 graph conventions
- `ddstool ros2-list` — list live ROS 2 nodes/topics visible from go-DDS
- Tested against ROS 2 Jazzy and Rolling

### Zenoh Federation

- `bridge/zenoh/` — bidirectional DDS↔Zenoh bridge using `go-zenoh`
- Key expression mapping: DDS topic `sensors/temp` → Zenoh key `dds/sensors/temp`
- QoS mapping: DDS reliability/durability → Zenoh congestion control/history
- Router mode: go-DDS acts as Zenoh router for cross-domain/cross-cloud federation
- `WithZenohFederation(routerAddr)` participant option

### Action Server Pattern

- `dds.ActionServer[Goal, Feedback, Result]` — long-running RPC with streaming feedback
- Built on three topics: `goal`, `feedback`, `result` with correlation IDs
- Cancel support via separate `cancel` topic
- Mirrors ROS 2 action interface for cross-stack compatibility

Success Criteria:
A go-DDS node participates in a live ROS 2 graph and routes data to cloud via Zenoh without a separate bridge process.

---

## Milestone 18 — Aerospace Integration `v1.4`

Goal:
Provide the certification evidence and protocol bridges required for safety-critical aerospace deployments.

### DDS-FACE Profile

- `face/` package implementing the FACE (Future Airborne Capability Environment) Transport Services segment
- FACE UoC (Unit of Conformance) boundary wrapper around `dds.Participant`
- Portability Layer (PL) type mapping between FACE data model and go-DDS CDR types
- DDS-FACE conformance test suite

### DO-178C / DAL-A Safety Evidence

- Structural coverage analysis report (MC/DC — modified condition/decision coverage) targeting DAL-A requirements
- Traceability matrix: high-level requirements → low-level requirements → source code → test cases
- Software Accomplishment Summary (SAS) template pre-filled for go-DDS core packages
- Independence analysis: CI-enforced reviewer separation for safety-critical modules
- Qualified Tool Considerations (QTC) document for go-DDS used as development tool

### ARINC 664 / AFDX Integration

- `tsn/afdx/` package: Virtual Link (VL) descriptor mapping to DDS topic QoS
- BAG (Bandwidth Allocation Gap) enforcement via existing `tsn.TAPRIOConfig`
- Jitter budget tracking per VL using `tsn.HealthTracker`
- `ddstool afdx verify` — validates AFDX VL configuration against loaded topic set

### Redundancy and Fault Tolerance

- Active/standby participant failover (`dds.RedundantParticipant`) with sub-millisecond switchover
- Dual-channel (redundant path) publishing — writes to both channels; subscribers deduplicate
- Built on existing `Ownership` QoS (Milestone 14) with aerospace-grade timing constraints

Success Criteria:
go-DDS can be positioned as evidence-supporting middleware for DO-178C DAL-B/A avionics systems and participates in AFDX Virtual Links.

---

## Milestone 19 — Platform Expansion `v1.5`

Goal:
Run go-DDS on bare-metal microcontrollers, RTOS environments, and browser/edge compute targets.

### RTOS / Bare-Metal

- `rtos/` package: pluggable OS abstraction (goroutine → task, channel → queue, time → RTOS tick)
- FreeRTOS port via `tinygo` — mock transport only (no network stack required for in-core pub/sub)
- Zephyr RTOS port — POSIX sockets + UDP transport on Cortex-M33 and RISC-V targets
- NuttX port — full RTPS over lwIP UDP
- Resource budgets: <32 KB flash, <8 KB RAM for minimal mock participant
- `examples/rtos-sensor/` — FreeRTOS task publishing temperature samples to a desktop subscriber

### WebAssembly (Wasm)

- `GOOS=wasip1` build target for mock + WebSocket transports (no CGo, no RTPS UDP)
- Cloudflare Workers and Fastly Compute deployment examples
- `examples/wasm-subscriber/` — browser DDS subscriber (TypeScript wrapper around Go Wasm module)

### Android / iOS

- `gomobile` bindings for `dds.Participant`, `dds.Publisher`, `dds.Subscriber`
- Swift and Kotlin idiomatic wrappers (`ios/`, `android/`)
- Background service mode: participant survives app backgrounding within platform constraints

Success Criteria:
A Cortex-M33 microcontroller and a cloud function share a DDS domain using the same go-DDS library.

---

## Milestone 20 — Certification Uplift `v1.6`

Goal:
Provide the safety certification evidence required for automotive ASIL-D, medical IEC 62304, industrial IEC 62443, and aerospace FACE profiles.

### ASIL-D Uplift (Automotive)

- Migrate from ASIL-B SEOOC to **ASIL-D** via decomposition: `go-DDS-A` (ASIL-D) + `go-DDS-B` (ASIL-D) dual-channel
- Fault Detection and Notification (FDN): runtime detection of channel divergence
- Freedom From Interference (FFI): memory/timing partition evidence between channels
- Updated HARA, FMEA, and Safety Case for ASIL-D claim
- ISO 26262-6:2018 §9 software unit testing evidence (MC/DC) for all ASIL-D-classified modules

### IEC 62304 — Medical Device Software

- Software Development Plan (SDP) and Software Maintenance Plan (SMP) conforming to IEC 62304 Class C
- Risk Management file linking to IEC 62304 §7.1 (risk analysis) — integrated with existing HARA
- Problem Resolution Record (PRR) process mapped to GitHub Issues workflow
- `cert/IEC62304/` — traceability matrix, SRS, SDS, and test records

### IEC 62443 — Industrial Cybersecurity

- Security Level 2 (SL-2) target: defense against intentional violation with moderate resources
- Security Requirements Specification (SRS) per IEC 62443-3-3
- Threat model update: STRIDE analysis of all external interfaces (RTPS, bridges, admin API)
- Cryptographic module documentation per IEC 62443-4-2 §CR 4.3
- `cert/IEC62443/` — security plan, verification report, penetration test checklist

### FACE Certification Package

- `cert/FACE/` — Unit of Conformance (UoC) registration documentation
- FACE conformance test results for Transport Services segment
- Portability Analysis Report: data model mapping between FACE and DDS-IDL types

Success Criteria:
go-DDS ships with certification packages targeting ASIL-D (automotive), IEC 62304 Class C (medical), IEC 62443 SL-2 (industrial), and FACE TS (aerospace) — covering the four largest safety-regulated embedded industries.

---

## Explicit Non-Goals

### Data Models

These belong elsewhere:

- VSS
- VDM
- S2DM
- Domain-specific schemas

### Data Access APIs

These belong elsewhere:

- VISSR
- REST APIs
- GraphQL APIs

### Control Plane

These belong elsewhere:

- RCP
- Deployment management
- Fleet management
- Configuration distribution

### Legacy DDS Complexity

Not roadmap priorities:

- MultiTopic
- QueryCondition
- StatusCondition hierarchies
- Vendor-specific APIs
- Full OMG API parity

---

## Strategic Differentiators

go-DDS aims to differentiate through:

1. Pure Go implementation — no CGo required for core path
2. Clean developer experience — IDL codegen, typed pub/sub, scenario testing
3. Built-in testing and validation — mock transport, fuzz targets, testutil harnesses
4. Strong observability — OTel, Prometheus, SSE dashboard, distributed tracing
5. Safety-oriented E2E support — ASIL-B SEOOC today, ASIL-D roadmap
6. TSN-native design — TAPRIO, SO_TXTIME, AFDX Virtual Links
7. Embedded-to-cloud deployment model — RTOS → Linux → Kubernetes → cloud
8. Multi-industry certification — automotive, aerospace, medical, industrial evidence packages
9. Modern protocol bridges — gRPC, REST, MQTT, WAN, Zenoh, ROS 2, OPC UA, SOME/IP
10. Transport flexibility — UDP multicast, shmem, TCP/TLS, DTLS, QUIC, WebSocket

The goal is not to become another DDS implementation.

The goal is to become the easiest certified data distribution platform to develop, test, validate, operate, and deploy — from microcontroller to cloud — across every safety-regulated industry.
