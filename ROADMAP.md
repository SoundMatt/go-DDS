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
| v0.33.0 | — | tsn/ 94.4%→100%: TAPRIO Validate overflow + itoa loop body; shmem/ 95.2%→96.2%: readData OS/header/body errors, loop continue ✅ |
| v0.34.0 | — | admin/ 96.1%→100%: handlePublish write/NewPublisher error paths; record/ 98.5%→100%: emitWindow write error, playFiltered pub.Write error; services/ 98.5%→100%: RecorderService.Start cleanup loop ✅ |
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
- **v0.14.4** — Fix two data races in `gc_latency_test.go` (payload-reuse + close/send); drop IV&V requirement (structural CI independence documented in `SAFETY_PLAN.md §3.1`); close G-61508-02 and G-DO178-09 in `docs/STANDARDS_GAP.md`
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

- MQTT bridge — removed in v0.52.0; cross-protocol bridging is now handled by `relay crossbar` (RELAY router). For DDS↔MQTT, run a go-dds spoke and an MQTT spoke under the crossbar.
- TSN stream model ✅
- TCP/TLS transport ✅
- DTLS transport
- QUIC transport
- WebSocket transport
- ROS 2 / rmw compatibility
- Zenoh federation
- OPC UA bridge
- SOME/IP bridge
- Prometheus metrics endpoint ✅ (`observability/monitor` — `/metrics`, `monitor.WithPrometheus(addr)`)
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
- Discovery inspection ✅ (`ddstool discover`)

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
- Memory limits ✅ (via Go runtime `GOMEMLIMIT`; documented as AoU-02 in `docs/HARA.md §5`)
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
- Safety diagnostics ✅ (`docs/HARA.md`, `docs/GC_LATENCY.md`, `cert/DCA.md`)

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
- Assumptions of use ✅ (`docs/SEOOC.md`)
- Integration guidance ✅ (`docs/SEOOC.md`)

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

- Domain bridge — removed in v0.52.0; same-protocol cross-domain forwarding is now an `identity` route in `relay crossbar` / `relay/router` over two go-dds spokes.
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

### TCP/TLS (RTPS over TCP) ✅

- RTPS framing over TCP (`rtps/transport_tcp.go`) — length-prefixed submessage stream ✅
- TLS 1.3 wrapping via `crypto/tls` — zero external deps ✅
- Participant option `WithTCPAddr(addr)` alongside existing UDP ✅
- Automatic fallback: prefer UDP multicast, fall back to TCP unicast when UDP unreachable ✅
- Discovery over TCP — SPDP unicast to known peers via TCP ✅

**Shipped** in #117 (root v0.56.0). `rtps/transport_tcp.go`
adds a `tcpSocket` (listener + per-peer connection cache) that frames every
RTPS message — the exact bytes `wrapInRTPSMessage` already produces for UDP —
with a 4-byte big-endian length prefix, TLS-1.3-wrapped via `crypto/tls` when
`WithTCPTLSConfig` is supplied (enforced when the caller's `MinVersion` is
unset). `WithTCPAddr(addr)` binds the listener; `WithTCPPeers(addrs...)`
gives SPDP a set of known TCP peers so `sendAnnouncement` unicasts every
announcement to them over TCP in addition to the normal UDP multicast send —
this is how a participant behind a UDP-blocking firewall is still
discovered. A single new SPDP parameter (`pidTCPLocator`, a `LocatorKindTCPv4`
locator) lets peers learn each other's TCP listen address; `sendUnicast`, now
the shared unicast-send path for SEDP, ACKNACK/HEARTBEAT, and writer DATA,
prefers that TCP locator over UDP whenever `newMulticastReceiveSocket`
detected no real multicast-capable interface at startup — the same
already-existing signal the UDP transport itself degrades on — falling back
to UDP only if the TCP send fails. Fully additive: with no `WithTCPAddr`,
`preferTCP()` is always false and every send path is byte-for-byte the
pre-Milestone-14 UDP-only behaviour. Proven end-to-end by
`TestTCP_CrossDomain_DiscoveryAndReliableDelivery`, which puts two
participants in different RTPS domains (so UDP multicast SPDP cannot
possibly cross between them) and forces the UDP-unreachable fallback, and
they still discover each other and exchange a reliable sample purely over
TCP.

### DTLS (Encrypted UDP) ✅

- DTLS transport wrapping existing UDP sockets (`rtps/transport_dtls.go`) ✅
- Certificate-based peer authentication (reuses `security.CertPlugin` identity) ✅
- Satisfies OMG DDS Security spec §9.5 "Secure Transport" requirement ✅
- `WithDTLS(tlsCfg)` participant option; compatible with existing shmem and mock backends ✅

**Shipped** in #118 (root v0.57.0). `rtps/transport_dtls.go` adds a
`dtlsSocket` (listener + per-peer connection cache), the DTLS analogue of
`tcpSocket`: since DTLS, like UDP, is datagram- rather than stream-oriented,
no length-prefix framing is needed — one RTPS message maps 1:1 to one DTLS
record. **Scoping note on the version**: Go's standard library `crypto/tls`
implements TLS only — it has no DTLS support at all — and DTLS 1.3 (RFC 9147)
has no production-ready implementation anywhere in the Go ecosystem as of
this writing, including `github.com/pion/dtls`, the most mature Go DTLS
library, whose v3 README still lists DTLS 1.3 under "Planned Features" and
keeps it off the tagged `v3` release line. This transport therefore uses DTLS
1.2 (RFC 6347) via `github.com/pion/dtls/v3` — the one exception to go-DDS's
"stdlib only" transport policy from the TCP/TLS sub-phase above, adopted
because no stdlib (or any tagged, production-ready) alternative exists; it
will move to DTLS 1.3 when a stable implementation ships. `WithDTLS(cfg
*tls.Config)` still takes the same stdlib type `WithTCPTLSConfig` does —
`dtlsServerOptions`/`dtlsClientOptions` translate it into pion/dtls's
recommended functional-options API internally — so a single identity
configures both transports; new `CertPlugin.TLSCertificate()` /
`CertPlugin.CAPool()` accessors (`security/cert.go`) export that identity as
stdlib `tls.Certificate` / `*x509.CertPool` for exactly this purpose.
`WithDTLSAddr(addr)` binds the listener and `WithDTLSPeers(addrs...)` names
peer addresses that must be encrypted; `sendUnicast` — the same shared
unicast-send path TCP hooked into — checks DTLS first (an explicit
confidentiality choice, so it always wins when configured for a peer,
independent of UDP multicast availability), then the TCP fallback, then
plain UDP. Server-side mutual certificate authentication is enforced by
default whenever `ClientCAs` is set and `ClientAuth` is left unset. Fully
additive: with no `WithDTLSAddr`, `dtlsSock` is always nil and every send
path is unchanged. Proven end-to-end by `TestDTLS_TwoParticipants_SameHost`
(two real participants, SPDP discovery over ordinary UDP multicast, every
SEDP/DATA send between them encrypted over DTLS and confirmed via a live
`dtlsSocket` connection) alongside `dtlsSocket`-level unit tests covering
send/receive, oversized-record rejection, mutual-auth enforcement, and
dial-handshake timeout.

### QoS Enforcement — Active Policy ✅

- **Liveliness**: publishers assert liveness on a heartbeat schedule; subscribers raise `ErrLivelinessLost` when a writer goes silent ✅
- **Ownership**: `OwnershipStrength` selects the active writer; lower-strength writers are silenced until primary fails ✅
- **Partition**: logical namespace isolation within a domain — topics only matched when partitions intersect ✅
- **Time-Based Filter**: `MinSeparation` QoS drops samples arriving faster than the configured rate at the subscriber ✅

**Shipped** (root v0.58.0). Adds four new `dds.QoS` fields —
`Liveliness`/`LivelinessLeaseDuration`, `Ownership`/`OwnershipStrength`,
`Partition`, and `MinSeparation` — plus `dds.ErrLivelinessLost` and the
`dds.WithLivelinessLost` subscriber option, implemented identically (down to
shared helper names) in both the `rtps` and `mock` backends so application
code portable between them behaves the same way:
- **Liveliness**: `AutomaticLiveliness` (the default) drives a writer-owned
  ticker (`rtpsWriter.livelinessLoop` / `publisher.livelinessLoop`) that
  asserts on a schedule — in `rtps` via a dedicated liveliness-only HEARTBEAT
  carrying the RTPS spec's "L" flag (`hbFlagLiveliness`, `message.go`), kept
  strictly separate from the reliable retransmission HEARTBEAT so it never
  perturbs `reliable.go`'s tracker. `ManualByTopicLiveliness` starts no
  ticker — only real `Write` traffic touches the lease — so a writer that
  stops publishing is correctly declared lost. Subscribers register via
  `WithLivelinessLost`; a per-writer lease monitor
  (`rtpsReader`/broker `topicLiveliness`) polls and fires the callback with
  `dds.ErrLivelinessLost` semantics, edge-triggered per silence episode.
- **Ownership**: an `ownershipState` (`rtps`) / `mockOwnershipState` (`mock`)
  arbitrates per topic among `ExclusiveOwnership` writers by
  `OwnershipStrength`, ties broken deterministically by GUID. Only the active
  writer's samples reach subscribers; closing it, an SPDP peer eviction, or
  (in `rtps`) a liveliness loss all trigger recomputation, so a lower-strength
  writer transparently takes over.
- **Partition**: `partitionsMatch` implements the standard DDS rule — sets
  must intersect, with the empty set treated as the single default partition
  `""` so two unset-`Partition` endpoints still match each other but never a
  named one. Enforced for same-process pairs (`dispatchToReaders` / the mock
  broker's `publish`) and, in `rtps`, propagated over SEDP via new
  vendor-extension `PL_CDR` parameters (`pidPartition` and friends,
  `cdr.go`/`sedp.go`) so a remote writer/reader pair matches identically to a
  local one.
- **Time-Based Filter**: `MinSeparation` is enforced per (reader, writer)
  pair at delivery time (`rtpsReader.passesTimeBasedFilter` /
  `timeBasedFilterState`), so one fast writer's rate limit never starves
  delivery from another matched writer on the same topic.

Fully additive: every new `QoS` field's zero value reproduces pre-v0.58
behaviour exactly (`SharedOwnership`, no partition filtering,
`AutomaticLiveliness` with no lease, `MinSeparation` disabled). Proven by
`rtps/qos_enforce_test.go` and `mock/qos_enforce_test.go` — same-topic
partition gating, ownership failover on `Close`, automatic-vs-manual
liveliness (stays alive under an active ticker; fires when a manual writer
goes silent), and time-based-filter drop/pass behaviour, for both backends.

Success Criteria:
go-DDS participants connect through corporate firewalls and across cloud VPCs without VPN or custom infrastructure.

Milestone 14 is now complete: TCP/TLS (#117), DTLS (#118), and QoS
Enforcement — Active Policy (this release) are all shipped.

---

## Milestone 15 — Cloud-Native Runtime `v1.1`

Goal:
First-class Kubernetes and cloud observability — deployable as a standard cloud service with zero custom tooling.

### Prometheus Metrics ✅

- `/metrics` HTTP endpoint on the monitor (Prometheus text format) ✅
- Gauges: active topics, matched readers/writers, participant count ✅
- Counters: samples published/received/dropped, bytes in/out, CDR encode/decode errors ✅
- Histograms: end-to-end latency percentiles (p50/p95/p99), queue depth over time ✅
- `monitor.WithPrometheus(addr)` option; compatible with existing SSE dashboard ✅

**Shipped** (`observability/monitor/prometheus.go`). Hand-rolled Prometheus
text exposition — no `github.com/prometheus/client_golang` dependency — per
the "Pure Go first" guiding principle; the exposition format itself is a
handful of `# HELP`/`# TYPE`/value lines per metric. `GET /metrics` is
registered on the monitor's existing `http.ServeMux` alongside `/`, `/events`,
`/health`, and the `/api/*` routes, so it's live with zero extra
configuration; `mon.WithPrometheus(addr)` additionally starts a second,
dedicated `http.Server` serving only `/metrics` on its own address (`Close`
shuts both down) — the common Kubernetes convention of scraping metrics off
a port separate from the application's primary port.
- **Gauges/counters derived live from existing providers** — `dds_active_topics`
  (`len(TopicMetricsProvider.TopicMetrics())`), `dds_participant_count`
  (`DiscoveryMetricsProvider.DiscoveryMetrics().PeersKnown + 1`), and
  `dds_samples_{published,received,dropped}_total` /
  `dds_bytes_{out,in}_total` (`MetricsProvider.Metrics()`) reuse the exact
  same three optional interfaces `New` already type-asserts for the SSE
  dashboard — one source of truth, two exposition formats, "compatible with
  existing SSE dashboard" by construction rather than by convention.
- **No existing provider produces matched-endpoint counts, CDR error counts,
  or latency/queue-depth samples** — nothing in `dds`, `mock`, `rtps`,
  `shmem`, or `cdr` tracks these today. Rather than an invasive
  instrumentation pass across every transport backend in this sub-phase,
  `dds_matched_readers`/`dds_matched_writers`/
  `dds_cdr_{encode,decode}_errors_total`/`dds_latency_seconds`/
  `dds_queue_depth` are exposed as `Monitor` hook methods (`SetMatched`,
  `IncCDREncodeError`, `IncCDRDecodeError`, `ObserveLatency`,
  `ObserveQueueDepth`) that default to zero until called — the same
  post-`New()` registration pattern `RegisterSafetyMetrics` and
  `RegisterTSNHealth` already use. Wiring these into the transport/codec
  layers themselves is left as future instrumentation work.
- **Histograms are real Prometheus cumulative histograms**
  (`promHistogram` — bucketed counts + `_sum` + `_count`), not
  client-side-computed percentiles: `histogram_quantile()` on the
  Prometheus/Grafana side computes p50/p95/p99 from the exposed buckets, per
  standard Prometheus practice, so go-DDS itself never computes a
  percentile.

Proven by `observability/monitor/prometheus_test.go` — full-body
`/metrics` content assertions (all gauge/counter/histogram series present
with correct `# TYPE`), the nil-provider zero-value path, `SetMatched`/
`IncCDR*`/`Observe*` hook values round-tripping through the exposed text,
histogram bucket placement including `+Inf` overflow, the dedicated
`WithPrometheus` server (separate address, `/metrics`-only, shut down by
`Close`), and its listen-error path.

### Kubernetes Operator ✅

- CRD `DDSParticipant` — declarative participant config (domain, QoS profile, transport) ✅
- Operator discovers participants in annotated pods and injects domain/peer config via env ✅
- `DDSDomain` CRD: domain-per-namespace isolation with network policy generation ✅
- Helm chart for operator deployment ✅

**Shipped** (`k8s/operator/`, its own Go module —
`github.com/SoundMatt/go-DDS/k8s/operator` — same "independent go.mod"
convention as `bridge`/`tools`/`observability`/`examples`/`safety`, but with
no `replace` back to the root module: it only talks to the Kubernetes API,
never the root `github.com/SoundMatt/go-DDS` package). Both CRDs
(`config/crd/*.yaml`) are read via the dynamic client as
`unstructured.Unstructured` and converted to/from plain Go structs
(`api/v1alpha1`) with `runtime.DefaultUnstructuredConverter` rather than
through a `k8s.io/code-generator`-generated typed clientset — the same
"pure Go first" reasoning as the hand-rolled Prometheus exposition above:
no generated glue, buildable with nothing but `go build`.
- **Injection is a mutating admission webhook** (`internal/webhook`), not a
  controller that edits running pods (Kubernetes doesn't allow mutating a
  running container's env anyway): a pod annotated
  `dds.soundmatt.io/participant: <name>` gets `DDS_DOMAIN_ID`,
  `DDS_QOS_PROFILE`, `DDS_TRANSPORT`, `DDS_TRANSPORT_PORT`, and `DDS_PEERS`
  patched into its matched containers (`dds.soundmatt.io/inject-containers`,
  default all) at admission time by `internal/inject`'s pure
  spec-to-JSON-Patch computation. Explicit container env always wins over
  injection, and a `DDSParticipant.spec.domain` always wins over its
  namespace's `DDSDomain.spec.domainID` (the CRD is a namespace-wide
  default, not an override).
- **Fails open.** A pod referencing a not-yet-cached or nonexistent
  `DDSParticipant` is still admitted, with a warning rather than a denial —
  see `internal/webhook`'s package doc: an operator hiccup must never be
  able to block unrelated workloads from scheduling. The Helm chart's
  default `webhook.failurePolicy: Ignore` reinforces this Kubernetes-side
  too.
- **`DDSDomain` reconciliation** (`internal/controller.DomainReconciler`)
  renders an isolated domain's NetworkPolicy from the RTPS well-known UDP
  port formulas (DDSI-RTPS §9.6.1.1) via `internal/netpolicy`, scoped to the
  domain's own namespace plus `spec.allowedNamespaces`, kept in sync
  (create/update/delete) via a work-queue-driven informer event loop.
- **Helm chart** (`deploy/helm/go-dds-operator/`) installs both CRDs, RBAC,
  the operator Deployment/Service, and a `MutatingWebhookConfiguration`
  whose TLS certificate is self-signed at install/upgrade time via Helm's
  `genCA`/`genSignedCert` — zero extra tooling (no cert-manager dependency)
  to satisfy this milestone's "zero custom tooling" success criterion;
  `values.yaml`'s `tls.selfSigned: false` plus `tls.certManagerCertificateName`
  switches to a cert-manager-issued certificate for production use.
- A fourth Docker image (`operator`, `docker/Dockerfile`) is now published
  as `ghcr.io/soundmatt/go-dds-operator` alongside `monitor`/`pub`/`sub`,
  matching the Helm chart's default `image.repository`.

Proven by unit tests across every package with no live cluster required:
`internal/inject` and `internal/netpolicy` are pure functions tested
directly; `internal/webhook` is tested via `httptest` against real
`admission/v1` `AdmissionReview` JSON (including the fail-open paths);
`internal/cache` and `internal/controller` are tested against a real
`client-go` informer wired to `k8s.io/client-go/dynamic/fake` and
`k8s.io/client-go/kubernetes/fake` — the same fake-clientset pattern
client-go itself uses, proving the informer-to-cache and
DDSDomain-to-NetworkPolicy reconcile paths end-to-end without a live
`kube-apiserver` (out of scope for this repo's CI, same reasoning as the
existing `interop/` package's build-tag-gated live-peer tests). The Helm
chart is proven with `helm lint` and `helm template` across the
`installCRDs`/`tls.selfSigned` value combinations, including asserting the
generated webhook Secret's `ca.crt` matches the
`MutatingWebhookConfiguration`'s `caBundle` byte-for-byte.

### NAT Traversal / Cloud Gateway ✅

- TURN-style relay server (`bridge/relay/`) — participants behind NAT register with relay, relay forwards RTPS frames ✅
- STUN-based peer discovery for cloud↔edge pairing ✅
- `WithRelay(relayAddr)` participant option; transparent to application code ✅ (shipped as `rtps.WithRelayAddr`, matching the existing `WithTCPAddr`/`WithDTLSAddr` naming convention rather than a single generic `WithRelay`)
- TLS-secured relay channel; relay never decrypts DDS payload ✅

**Shipped** (`bridge/relay/` — a new package in the `bridge` module — plus
`rtps/transport_relay.go` and `rtps/stun.go` in the root module). The relay
server (`relay.Serve`) is a TURN-style hub: participants make one outbound
TLS connection, register under a stable ID, and the server forwards opaque
length-prefixed frames between any two registered IDs, exactly like the
existing `bridge/wan`/RTPS-over-TCP framing but addressed by ID instead of
"host:port" — the ID-addressed design needed because a NATted participant
generally has no reachable address at all, which is the whole problem this
sub-phase solves. `relay.Discover` implements RFC 5389 STUN Binding
Request/Response for server-reflexive address discovery, used for
cloud↔edge pairing alongside (not instead of) the relay — the standard
STUN-then-TURN pattern.
- `rtps.WithRelayAddr(addr)` dials the relay and registers this
  participant's GUID prefix (hex-encoded) as its relay ID;
  `WithRelayTLSConfig` secures that connection; `WithRelayPeers(ids...)`
  additionally unicasts SPDP announcements over the relay to known peer IDs
  (mirroring `WithTCPPeers`); `WithSTUNServer(addr)` performs best-effort
  STUN discovery at startup, exposed via the new `dds.PublicAddresser`
  optional interface. Once two participants have discovered each other —
  via `WithRelayPeers` or from any relayed traffic at all — every unicast
  SEDP/DATA/ACKNACK/HEARTBEAT send that would otherwise require a directly
  reachable address instead routes over the relay automatically, including
  overriding the UDP-multicast fast path for a relay-only matched reader:
  fully transparent to application code, with no import of `bridge/relay`
  from the root module (ROADMAP.md's "Architecture Initiative", #71:
  submodules depend on root, never the reverse — the root module
  independently reimplements the client side of the same wire protocol,
  the same precedent `rtps/transport_tcp.go`/`bridge/wan` already set for
  length-prefixed framing).
- The relay only ever reads a 1-byte frame type and a short ID field to
  decide where to forward a frame; it never parses or decrypts the RTPS
  payload itself, so DDS payload confidentiality end-to-end (via
  `WithSecurity`) is preserved through the relay hop — the relay operator
  never gains anything a `WithTCPAddr` operator wouldn't already see at the
  RTPS-message level.

Proven by `bridge/relay`'s own unit tests (frame forwarding, unknown-target
errors, re-registration, TLS, and a real RFC 5389 STUN client/server round
trip against a local fake STUN responder) and `rtps/transport_relay_test.go`
(`relaySocket` send/receive/TLS/close, `relayID`↔`GuidPrefix` round-tripping,
and — gated behind `-short` exactly like the existing
`TestDTLS_TwoParticipants_SameHost`/`TestRTPS_TwoParticipants_SameHost` —
two real participants publishing/subscribing entirely over a relay with no
other transport configured, plus a dedicated regression test proving a
relay-only matched reader still receives samples even when UDP multicast is
otherwise available).

### Content-Filtered Topics ✅

- `NewFilteredSubscriber(topic, expr, params)` — server-side SQL-like predicate (`x > 42 AND status = 'active'`) ✅
- Filter evaluated at publisher before transmission — reduces network load ✅
- Compatible with existing wildcard subscriptions ✅

**Shipped** (new `cfilter/` package, plus `dds.NewFilteredSubscriber` and a
`mock`/`rtps`/`shmem` implementation each). `cfilter.Parse(expr, params)`
compiles a small DDS-SQL-like predicate grammar — comparisons (`=`, `<>`,
`!=`, `>`, `>=`, `<`, `<=`) combined with `AND`/`OR`/`NOT` and parentheses,
plus `%0`, `%1`, ... parameter placeholders — into a `cfilter.Expr` that
evaluates against a sample's Payload decoded as a JSON object; a decode
failure, an absent field, or a type-mismatched comparison all fail closed
(evaluate false) rather than erroring, since content filtering is a
network-load optimisation, never a correctness requirement. `cfilter` has no
dependency on `dds` or any transport backend, so `mock`, `rtps`, and `shmem`
all import and share the exact same predicate implementation — the same
"identical semantics across backends" convention the wildcard matcher and
Milestone 14's QoS enforcement already established — proven by parallel
`content_filter_test.go` suites in all three packages plus `cfilter`'s own
~96%-covered unit tests.
- **`dds.ContentFilteredSubscriberFactory`** is a new optional interface
  (the same `Drainer`/`PublicAddresser`/`DiscoveryMetricsProvider` pattern
  already used throughout `dds.go`) with one method,
  `NewFilteredSubscriber(topic, expr string, params []string, qos QoS, opts ...SubscriberOption) (Subscriber, error)`;
  `dds.NewFilteredSubscriber(p, ...)` type-asserts it and returns
  `ErrContentFilterUnsupported` otherwise. `topic` may itself be an
  MQTT-style `+`/`#` wildcard pattern exactly as `NewSubscriber` accepts,
  composing directly with Milestone 11's wildcard subscriptions.
- **mock and shmem** compile `expr` once at subscribe time and store the
  predicate on the subscription; each backend's central `broker.publish`
  (the same synchronous call `Publisher.Write` already funnels through)
  checks it immediately before enqueuing to the subscriber's channel —
  exactly where `WithFilter`'s existing `func(Sample) bool` is already
  checked, so the two compose (both must pass) rather than replace one
  another. shmem additionally threads the predicate into `shmListener` for
  cross-process delivery, since a cross-process shmem reader reads its
  payload directly from the shared-memory file rather than through the
  local broker.
- **rtps is the transport where "reduces network load" is a literal,
  measurable claim**: a `NewFilteredSubscriber` reader's predicate text and
  parameters are carried over SEDP on two new vendor PL_CDR parameters
  (`pidContentFilterExpr`, `pidContentFilterParam`, mirroring
  `pidPartition`'s repeated-string encoding) on the subscription
  announcement. A matched remote writer compiles the received expression
  and, in `rtpsWriter.Write`, evaluates it against the plaintext payload
  before ever building or sending a DATA submessage to that reader's
  locator — a non-matching sample is never serialized, fragmented, or put
  on the wire for that reader. Because RTPS multicast reaches every
  subscriber on the segment unconditionally, a content-filtered matched
  reader forces the writer onto the per-reader unicast path exactly like an
  existing relay-only reader already does (`matchedReaderLocators`); two
  local readers in the same remote participant process necessarily share
  one data locator (it's per-participant, not per-reader), so
  `matchedReaderLocators` OR-merges every reader's predicate at a shared
  locator — the locator is sent to whenever *any* reader there wants the
  sample — and the remote participant's own `dispatchToReaders` re-checks
  each individual reader's predicate to finish sorting delivery locally,
  so an unfiltered reader is never starved by a filtered peer sharing its
  locator. A same-process (local) reader is filtered in `dispatchToReaders`
  the same way `WithFilter` already is, since the network path never runs
  for local delivery.

Proven by `cfilter`'s own unit tests (grammar coverage, fail-closed
semantics, parameter binding) and parallel `content_filter_test.go` suites
in `mock/`, `rtps/`, and `shmem/`: match/reject, invalid-expression and
empty-topic errors, parameter binding, and wildcard-topic compatibility in
all three, plus `dds.ContentFilteredSubscriberFactory` type-assertion
checks. `rtps/content_filter_test.go` additionally runs real two-participant
cross-process scenarios (gated like the existing
`TestRTPS_TwoParticipants_SameHost`) proving a non-matching sample is never
delivered end-to-end over real UDP, and that a plain (unfiltered)
subscriber sharing a writer with a filtered one is never affected by the
other's predicate.

Success Criteria:
A go-DDS deployment on Kubernetes is observable via standard Prometheus/Grafana and reachable from any cloud region without a VPN.

---

## Milestone 16 — QUIC + WebSocket Transports `v1.2`

Goal:
Connect browsers, edge compute, and congestion-sensitive cloud paths natively.

### QUIC Transport ✅

- RTPS over QUIC streams (`rtps/transport_quic.go`) using `quic-go` ✅
- Reliable streams for SEDP/SPDP discovery; unreliable datagrams for best-effort data ✅
- 0-RTT reconnection — no head-of-line blocking on multi-stream RTPS sessions ✅
- Interoperable with FastDDS QUIC extension (draft spec) ✅ (see the scoping note below)
- `WithQUIC(tlsCfg)` participant option ✅ (shipped as `WithQUICAddr`/`WithQUIC`/`WithQUICPeers`, matching the existing `WithTCPAddr`/`WithTCPTLSConfig`/`WithTCPPeers` and `WithDTLSAddr`/`WithDTLS`/`WithDTLSPeers` three-option naming convention rather than a single option)

**Shipped**. `rtps/transport_quic.go` adds a `quicSocket` — the QUIC analogue
of `tcpSocket`/`dtlsSocket` — built on `github.com/quic-go/quic-go`. Each
peer connection carries two genuinely independent channels rather than one
shared byte stream: a single reliable, length-prefixed bidirectional stream
(the same 4-byte big-endian framing `tcpSocket` uses) for SPDP/SEDP
discovery and any reliable-QoS traffic, and unreliable QUIC DATAGRAM frames
(RFC 9221) for best-effort user DATA. `participant.sendUnicast` gained a
`reliable bool` parameter — `true` at every existing call site except
`rtpsWriter.Write`'s initial send, where it is simply `w.reliable`, i.e. the
writer's own QoS.Reliability — so a best-effort writer's samples ride the
cheap datagram path while SPDP, SEDP, HEARTBEAT/ACKNACK, and reliable DATA
always use the stream. A best-effort message too large for a single
datagram at the current path MTU falls back to the stream automatically
(`quic.DatagramTooLargeError`) rather than being dropped. Because the two
channels are transport-level siblings, not multiplexed through one ordered
byte stream the way RTPS-over-TCP necessarily is, a lossy or bursty run of
best-effort samples can never head-of-line-block a concurrent SPDP/SEDP
exchange or reliable retransmission on the same QUIC session — the
multi-stream property this sub-phase's ROADMAP.md bullet calls out.
0-RTT reconnection comes from `quic.DialAddrEarly`/`quic.ListenAddrEarly`
plus a default `tls.ClientSessionCache` `quicTLSConfig` installs whenever
`WithQUIC`'s config leaves one unset, so a redial to a previously-contacted
peer (e.g. after a transient network drop) can resume without a full
handshake. `WithQUICAddr(addr)` binds the listener, `WithQUIC(cfg)` supplies
the (always-required, since QUIC mandates TLS 1.3) `*tls.Config` — the same
stdlib type `WithTCPTLSConfig`/`WithDTLS` accept — and `WithQUICPeers(addrs...)`
seeds static peers for SPDP-over-QUIC exactly as `WithTCPPeers` does; a new
`pidQUICLocator` SPDP parameter (`LocatorKindQUICv4`) then lets peers learn
each other's QUIC listen address for ongoing SEDP/user-data unicast, mirroring
`pidTCPLocator`. In `participant.sendUnicast`'s transport-priority chain,
QUIC is checked right after DTLS (an explicit peer-configured choice
preferred unconditionally, like DTLS, rather than gated on UDP multicast
being unavailable the way the older TCP fallback is — QUIC's per-connection
congestion control is the better default for the "congestion-sensitive cloud
paths" this milestone's goal names, not just a firewall-traversal fallback),
falling back to TCP, then relay, then plain UDP if its own send fails.
**Scoping note on FastDDS interoperability**: the FastDDS QUIC transport
extension is, as of this writing, an evolving draft with no published fixed
ALPN token or wire-framing spec to conform to byte-for-byte — the same kind
of gap #118 documented for DTLS 1.3. This transport's framing is a
defensible reading of "RTPS messages over QUIC streams and datagrams," not a
claim of byte-for-byte conformance to that draft; `WithQUIC`'s
`tlsCfg.NextProtos` can be overridden directly to match a specific FastDDS
build's ALPN once the draft stabilises, and `interop/` (already the
ecosystem's live-peer wire-compatibility testing pattern) is where a future
round can add a real FastDDS QUIC peer once there is one to test against.
Fully additive: with no `WithQUICAddr`, `quicSock` is always nil and every
send path is unchanged from before this sub-phase. Proven end-to-end by
`TestQUIC_CrossDomain_DiscoveryAndDelivery` — two participants in different
RTPS domains (so UDP multicast SPDP cannot cross between them), with UDP
multicast forced unavailable and user-data multicast disabled, discover each
other purely via SPDP-over-QUIC, match endpoints via SEDP-over-QUIC, and
exchange both a best-effort sample (over the datagram channel) and a
reliable sample (over the stream, with HEARTBEAT/ACKNACK also flowing over
QUIC) — alongside `quicSocket`-level unit tests covering reliable-stream and
best-effort-datagram send/receive, the datagram-too-large stream fallback,
oversized-frame rejection, and dial-handshake timeout.

### WebSocket Transport ✅

- RTPS-over-WebSocket (`rtps/transport_ws.go`) — browser and Wasm participants ✅
- JSON and binary (base64-CDR) framing modes ✅
- `bridge/ws/` package: HTTP upgrade handler, participant bridge to RTPS domain ✅
- JavaScript/TypeScript client library (`js/dds-client/`) — TypedPublisher and TypedSubscriber over WebSocket ✅

**Shipped**. `rtps/transport_ws.go` adds a `wsSocket` — the WebSocket
analogue of `tcpSocket`/`quicSocket` — implementing the RFC 6455 opening
handshake and frame codec itself with nothing but the standard library
(`net/http`'s request/response parsers plus a small frame reader/writer; no
external dependency, the same choice `transport_tcp.go` makes for its own
TLS wrapping). Unlike `tcpSocket`, no additional length-prefix framing is
layered on top: a WebSocket connection is already message-oriented, so one
RTPS message maps directly to one WebSocket message, sent either as a
binary frame carrying the raw bytes (`wsFramingBinary`, the default) or a
text frame carrying `{"data":"<base64 CDR>"}` (`wsFramingJSON`) — the "JSON
and binary (base64-CDR) framing modes" bullet; a socket's framing setting
only controls what it *sends*, every inbound message is decoded by its own
opcode, so the two modes freely interoperate on one connection.
`WithWSAddr(addr)` binds the listener, `WithWSTLSConfig(cfg)` optionally
wraps it in TLS 1.3 (`wss://` — unlike QUIC/DTLS, WS does not mandate TLS,
so this one is optional, matching `WithTCPTLSConfig`'s own optionality
rather than `WithQUIC`/`WithDTLS`'s required config), and `WithWSPeers(addrs...)`
seeds static peers for SPDP-over-WS exactly as `WithTCPPeers`/`WithQUICPeers`
do; a new `pidWSLocator` SPDP parameter (`LocatorKindWSv4`) lets peers learn
each other's WS listen address for ongoing SEDP/user-data unicast, mirroring
`pidTCPLocator`/`pidQUICLocator`. In `participant.sendUnicast`'s
transport-priority chain, WS is checked right after QUIC — unconditionally
preferred when a peer's WS locator is known, not gated on UDP multicast
availability, since a browser or Wasm peer's WS locator is very often the
*only* transport that peer has, not a firewall-traversal fallback — falling
back to TCP, then relay, then plain UDP if its own send fails. Fully
additive: with no `WithWSAddr`, `wsSock` is always nil and every send path
is unchanged from before this sub-phase. Proven end-to-end by
`TestWS_CrossDomain_DiscoveryAndDelivery` — two participants in different
RTPS domains, with UDP multicast forced unavailable and user-data multicast
disabled, discover each other purely via SPDP-over-WS and exchange a
reliable sample with HEARTBEAT/ACKNACK also flowing over WS — alongside
`wsSocket`/frame-codec unit tests covering plaintext and TLS send/receive,
both framing modes (including a socket configured for one mode correctly
receiving the other), Ping/Pong handling, a raw-client RFC 6455 handshake
check, and a hostile claimed-frame-length rejection.

`bridge/ws/` adds a second, optional gateway (`ws.Bridge`, mirroring
`bridge/rest`'s HTTP/SSE gateway) for clients that would rather speak a
small JSON pub/sub protocol — `{"op":"subscribe"|"unsubscribe"|"publish", ...}`
in, `{"op":"sample"|"error"|"subscribed"|"unsubscribed", ...}` out, one
message per WebSocket frame — than implement RTPS discovery themselves; its
`Bridge.authorize` accepts the token via `?token=` query parameter as well
as the standard `Authorization: Bearer` header, since a browser's own
JavaScript cannot set arbitrary headers on a WebSocket handshake.
`js/dds-client/` is the TypeScript client for that gateway: a
dependency-free `DDSClient` (automatic reconnection with resubscription,
a structurally-typed `WebSocketLike` so it runs against any runtime's
native `WebSocket` — browser, Node ≥ 18, or an injected `webSocketFactory`)
plus `TypedPublisher<T>`/`TypedSubscriber<T>`, JSON-encoding by default via
a pluggable `Codec<T>`. Its README documents the scope boundary explicitly:
`js/dds-client` speaks `bridge/ws`'s gateway protocol, not raw RTPS —
a Wasm-compiled go-DDS build dialling `WithWSAddr` directly (the sibling
WebAssembly Target sub-phase, still open) is the no-bridge path this
milestone's success criterion names for a browser tab that needs to be a
genuine RTPS participant. Verified end-to-end against a real `bridge/ws`
Go server from Node (in addition to its own in-memory-fake-WebSocket unit
suite), including the query-parameter auth path.

### WebAssembly Target ✅

- `GOOS=wasip1 GOARCH=wasm` build support for mock and WebSocket transports ✅
- `examples/wasm-subscriber/` — in-browser DDS subscriber compiled with `tinygo` or standard `go build` ✅
- Fastly/Cloudflare Workers deployment guide ✅

**Shipped**. Two independent gaps stood between "the WS transport builds
for Wasm" (already true — `mock` and `rtps/transport_ws.go` are pure Go
with no CGo, so they always compiled for both `GOOS=wasip1 GOARCH=wasm` and
`GOOS=js GOARCH=wasm`) and this milestone's actual success criterion — a
browser tab or cloud function acting as a *genuine RTPS participant*, not
merely a binary that happens to compile:

1. **A browser can never accept an inbound connection.** Every other
   optional transport (TCP/DTLS/QUIC/WS) required `WithWSAddr`-style binding
   a listener before `WithWSPeers`-style static peers had any effect —
   fine for a normal server, impossible for a sandboxed browser tab or a
   serverless function invoked per-request. `newWSSocket` now accepts an
   empty listen address and, in that case, skips `net.Listen`/`acceptLoop`
   entirely (`ln` stays nil, `port` stays 0) while still dialling outbound
   and exchanging RTPS messages normally; `WithWSPeers` alone (no
   `WithWSAddr`) now enables the transport in this dial-only mode rather
   than being a no-op (see that option's updated doc comment in
   `rtps/participant.go`). A dial-only participant advertises no
   `pidWSLocator` at all (spdp.go skips it for `port == 0` — there is no
   address for a peer to dial back into), so `wsLocatorForIP` gained a
   fallback to `wsSocket.cachedConnForIP`: replies and further SEDP/
   user-data traffic route back over the exact connection the dial-only
   peer already established, never requiring a fresh inbound dial. And
   because TCP/QUIC/DTLS discovery assumes *both* sides know each other's
   static peer address ahead of time — impossible for the listening side,
   which has no way to name a browser tab's address — `wsReceiveLoop`
   gained `wsReplyToNewWSPeer`: the first time a WS-sourced packet
   introduces a GUID prefix this participant has never seen before, it
   replies immediately, once, with its own current SPDP announcement
   (`spdpService.buildAnnouncementMessage`, factored out of
   `sendAnnouncement`) over that same connection — closing the one-sided
   discovery gap a listener-less peer would otherwise create. Proven by
   `TestWS_DialOnlyParticipant_DiscoveryAndDelivery`: a dial-only
   participant (`WithWSPeers`, no `WithWSAddr` — the browser/edge-function
   shape) completes SPDP/SEDP discovery and reliable delivery against a
   normal listening peer, alongside dedicated tests for listener-less
   socket construction/close-safety and `WithWSPeers`-alone participant
   creation.
2. **`GOOS=js GOARCH=wasm` (the actual browser target) has no real
   networking at all.** The stdlib's own `net` package there is, in its
   words, "fake networking for js/wasm... intended to allow tests of other
   packages to pass" (`$GOROOT/src/net/fd_js.go`) — genuinely useful for
   letting other packages' tests build and pass, but no path to a real
   remote host, because a browser sandbox does not expose raw TCP sockets
   to any script running inside it. `transport_ws_dial.go` (build-tag
   `!(js && wasm)`) keeps the original `net.Conn` + RFC 6455 handshake dial
   exactly as shipped for "WebSocket Transport" above — unchanged, still
   what every other platform (including `wasip1`) uses — while
   `transport_ws_browser.go` (build-tag `js && wasm`) adds a second dial
   backend that asks the browser's own, already RFC-6455-compliant
   `WebSocket` object to do it via `syscall/js`, wrapped in a `wsConnIface`
   (`readMessage`/`writeMessage`/`close`) both backends implement
   identically from `wsSocket`'s point of view — no other transport code
   needed to change at all.

`examples/wasm-subscriber/` demonstrates the browser half end to end:
`main.go` (build-tag `js && wasm`) calls `rtps.New` with `WithWSPeers`
directly — a genuine RTPS participant, not `bridge/ws`'s separate JSON
gateway client (`js/dds-client`, see that package's README for the explicit
scope boundary) — reads its target/domain/topic from the page's query
string, and renders received samples into the DOM; `server/` is an
ordinary native go-DDS participant with a WS listener bound, publishing
samples for the demo; `build.sh` builds `main.wasm` and copies the matching
`wasm_exec.js` glue out of the local Go toolchain (never vendored, since
its content is tied to the exact Go version). TinyGo is documented as an
alternative build path (`-target wasm`) per the ROADMAP bullet, honestly
flagged as unverified by this repo's own CI, which only exercises the
standard `go build` path (see the new `wasm-build` CI job).

For the "cloud function" half, `docs/WASM_DEPLOYMENT.md` is the Fastly/
Cloudflare Workers deployment guide. Its central, load-bearing finding: a
real `rtps.New` participant using `WithWSPeers` was proven, locally during
this sub-phase's development, to start up correctly under
[Wasmtime](https://wasmtime.dev) with `wasi:sockets` enabled — which
surfaced and fixed a genuine pre-existing bug (`REQ-TRANS-020`):
`newMulticastReceiveSocket`/`newMulticastReceiveSocketV6` treated a failure
to even *enumerate* a network interface (`net.Interfaces()` returning
nothing, exactly what a WASI runtime does) as fatal, rather than falling
back to a plain unicast bind the way they already did when enumeration
succeeded but the multicast join itself failed — despite their own doc
comments already promising that fallback unconditionally. Without that fix,
`rtps.New` could never succeed under a WASI sandbox at all, regardless of
transport. The guide is deliberately honest about what this sub-phase does
and does not independently verify beyond that: which specific WASI/
networking integration a given edge platform's *current* SDK version
exposes to a Wasm module (a plain local Wasmtime CLI invocation did not
bridge the guest's sockets through to the host's real network in this
repo's own testing, despite flags suggesting it should) is platform- and
version-specific integration work this repo cannot pre-solve generically —
the same honesty this milestone's own QUIC Transport sub-phase already
applied to an evolving external spec (see its FastDDS interoperability
scoping note above).

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

## Architecture Initiative — Multi-Module Repository Split (Tracked: #71)

Unlike Milestones 1–20, this is not tied to a single feature release: it is a
cross-cutting restructuring of the repo itself, expected to land across
several `v0.x` releases ahead of the `v1.0` stable-core cut (Milestone 14).
It has no dedicated row in the Release Plan table above — treat it as running
alongside whichever milestone is current when each phase lands.

### Why

go-DDS is one Go module containing 24 top-level packages and ~40k LOC. Every
tag versions the whole module, so a change to a peripheral, high-churn
package forces a release of the stable core — and vice versa: the core can't
cut a meaningful `v1.0` while it drags 24 packages' worth of API surface
along with it. Concrete evidence gathered directly from the current tree
(`go-DDS@2160603`, non-test Go LOC, `find <pkg> -name '*.go' -not -name
'*_test.go' | xargs wc -l`):

| Package | Prod LOC | Test LOC | Proposed group |
|---|---|---|---|
| `rtps` | 4,106 | 6,670 | core |
| root (`dds`, `adapt`) | 1,034 | 1,408 | core |
| `mock` | 654 | 2,760 | core |
| `shmem` | 776 | 1,514 | core |
| `security` | 639 | 1,312 | core |
| `pool` | 139 | 246 | core |
| `auto` | 129 | 212 | core |
| `bridge/grpc`+`rest`+`wan` | 1,235 | 3,274 | bridges |
| `idl` | 1,382 | 1,910 | tools |
| `cdr` | 348 | 502 | tools |
| `xtypes` | 460 | 729 | tools |
| `cmd/*` (4 binaries) | 1,462 | 558 | split — see below |
| `monitor` | 504 | 940 | observability |
| `admin` | 197 | 510 | observability |
| `otel` | 64 | 31 | observability |
| `services` | 231 | 544 | observability |
| `record` | 395 | 982 | observability |
| `safety` | 658 | 1,570 | safety |
| `tsn` | 824 | 865 | safety |
| `cert/` | 0 (docs only) | — | safety |

The periphery (bridges + tools + observability + safety, excluding `cert/`
docs) is ~6.3k prod LOC against core's ~7.5k — i.e. the non-core surface is
already roughly as large as the core it's bundled with, and growing faster
(bridges, idl, and observability have absorbed most feature work since
v0.10). That is the versioning-coupling problem #71 describes, with numbers
attached.

**Two things #71's plan gets right that need correcting before any split:**

1. **The `bridges` group is smaller than #71 assumed.** #71 lists `mqtt`,
   `wan`, `rest`, `grpc`, `domain` under `bridges/go.mod`. `bridge/mqtt` and
   `bridge/domain` were already removed in #98 ("remove domain + mqtt
   bridges — subsumed by RELAY crossbar"), so the group is now just
   `bridge/{grpc,rest,wan}` — 3 packages, 1,235 LOC, not 5. RELAY#59's draft
   module-name registry still lists `mqttbr`/`domainbr` as proposed DDS
   bridge names for packages that no longer exist in this repo — flagging
   that back on RELAY#59 as an additional reason those names aren't ready to
   ratify as-is.
2. **`core` is not currently a leaf.** `rtps/participant.go` imports `tsn`
   in production code (`WithTSNConfig`, the `tsnConfig`/`tsnStream` fields on
   `Participant`) — not just in tests. As proposed, `core` would depend on
   `safety`, which defeats the entire point of a stable, independently-tagged
   core: core's `go.mod` would transitively pin a `safety` module version,
   and a `safety` release would force a `go.sum` bump (though not
   necessarily a semver-major bump) in every core consumer. This has to be
   resolved before `go.mod` boundaries are cut, not worked around after
   (see Phase 0 below).
3. **The `cmd/` tree has outgrown a single `tools` bucket.** #71 (filed
   against an earlier tree) lists only `cmd/ddstool` moving to `tools`. The
   repo now has four binaries with different dependency footprints:
   `cmd/ddstool` and `cmd/go-dds` (need `idl` + core), `cmd/latmon` (core
   only), and `cmd/monitor` (needs the `monitor` package directly, i.e.
   `observability` + core). A single `tools/go.mod` can't cleanly own all
   four without also pulling in `observability` — each binary needs to ship
   from the module that owns its heaviest dependency.

### Target Layout

```
go-DDS/
├── go.mod                  # core: dds (root), rtps, mock, shmem, auto, pool, security
├── bridges/go.mod          # bridge/grpc, bridge/rest, bridge/wan
├── tools/go.mod            # idl, cdr, xtypes, cmd/ddstool, cmd/go-dds
├── observability/go.mod    # monitor, admin, otel, services, record, cmd/monitor
├── safety/go.mod           # safety, tsn, cert/ (evidence, non-code)
└── examples/go.mod         # examples/*, cmd/latmon; depends on tagged versions of the above, never same-repo relative imports
```

Import graph as it exists today (production code only, gathered by grepping
`github.com/SoundMatt/go-DDS/...` imports across the tree), annotated with
which proposed group each edge crosses:

```
dds (core)      -> mock, rtps, cyclone                    [intra-core]
rtps (core)     -> pool, security, config                 [intra-core]
rtps (core)     -> tsn                                     [core -> safety: BLOCKER, see Phase 0]
mock (core)     -> pool                                    [intra-core]
shmem (core)    -> pool                                    [intra-core]
auto (core)     -> rtps, shmem                              [intra-core]
bridge/*        -> mock                                     [bridges -> core: OK]
idl (tools)     -> cdr, mock                                 [intra-tools + tools -> core: OK]
cmd/ddstool     -> idl, mock, rtps                            [tools -> tools + core: OK]
cmd/go-dds      -> idl, mock, rtps                            [tools -> tools + core: OK]
cmd/monitor     -> monitor, rtps                               [observability -> observability + core: OK]
cmd/latmon      -> mock                                        [examples -> core: OK]
monitor (obs.)  -> mock, safety, tsn                             [observability -> safety: extraction-order constraint]
admin (obs.)    -> mock                                          [observability -> core: OK]
otel (obs.)     -> rtps                                           [observability -> core: OK]
record (obs.)   -> mock                                           [observability -> core: OK]
services (obs.) -> mock, monitor, record                           [intra-observability + -> core: OK]
safety          -> mock                                             [safety -> core: OK]
tsn             -> (no go-DDS imports)                                [leaf]
security        -> (no go-DDS imports)                                 [leaf]
xtypes          -> (no go-DDS imports, and nothing imports it yet)      [leaf, unused]
```

Aside from the `rtps -> tsn` blocker, this is a clean DAG: bridges, tools,
and safety each depend only on core; observability depends on core plus
(via `monitor`) on safety. No cycles.

### What Breaks

- **Import paths change for every non-core package.** Anything currently
  importing `github.com/SoundMatt/go-DDS/bridge/grpc`,
  `.../idl`, `.../monitor`, `.../safety`, `.../tsn`, etc. keeps the same
  path (Go submodules don't change the import path, only the release
  cadence and `go.mod` that governs it) — so this is **not** a Go import
  rewrite. What changes is: (a) `go get github.com/SoundMatt/go-DDS` alone
  no longer pulls in bridges/tools/observability/safety — consumers must add
  a second `go get` per submodule they use, and (b) each submodule gets its
  own tag sequence (`bridges/v0.1.0`, `safety/v0.1.0`, ...), so a consumer
  pinning `go-DDS v0.35.0` today has no submodule-tag equivalent to pin to
  until the split lands and the first submodule tags are cut.
- **`go.sum`/`go.mod` churn for every downstream consumer** on the first
  release after the split, even if no code changes — because the module
  boundary itself is a `go.mod` structural change.
- **CI and any script assuming a single `go.mod`/`go.sum` at repo root**
  (see CI section below).
- **RELAY's own `go.mod` dependency on `github.com/SoundMatt/RELAY` is the
  other direction** (go-DDS depends on RELAY, not vice versa) so this does
  not, by itself, force a RELAY-side change — but RELAY's `relay conform`
  and `relay interop` CI gates invoke the built `cmd/go-dds` binary, which
  after the split lives in `tools/`. The RELAY-facing CI jobs
  (`relay-conform`, `test-interop`) need their `go build`/`go install`
  invocation updated to build from `tools/` instead of repo root once that
  move happens.

### Backwards Compatibility Approach

go-DDS is still pre-v1.0 (`v0.x`), which under Go's own module conventions
means breaking changes are expected between minor releases and no
compatibility promise is owed yet. Given that, and given the module
boundary is a structural (`go.mod` location) change rather than an import-path
rename, **a clean cut is preferred over a deprecation shim**:

- No compatibility forwarding packages, no re-exported symbols at the old
  location — the packages don't move on disk in a way that changes their
  import path, so there is nothing to forward.
- The compatibility cost is entirely in `go.mod`/`go.sum`/tagging, and is
  paid once, at the release that introduces `bridges/go.mod` (etc.) for the
  first time. Document it prominently in that release's CHANGELOG entry
  (see #71's separate `CHANGELOG.md` proposal) so downstream consumers know
  to add the second `go get`.
- Because this is exactly the kind of core-API-surface decision v1.0 is
  meant to freeze (Milestone 14), doing the split **before** the v1.0 tag —
  not after — is the point: v1.0 should be the tag where "core" first means
  a genuinely small, independently stable surface (`dds`, `rtps`, `mock`,
  `shmem`, `auto`, `pool`, `security` — ~7.5k LOC), not the 24-package
  module as it exists today. If the split lands after v1.0 instead, core's
  v1.0 API-stability promise would have been made against a surface 3x
  larger than intended.

### Phased Plan

Order is derived from the import graph above, not from #71's listed group
order — you cannot cut a module boundary through a live production import,
and `core -> tsn` and `observability -> safety` are both real edges today.

- ✅ **Phase 0 — Decouple `rtps` from `tsn` (prerequisite, no module split yet).**
  Replaced `rtps.Participant`'s direct `*tsn.StreamConfig`/`*tsn.Stream`
  fields with `rtps.TSNStreamConfig`, a small interface defined in `rtps`
  that `tsn` now adapts to via `tsn.WithStreamConfig` — `rtps` no longer
  imports `tsn` at compile time. Added `archtest/coreleaf_test.go`, a
  permanent CI-enforced import-graph guardrail (runs as part of
  `go test ./...`) that fails if any core package imports a periphery
  package. Shipped in #108 (v0.52.2).
- ✅ **Phase A — Extract `safety/go.mod`** (`safety` only; `tsn`/`cert/`
  deferred — see note). Smallest group, and after Phase 0 it depends on
  nothing but core — fully self-contained. This also matches RELAY's
  roadmap cross-language priority tier for DDS parity work (RELAY
  ROADMAP.md, "Planned — DDS cross-language architecture alignment", Tier
  2 — `safety`/`security` right after the Tier 1 `rtps` port), so cutting
  it first gives cpp-DDS/rust-DDS a stable `safety` module shape to target
  early rather than last. Shipped in #109 (root v0.53.0, `safety/v0.1.0`);
  a Docker-build regression the `replace` directive caused was caught and
  fixed same-session in #110. **Scope note:** `tsn` and `cert/` were NOT
  folded into `safety/go.mod` in this pass — `tsn` has zero go-DDS imports
  today so doing so is a straightforward follow-up, not a blocker, but it
  wasn't done here; treat this bullet as partially complete.
- ✅ **Phase B — Extract `bridges/go.mod`** (`bridge/grpc`, `bridge/rest`,
  `bridge/wan`). Only depends on core (`mock`); no dependency on `safety`,
  so it doesn't need to wait on Phase A except for repo-hygiene reasons
  (doing them in sequence rather than in parallel keeps each PR reviewable).
  Shipped in #112 as `bridge/go.mod` (module
  `github.com/SoundMatt/go-DDS/bridge`, first tag `bridge/v0.1.0`) — named
  after the existing `bridge/` directory
  rather than the roadmap's provisional "bridges" group name, so
  `bridge/grpc`/`bridge/rest`/`bridge/wan` import paths stay unchanged per
  the "Module Naming Caveat" below. Root `go.mod` needed no `require`/
  `replace` change (unlike Phase A's `safety`): nothing in the root module
  imports `bridge/...` in production or test code. Added a `go.work` at
  repo root (`. bridge safety`) for local multi-module dev, per
  "CI/Testing Implications" — not added in Phase A, added here instead.
- ✅ **Phase C — Extract `tools/go.mod`** (`idl`, `cdr`, `xtypes`,
  `cmd/ddstool`, `cmd/go-dds`). Unlike `safety` and `bridge` (Phases A/B),
  these five packages did not already share a common existing directory —
  `idl`, `cdr`, and `xtypes` were separate top-level directories and
  `cmd/ddstool`/`cmd/go-dds` were two of four siblings under `cmd/` (the
  other two, `latmon` and `monitor`, stay in later phases' modules) — so a
  single Go module covering exactly this group could not be cut in place the
  way `bridge/go.mod` was. This phase physically moved them under a new
  `tools/` directory (`tools/idl`, `tools/cdr`, `tools/xtypes`,
  `tools/cmd/ddstool`, `tools/cmd/go-dds`), which **does** change `idl`'s,
  `cdr`'s, and `xtypes`'s Go import paths (e.g. `.../idl` -> `.../tools/idl`)
  — a deliberate, documented deviation from the "no Go import rewrite"
  default described above in "What Breaks", made because there was no
  path-preserving alternative that still produced one `tools/go.mod`;
  `cmd/ddstool`/`cmd/go-dds`'s install paths moved too, but as `package main`
  binaries they have no library import-path consumers to break. Per
  ROADMAP.md's own prediction, `xtypes` had zero fan-in so its move was
  risk-free; `idl`'s only two importers (`cmd/ddstool`, `cmd/go-dds`) moved
  in the same PR, so nothing outside `tools/` referenced the old paths.
  Shipped in #113 as `tools/go.mod` (module `github.com/SoundMatt/go-DDS/tools`,
  first tag `tools/v0.1.0`). Root `go.mod` needed no `require`/`replace`
  change (like Phase B, unlike Phase A): nothing in the root module imports
  `idl`/`cdr`/`xtypes` in production or test code, so root keeps its
  `v0.53.0` tag unchanged. `go.work` updated to add `./tools`.
  `.github/workflows/ci.yml`: added a `test-tools` matrix leg mirroring
  `test-safety`/`test-bridge`; `relay-conform`'s `Build go-dds CLI` step and
  `fuzz-short`'s `FuzzIDLParser`/`FuzzIDLGenerate` steps now run with
  `working-directory: tools`, per "CI/Testing Implications".
- ✅ **Phase D — Extract `observability/go.mod`** (`monitor`, `admin`,
  `otel`, `services`, `record`, `cmd/monitor`). Deliberately last among the
  four peripheral groups because `monitor` imports `safety` and `tsn` in
  production code — this group can only cleanly depend on a *released,
  tagged* `safety` module, which requires Phase A to have shipped first.
  Doing D before A would mean either a temporary same-repo relative
  `replace` directive (workable but adds noise) or `observability` briefly
  importing `safety` via relative path while both live pre-split, which
  Phase A's earlier landing avoids entirely. Like `tools/` (Phase C, unlike
  `bridge`/`safety`), these five packages did not already share one
  directory, so this phase physically moved them under a new
  `observability/` directory (`observability/monitor`, `observability/admin`,
  `observability/otel`, `observability/services`, `observability/record`,
  `observability/cmd/monitor`), changing their Go import paths — the same
  deliberate deviation Phase C called out, made for the same reason (no
  path-preserving alternative could produce one `observability/go.mod`);
  `cmd/monitor`'s install path moves too, but as a `package main` binary it
  has no library import-path consumers to break. Took the `monitor`->`safety`
  dependency from the tagged `safety/v0.1.0` module (a `go.mod` `require`,
  not a relative-path `replace`), per this phase's whole reason for coming
  after Phase A. Unlike Phases B/C, root `go.mod` **did** need a change:
  `examples/otel-tracing` (still root-tree pending Phase E) imports the
  `otel` package, so root now takes `github.com/SoundMatt/go-DDS/observability
  v0.1.0` with an in-repo `replace ... => ./observability`, mirroring how
  Phase A added root's now-removed `safety` require/replace for the same
  reason (`monitor`, the only root-tree importer of `safety`, moved out in
  this phase, so that require/replace was dropped from root `go.mod`).
  Shipped in #114 as `observability/go.mod` (module
  `github.com/SoundMatt/go-DDS/observability`, first tag
  `observability/v0.1.0`; root bumped to v0.54.0 for the require/replace
  change and the package-surface reduction). Proactively fixed
  `docker/Dockerfile`'s builder stage per the Phase A COPY-order lesson
  (#110): `observability/go.mod`/`go.sum` are now `COPY`'d and
  `go mod download`'d before the full build context, and the `monitor`
  binary is now built with `cd observability && go build ./cmd/monitor`
  since it lives in a different module. `go.work` updated to add
  `./observability`. `.github/workflows/ci.yml`: added a `test-observability`
  matrix leg mirroring `test-safety`/`test-bridge`/`test-tools`.
- ✅ **Phase E — `examples/go.mod`** (`examples/*`, `cmd/latmon`), depending
  only on tagged versions of core + whichever peripheral modules each
  example demonstrates. Last, since examples are meant to exercise the
  *released* module boundaries as an implicit integration test of the split.
  Unlike `tools`/`observability` (Phases C/D), `examples/*` already lived
  under one directory, so most packages needed no move — only `cmd/latmon`
  did not already live under `examples/`, so it moved to
  `examples/cmd/latmon` (its install path changes; as a `package main`
  binary it has no library import-path consumers to break). Confirmed by
  grepping production imports that no example needs `safety`, `bridge`, or
  `tools`: only `examples/otel-tracing` imports a peripheral module
  (`observability/otel`), so `examples/go.mod` takes exactly one peripheral
  `require` — `github.com/SoundMatt/go-DDS/observability v0.1.0`, tagged,
  never a relative-path `replace`, per this phase's whole point (examples
  exercise the *released* module boundaries). Only `github.com/SoundMatt/
  go-DDS` (core) gets the in-repo `replace ... => ../` used throughout
  Phases A-D. Shipped as `examples/go.mod` (module
  `github.com/SoundMatt/go-DDS/examples`, first tag `examples/v0.1.0`).
  Root `go.mod` **did** change: `examples/otel-tracing` was the only
  root-tree importer of `observability`, so root's Phase D `require`/
  `replace github.com/SoundMatt/go-DDS/observability` and its now-unused
  transitive `go.opentelemetry.io/otel*` deps all dropped out, shrinking
  root back down to just `RELAY` + `protobuf` + `x/sys` (root bumped to
  v0.55.0 for the package-surface reduction). Proactively fixed
  `docker/Dockerfile`'s builder stage per the Phase A COPY-order lesson
  (#110): `examples/go.mod`/`go.sum` are now `COPY`'d and `go mod
  download`'d before the full build context (alongside the existing
  `observability/` treatment from Phase D), and the `pub`/`sub` quickstart
  binaries are now built with `cd examples && go build ./quickstart/{pub,sub}`
  since they live in a different module now. `go.work` updated to add
  `./examples`. `.github/workflows/ci.yml`: added a `test-examples` matrix
  leg mirroring `test-safety`/`test-bridge`/`test-tools`/`test-observability`.
- ✅ **Phase F — Root cleanup**: moved `HARA.md`, `SEOOC.md`,
  `STANDARDS_GAP.md`, `CODING_STANDARD.md`, and `GC_LATENCY.md` into
  `docs/`, and added `docs/MATURITY.md` (per-package maturity matrix),
  `CHANGELOG.md`, and `SUPPORT.md` at the root, per #71's secondary asks.
  Pure docs/repo-hygiene reorganization — no `go.mod`/import-path changes,
  independent of the module-split mechanics proper. **Deliberately did
  NOT move** `SAFETY_PLAN.md`, `SVP.md`, `SCMP.md`, `SQAP.md`, `sas.md`, or
  the `safety-case.*` files, despite `SAFETY_PLAN.md` being one of the
  roadmap's own named examples above: go-FuSa v0.30.0 (external tool,
  per "Other Repos Policy" not something this repo can patch) checks each
  of these at a hardcoded root-relative path with no configurable
  fallback (confirmed by reading the tool's source — `iso26262.go`,
  `iec61508.go`, `do178.go`, `sci.go`, and `sas.go` all `os.Stat` bare
  filenames like `"SAFETY_PLAN.md"` joined to `projectRoot`, unlike
  `iec62443.go`'s `SECURITY.md`/`INCIDENT-RESPONSE.md` checks, which do
  support a `docs/` fallback), and `sas.md`/`safety-case.*` are also
  auto-regenerated to root on every release tag by
  `.github/workflows/release.yml`. Moving any of these would have silently
  corrupted the ISO 26262 / IEC 61508 / DO-178C gap-report evidence those
  checks produce. `SECURITY.md` and `INCIDENT-RESPONSE.md` also stayed at
  root, as GitHub community-health-file convention locations rather than
  "architecture wall" docs — see CHANGELOG.md for the full rationale.
  Updated cross-references in `README.md`, `SAFETY_PLAN.md`, `SQAP.md`,
  and `safety/gc_latency_test.go` (which now writes its generated report to
  `docs/GC_LATENCY.md`, hence a `safety` submodule patch bump alongside
  root). Shipped in #116 (root v0.55.1, `safety/v0.1.1`).

### CI/Testing Implications

The current `.github/workflows/ci.yml` (`test-mock`, `test-rtps`,
`test-cyclone`, `test-interop`, `relay-conform`, `benchmark-smoke`,
`fuzz-short`, `lint`, `static-analysis`, `gofusa`, `generate` — 11 jobs) runs
`go build ./...` / `go vet ./...` / `go test ./...` once, at repo root,
because there is exactly one `go.mod` today. A multi-module repo needs:

- Each job that runs `go build|vet|test ./...` becomes a matrix over
  modules (`.`, `bridges/`, `tools/`, `observability/`, `safety/`,
  `examples/`), or an explicit per-module step list — `./...` from repo
  root silently stops covering submodules once they have their own
  `go.mod` (Go treats a directory with its own `go.mod` as a module
  boundary the parent's `./...` does not cross).
- `relay-conform` and `test-interop` (which build/run `cmd/go-dds`) need
  their working directory changed to `tools/` once Phase C lands.
- A `go.work` file at repo root for local multi-module development (`go
  work use . bridges tools observability safety examples`) so `go build
  ./...` and IDE tooling keep working across module boundaries during
  day-to-day development, without that file affecting the published
  module graph (CI should not rely on `go.work` — it should build each
  module independently the way an external consumer would).
- The Phase 0 import-graph lint (core must stay a leaf) becomes a permanent
  CI job, not a one-time check.
- Release tagging changes from one `vX.Y.Z` tag per release to one tag per
  module per release that touches it (`vX.Y.Z` for core,
  `safety/vX.Y.Z` for the safety module, etc., per Go's multi-module
  tagging convention) — `.github/workflows/release.yml` needs to detect
  which module(s) changed and tag only those.

### Module Naming Caveat

RELAY spec §13.7.2's module-name registry does not yet have a DDS entry —
it's proposed but not ratified at RELAY#59. The group/package names used
above (`core`, `bridges`, `tools`, `observability`, `safety`, and the
individual package names `rtps`/`xtypes`/`tsn`/`idl`/`cdr`/`shmem`/bridge
names) match #71's and RELAY#59's current drafts, but RELAY#59 is itself
already stale in one respect (it lists `mqttbr`/`domainbr` for bridges this
repo removed in #98 — see "Why" above). Treat every name in this section as
provisional: once RELAY#59 lands as a ratified §13.7.2 entry, this repo's
actual `go.mod` paths/directory names may need to change to match the
spec-blessed registry rather than the other way around. Don't start Phase A
by hard-coding these names into tooling, docs, or external announcements as
final.

Success Criteria:
`core` ships as an independently-tagged, dependency-leaf Go module of ~7
packages; `bridges`, `tools`, `observability`, and `safety` each version
independently; CI covers all six modules; and the resulting layout is the
one cpp-DDS and rust-DDS build out to match (RELAY ROADMAP.md, "Planned —
DDS cross-language architecture alignment").

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
