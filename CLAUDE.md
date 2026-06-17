# go-DDS — Claude session guide

Repo: `github.com/SoundMatt/go-DDS` (fork owned by Matt Jones).  
Local path: `/Users/matt/Documents/Coding/SoundMatt/go-DDS`

## Project overview

A certified, multi-industry Go DDS (Data Distribution Service) pub/sub library targeting automotive, aerospace, medical, industrial, robotics, and cloud deployments.  
Pure-Go interface (`dds` package) with swappable backends and transports:

| Package         | What it is                                                    |
|---|---|
| `.`             | `dds` — interfaces, QoS, WaitSet, sentinel errors             |
| `mock/`         | In-process broker, zero deps, use for unit tests              |
| `rtps/`         | Pure-Go RTPS/UDP (no CGo)                                     |
| `cyclone/`      | CycloneDDS CGo (`-tags cyclone`)                              |
| `shmem/`        | Shared-memory transport (zero-copy, same-host)                |
| `auto/`         | Automatic transport selection (shmem → RTPS fallback)         |
| `security/`     | NullPlugin, HMAC-SHA-256, AES-256-GCM, CertPlugin             |
| `interop/`      | Wire-compat tests vs CycloneDDS (`-tags interop`)             |
| `monitor/`      | Real-time web monitor (SSE, no external deps)                 |
| `tsn/`          | TSN stream model (TAPRIO, SO_TXTIME, AFDX Virtual Links)      |
| `bridge/`       | Protocol bridges: gRPC, REST, MQTT, WAN, domain, grpc         |
| `rpc/`          | Request-reply (Requester/Replier) over DDS topics             |
| `otel/`         | OpenTelemetry tracing adapter                                 |
| `idl/`          | IDL parser + Go code generator (structs, enums, @key)         |
| `cdr/`          | CDR/XCDR1 encode/decode                                       |
| `xtypes/`       | DDS-XTypes dynamic data, TypeRegistry, compatibility check    |
| `safety/`       | E2E protection, RateMonitor, SafetyEvent, schema validation   |
| `admin/`        | HTTP admin API                                                |
| `config/`       | YAML/JSON participant configuration                           |
| `pool/`         | BytePool for zero-allocation loaned samples                   |
| `record/`       | Topic recorder and replay service                             |
| `services/`     | RecorderService, ReplayService, MonitorService                |
| `testutil/`     | Test harnesses: AssertSample, TopicRecorder, BurstPublish     |
| `testutil/scenario` | Declarative scenario test runner                          |

**Roadmap transports (planned):** TCP/TLS · DTLS · QUIC · WebSocket  
**Roadmap integrations (planned):** ROS 2 rmw · Zenoh · OPC UA · SOME/IP · Prometheus  
**Roadmap platforms (planned):** RTOS/bare-metal · WebAssembly · Android/iOS  
**Roadmap certifications (planned):** ASIL-D · DO-178C DAL-A · IEC 62304 · IEC 62443 · FACE TS

## Per-PR checklist

1. `git checkout main && git pull origin main`
2. `git checkout -b fix/<area>-<short>` or `feat/<area>-<short>`
3. Implement + tests.
4. `go build ./...`
5. `go vet ./rtps/... ./...`
6. `go test -race -count=1 -short ./rtps/... && go test -race -count=1 ./...`
7. Commit (see style below).
8. `git push origin <branch>`, open PR targeting `main`.
9. Wait for all CI checks green, then merge (squash).
10. Tag patch/minor releases after merge.

## Commit message style

```
type(scope): short summary

Body explaining *why*, not what. Reference relevant ROADMAP.md items.

Signed-off-by: Matt Jones <matt@jellybaby.com>
```

Use `git commit -F - <<'COMMIT' ... COMMIT` (heredoc) to avoid zsh
history expansion on `%`, `!`, and `(`.

## Go conventions for this repo

- All errors returned by public API must wrap a sentinel from `dds.go`
  (`dds.ErrClosed`, etc.) with `fmt.Errorf("...: %w", dds.ErrClosed)`.
- Internal `rtps` package tests live in `rtps/packet_test.go`
  (`package rtps`) for direct access to unexported types.
- Public integration tests live in `rtps/rtps_test.go` (`package rtps_test`).
- `mock` is the default test backend; use `rtps` tests only when testing
  wire-protocol behaviour.
- No `sync.Mutex` wrapping `sync.Map` — they're self-synchronising.
- `heartbeatLoop` and similar goroutines receive `done <-chan struct{}` by
  **value** at startup so `Close()` can nil the field without a race.
- `go vet` and `golangci-lint` must pass before pushing.

## Known active branches / version history

| Tag      | Highlights |
|---|---|
| v0.1–0.2 | Core interfaces, RTPS, Security, TransientLocal, IPv6 |
| v0.3–0.8 | Sentinel errors, QoS, fragmentation, wildcard, metrics, shmem, pool, E2E safety |
| v0.9–0.11 | Enterprise security, dynamic data, bridges (REST/gRPC/WAN/MQTT), Docker, GHCR |
| v0.12    | Examples, safety completeness, IDL/CDR compiler, TAPRIO, go-FuSa coverage |
| v0.13.x  | IDL factory codegen, RateMonitor, TSN health dashboard, nested CDR, enum/array IDL |
| v0.14.0  | IDL go/format, @key, typedef, roundtrip, ddstool idl, fuzz targets |
| v0.14.1  | go-FuSa v0.19.0 upgrade: LINT001/ANA007/CYBER017 fixes across idl/, ddstool |
| v0.14.2  | CI: pinned gofusa v0.19.0 gate job |
| v0.14.3  | Release workflow; gitignore check-report.json |
| v0.14.4  | Fix GC latency races; independence policy (no IV&V); Docker golang:1.22→1.25 |
| v0.14.5  | Docker Node.js 24 opt-in; go-FuSa v0.21.0 |
| v0.15.0  | go-FuSa v0.25.1; otel/ OpenTelemetry adapter; roadmap ✅ audit |
| v0.16.0  | LoaningPublisher API; testutil/scenario runner; 241 requirements |
| v0.17.0  | auto/ transport selection (shmem→RTPS fallback); 244 requirements |
| v0.18.0  | Quality polish: coverage tests, fusa:req traceability, CONTRIBUTING.md expansion, new examples |
| v0.19.0  | shmem.NewLoaningPublisher; cdr/xtypes fuzz targets; auto/ coverage 76.9% |
| v0.20.0  | shmem DiscoveryMetrics/TopicMetrics/Health parity; idl/roundtrip coverage 80% |
| v0.21.0  | cyclone optional interface parity; bridge/grpc coverage 75.6%→86.7% |
| v0.22.0  | bridge/rest coverage 78.8%→91.2%; cdr/ coverage 91.7%→94.2% |
| v0.23.0  | idl/ 87.2%→88.1%; idl/roundtrip 80%→85.3%; monitor/ 90.0%→90.5% |
| v0.24.0  | bridge/grpc 86.7%→91.1%; bridge/wan 92.8%→93.6% |
| v0.25.0  | cdr/ 94.2%→99.2%; bridge/rest 91.2%→95.6%; bridge/mqtt 97.5%→100% |
| v0.26.0  | rpc/ 86.1%→98.1%; cdr/ 99.2%→100%; bridge/rest 95.6%→98.2% |
| v0.27.0  | mock/ 96.4%→98.0%; bridge/wan 93.6%→98.4%; testutil/scenario/ 100% |
| v0.28.0  | shmem/ 93.3%→95.2%; monitor/ 90.5%→91.0% |
| v0.29.0  | go-FuSa v0.25.1→v0.30.0 upgrade |
| v0.30.0  | idl/roundtrip 85.3%→96.0%; bridge/grpc 91.1%→92.8%; Docker golang:1.26 + alpine:3.21 |
| v0.31.0  | idl/ 88.4%→92.5%: parser error paths, typedef/struct sub-module resolution, camelCase/toGoName edge cases |
| v0.32.0  | rtps/ 86.8%→87.3%: CDR/locator/submessage error paths, TryRead closed channel, sendHeartbeatLocked, waitDrain |
| v0.33.0  | tsn/ 94.4%→100%: TAPRIO Validate overflow + itoa loop; shmem/ 95.2%→96.2%: readData error paths, loop continue |
| v0.34.0  | admin/ 96.1%→100%: handlePublish write/NewPublisher error paths; record/ 98.5%→100%: emitWindow write error, playFiltered pub.Write error; services/ 98.5%→100%: RecorderService.Start cleanup loop |
| v0.35.0  | go-FuSa v0.25.1→v0.30.0 upgrade; HARA/IEC62443 gap reports; INCIDENT-RESPONSE.md; SECURITY.md; CODEOWNERS |
| v0.36.0  | auto/ 76.9%→100%: transport selector error paths; monitor/ 91.0%→96.5%: SSE context-cancel, errResponseWriter |
| v0.37.0  | rtps/ 86.8%→89.5%: SPDP/locator/heartbeat/waitDrain coverage paths |
| v0.38.0  | bridge/grpc 91.1%→94.4%; idl/ 88.4%→96.7%; bridge/wan 98.4%→99.2%: race-free test |
| v0.39.0  | bridge/grpc 94.4%→98.9%; dds/ 93.6%→98.2%; rtps/ 89.5%→92.1%; fuzz isolation fix |
| v0.40.0  | rtps/ 92.1%→92.5%: fragment path, topic-mismatch dispatch, double-write counter; fuzz isolation; lint fixes |
| **main** | **next coverage target** |

## CI matrix

| Job            | What runs |
|---|---|
| test-mock      | ubuntu/macOS/Windows × Go 1.25/1.26, race detector |
| test-rtps      | ubuntu, `-short` |
| test-cyclone   | ubuntu-22.04, probe-and-skip if apt unavailable |
| benchmark-smoke | ubuntu, 1 iter each |
| fuzz-short     | ubuntu, 10 s per target |
| lint           | golangci-lint |
| gofusa         | go-FuSa v0.30.0 safety check (0 errors required) |
| dco            | DCO sign-off check |
| test-interop   | Docker probe-and-skip |

## Important files

| File | Purpose |
|---|---|
| `dds.go` | All public interfaces and types |
| `rtps/participant.go` | RTPS participant, writers, readers, dispatch |
| `rtps/spdp.go` | SPDP discovery, lease enforcement, eviction |
| `rtps/sedp.go` | SEDP endpoint matching, remote reader/writer tracking |
| `rtps/message.go` | Submessage marshal/parse (DATA, HEARTBEAT, ACKNACK, GAP) |
| `rtps/fragment.go` | DATA_FRAG marshal/parse/reassembly |
| `rtps/wildcard.go` | MQTT-style topic pattern matching |
| `rtps/persist.go` | Disk-backed TransientLocal history |
| `rtps/reliable.go` | sendHistory, recvTracker |
| `rtps/transport.go` | UDP socket helpers (IPv4, IPv6, multicast) |
| `ROADMAP.md` | Every roadmap item with per-item implementation context |
| `CONTRIBUTING.md` | Contributor guide, test modes, package contracts |

## Autonomous operation

Matt has granted blanket permission for all Go operations in this repo:
`go build`, `go test`, `go vet`, `golangci-lint`, `git` operations, `gh`
CLI for PR management. No confirmation needed for any of these.
Do NOT ask for permission before running tests, building, or committing.

## VISS / VISSR note

VISS/VISSR domain-specific integration is **out of scope** for go-DDS.
It belongs in the VISSR package (`/Users/matt/Documents/Coding/Covesa/vissr`).
go-DDS is a general-purpose DDS library; the MQTT bridge and vehicle-domain
topic conventions live in VISSR.
