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
| v0.10 | — | Routing, Context API, Secure Discovery |

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
