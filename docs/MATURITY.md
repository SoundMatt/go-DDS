# Package Maturity Matrix

go-DDS is pre-`v1.0` (see ROADMAP.md, Milestone 14 for the stable-core cut)
and, since the "Architecture Initiative — Multi-Module Repository Split"
(#71, ROADMAP.md), is split across six Go modules with independent tag
sequences. This matrix records the maturity of each package so consumers can
judge how much API churn to expect, independent of which module a package
happens to live in.

## Maturity levels

| Level | Meaning |
|---|---|
| **Stable** | API is settled. Breaking changes are rare, called out prominently in CHANGELOG.md, and avoided across `v0.x` minor bumps where practical. Candidate surface for the `v1.0` cut. |
| **Beta** | Functionally complete and used in the test suite / examples, but the API may still change between minor versions as the module matures past its first `v0.1.0` tag. |
| **Experimental** | Works, but interfaces are expected to change; not yet exercising every code path in CI to the same depth as Stable/Beta packages. |
| **Reference** | Demonstration code (examples, CLI entry points). No API-compatibility promise at all — it exists to show usage, not to be imported as a library. |
| **Deprecated** | Scheduled for removal; do not start new usage. |

## Core (`go.mod`, module `github.com/SoundMatt/go-DDS`, root tags `vX.Y.Z`)

| Package | Maturity | Notes |
|---|---|---|
| `.` (root — `dds`, `adapt`) | Stable | The public `dds.*` API. Core surface targeted for the `v1.0` freeze (ROADMAP.md Milestone 14). |
| `rtps` | Stable | Pure-Go wire implementation; largest test/LOC ratio in the repo. Enforced as a dependency leaf by `archtest/coreleaf_test.go`. |
| `mock` | Stable | Default in-process backend; zero dependencies; used throughout the test suite. |
| `shmem` | Stable | Shared-memory transport, zero UDP overhead for same-host pub/sub. |
| `security` | Stable | Pluggable payload security (HMAC/AES-GCM/X.509/ACL/anti-replay). Dependency leaf. |
| `pool` | Stable | Allocation-free buffer/ring recycling; small, settled surface. |
| `auto` | Stable | Automation/transport-selection glue over `rtps` + `shmem`. |
| `config` | Stable | JSON/YAML participant configuration + validation. |
| `cyclone` | Beta | CycloneDDS interop via CGo; requires `libcyclonedds-dev` + `-tags cyclone`, so it gets materially less default CI coverage than the pure-Go packages. |
| `interop` | Beta | RTPS interoperability test harness against CycloneDDS; supporting/test-role package, not a general-purpose library surface. |
| `rpc` | Beta | Request/reply layer over pub/sub. |
| `tsn` | Beta | TSN stream model + TAPRIO scheduling. Conceptually part of the `safety` group (ROADMAP.md Target Layout) but its extraction into `safety/go.mod` was explicitly deferred in Phase A — still ships from core today. |
| `testutil` | Beta | Test-harness helpers (`NewParticipant`, `AssertSample`, `TopicRecorder`, `BurstPublish`); used by this repo's own tests, general-purpose but not test-covered as a first-class public API. |
| `archtest` | N/A (internal) | No production code — exists solely to run the `TestCoreIsDependencyLeaf` CI guardrail. Not an importable package for consumers. |

## `bridge` (module `github.com/SoundMatt/go-DDS/bridge`, tags `bridge/vX.Y.Z`, first tag `bridge/v0.1.0`)

| Package | Maturity | Notes |
|---|---|---|
| `bridge/grpc` | Beta | gRPC bridge; module still on its first `v0.1.0` tag since the Phase B extraction. |
| `bridge/rest` | Beta | REST bridge; same extraction-age caveat as `bridge/grpc`. |
| `bridge/wan` | Beta | WAN bridge (TCP, length-framed JSON, optional TLS + shared-token auth). |

## `tools` (module `github.com/SoundMatt/go-DDS/tools`, tags `tools/vX.Y.Z`, first tag `tools/v0.1.0`)

| Package | Maturity | Notes |
|---|---|---|
| `tools/idl` | Beta | `.idl` → Go struct + `Codec[T]` codegen; import path changed in Phase C (`.../idl` → `.../tools/idl`). |
| `tools/cdr` | Beta | CDR/XCDR1 encode/decode. |
| `tools/xtypes` | Experimental | Dynamic Data / XTypes support; per ROADMAP.md's own import-graph audit this package currently has zero production importers in this repo. |
| `tools/cmd/ddstool` | Reference | CLI (`pub`/`sub`/`discover`/`idl`); no library import-path consumers. |
| `tools/cmd/go-dds` | Reference | CLI used by RELAY's `relay conform` / `relay interop` CI gates as a built binary, not imported as a library. |

## `observability` (module `github.com/SoundMatt/go-DDS/observability`, tags `observability/vX.Y.Z`, first tag `observability/v0.1.0`)

| Package | Maturity | Notes |
|---|---|---|
| `observability/monitor` | Beta | Real-time dashboard (`/health`, `/api/topics`, `/api/diagnostics`, SSE); depends on the tagged `safety` module. |
| `observability/admin` | Beta | HTTP admin API, bearer-token auth. |
| `observability/services` | Beta | Managed service lifecycle (RecorderService, ReplayService, MonitorService). |
| `observability/record` | Beta | Topic recording (JSONL) + deterministic replay + fault injection. |
| `observability/otel` | Experimental | OpenTelemetry tracing integration; smallest package in this module by LOC. |
| `observability/cmd/monitor` | Reference | CLI entry point for `observability/monitor`; no library import-path consumers. |

## `safety` (module `github.com/SoundMatt/go-DDS/safety`, tags `safety/vX.Y.Z`, first tag `safety/v0.1.0`)

| Package | Maturity | Notes |
|---|---|---|
| `safety` | Beta | E2E protection header (CRC-16, sequence counter, freshness) + deterministic queue. ASIL-B / SIL 2 claims apply per `SAFETY_PLAN.md` and `SEOOC.md` (`docs/SEOOC.md`), but the module itself is young (`v0.1.0`, extracted in Phase A) — treat the *module boundary* as Beta even though the safety analysis behind it is mature. |
| `cert/` | N/A (docs only) | Certification evidence package (PSAC, SDP, SVP, SCMP, SQAP, LLR, SCR, DCA, TQPs, PRP, DEVIATIONS, RELEASE_LOG) — non-code, not part of the `safety` Go module. |

## `examples` (module `github.com/SoundMatt/go-DDS/examples`, tags `examples/vX.Y.Z`, first tag `examples/v0.1.0`)

| Package | Maturity | Notes |
|---|---|---|
| `examples/*` (quickstart, auto-transport, scenario-dsl, command-response, loaned-samples, otel-tracing, sensor-pipeline, secure-topic, taprio-stream) | Reference | Demonstration code exercising the released module boundaries; no API-compatibility promise. |
| `examples/cmd/latmon` | Reference | Continuous rolling-window latency monitor CLI. |

## How this maps to versioning

Each module tags independently (`vX.Y.Z` for core, `bridge/vX.Y.Z`, `tools/vX.Y.Z`,
`observability/vX.Y.Z`, `safety/vX.Y.Z`, `examples/vX.Y.Z`) — see ROADMAP.md,
"Architecture Initiative", "CI/Testing Implications". A package's maturity
here is independent of which module it ships from: a Beta package in a
module that has otherwise reached `v1.x` is still Beta until this table says
otherwise. Update this file whenever a package's maturity changes, and when
a new package or module is added.
