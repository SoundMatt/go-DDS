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
| **main** | — | **go-FuSa v0.25.1; OpenTelemetry adapter (`otel/`); roadmap ✅ audit** |

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
- **main (unreleased)** — go-FuSa v0.25.1; `otel/` OpenTelemetry adapter (`NewTracer(tp)` bridges `dds.Tracer` to any OTel provider); roadmap ✅ audit (all implemented items marked)

---

## Current State

### Core DDS

- Participant API
- Publisher API
- Subscriber API
- Topic abstraction
- WaitSets
- QoS foundations

### RTPS

- Native RTPS implementation
- Discovery
- Reliability
- UDP transport

### Backends

- Native RTPS
- CycloneDDS
- Mock runtime
- Shared Memory

### Integration

- MQTT bridge
- TSN stream model

### Quality

- Unit testing
- Interoperability testing
- Fuzzing
- CI/CD

---

## Milestone 1 — Production Runtime `v0.6`

Goal:
Deliver a production-grade DDS runtime.

### Discovery

- Robust SPDP
- Robust SEDP
- Discovery diagnostics
- Discovery metrics
- Discovery health reporting

### Reliability

- Complete RTPS reliability
- DATA_FRAG
- Fragmentation
- Reassembly
- Recovery tuning

### Configuration

- YAML configuration
- JSON configuration
- Configuration validation

### Operations

- Runtime diagnostics
- Runtime inspection
- Runtime health

Success Criteria:
A deployment can be operated and debugged without custom tooling.

---

## Milestone 2 — Developer Experience `v0.7`

Goal:
Make go-DDS the easiest DDS implementation to develop against.

### Code Generation

- IDL parser
- Go code generation
- Typed publishers
- Typed subscribers

### Serialization

- CDR
- XCDR

### Local Development

- Virtual DDS runtime
- In-memory transport
- Single-process simulation

### Testing

- Mock participants
- Topic simulators
- Test harnesses

### CLI

- Topic inspection
- Publish tool
- Subscribe tool
- Discovery inspection

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

- Scenario runner _(deferred — `testutil/` helpers cover current needs; full framework planned for v0.16)_
- Automated test scripts _(deferred)_
- Expected behavior validation _(deferred)_

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
- Automatic transport selection _(deferred — planned for v0.16; requires unified participant wrapping shmem+UDP)_

### Zero Copy

- Loaned samples _(deferred — allocation-free path exists via `pool.BytePool`; zero-copy loan API planned for v0.16)_
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

1. Pure Go implementation
2. Clean developer experience
3. Built-in testing and validation
4. Strong observability
5. Safety-oriented E2E support
6. TSN-native design
7. Embedded-to-cloud deployment model
8. Modern operational tooling

The goal is not to become another DDS implementation.

The goal is to become the easiest data distribution platform to develop, test, validate, operate, and deploy.
