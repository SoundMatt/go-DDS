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
- RTPS recording
- Metadata recording ✅

### Replay

- Deterministic replay ✅
- Time-scaled replay ✅
- Filtered replay ✅

### Scenario Testing

- Scenario runner
- Automated test scripts
- Expected behavior validation

### Fault Injection

- Packet loss ✅
- Delay ✅
- Reordering
- Duplication ✅
- Corruption ✅

### Test Reporting

- Coverage integration
- Validation reports
- Test artifacts

Success Criteria:
DDS deployments become fully testable and reproducible.

---

## Milestone 4 — Observability `v0.6`

Goal:
Provide first-class visibility into distributed systems.

### Metrics

- Participant metrics
- Topic metrics
- Discovery metrics
- Transport metrics

### Tracing

- OpenTelemetry support
- Distributed tracing

### Monitoring

- Health monitoring
- Runtime statistics
- Topology visualization

### Dashboard

- Web monitoring
- Traffic inspection
- Performance visualization

Success Criteria:
Engineers can understand system behavior without packet captures.

---

## Milestone 5 — Edge Performance `v0.8`

Goal:
Support high-performance embedded and edge deployments.

### Shared Memory

- Cross-process transport ✅
- Automatic transport selection

### Zero Copy

- Loaned samples
- Preallocated buffers ✅

### Resource Controls

- Bounded queues ✅
- Memory limits
- Flow control

### Benchmarking

- Latency benchmarks
- Throughput benchmarks
- Scalability testing

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
- Schema validation

### Safety Runtime

- Deterministic queues ✅
- Panic containment ✅
- Runtime monitoring

### Safety Visibility

- Safety metrics
- Safety events ✅
- Safety diagnostics

### Documentation

- Safety manual
- Assumptions of use
- Integration guidance

Success Criteria:
Applications can build black-channel safety architectures using go-DDS.

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
- TAPRIO integration

### TSN Diagnostics

- Timing validation
- Stream health
- Schedule monitoring

Success Criteria:
DDS topics map directly to deterministic TSN streams.

---

## Milestone 8 — Enterprise Security `v0.9`

Goal:
Provide secure communication for enterprise and automotive deployments.

### Authentication

- Certificate identity
- Mutual authentication

### Authorization

- Topic permissions
- Access policies

### Protection

- Encryption ✅
- Signing ✅
- Replay protection

### Secure Discovery

- Discovery protection
- Identity validation

Success Criteria:
Deployments can securely operate across untrusted networks.

---

## Milestone 9 — Dynamic Data `v0.9`

Goal:
Support evolving distributed systems.

### XTypes

- TypeObject
- TypeIdentifier
- DynamicData

### Runtime Type Discovery

- Dynamic type inspection
- Compatibility validation

### Evolution

- Forward compatibility
- Backward compatibility

Success Criteria:
Systems can evolve without lock-step software updates.

---

## Milestone 10 — Enterprise Services `v0.9`

Goal:
Provide operational services around the middleware.

### Routing

- Domain bridge ✅
- WAN bridge ✅
- Protocol bridge (planned v0.10 — DDS ↔ gRPC/REST gateway)

### Administration

- Admin API
- Remote diagnostics
- Remote inspection

### Service Framework

- Recorder service
- Replay service
- Monitoring service

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
