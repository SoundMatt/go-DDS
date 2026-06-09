# go-DDS Safety Element Out Of Context (SEOOC)

**Standard**: ISO 26262:2018 Part 10 — Guidelines on ISO 26262  
**Claimed ASIL**: ASIL-B (for elements annotated `level: ASIL-B` in `.fusa-reqs.json`)  
**Version**: v0.14.0 and later  
**go-FuSa**: v0.21.0 (pinned in CI)  
**Maintainer**: Matt Jones &lt;matt@jellybaby.com&gt;

---

## 1. Capability Statement

go-DDS is a general-purpose DDS publish/subscribe library for Go. It is developed as a SEOOC: its functional-safety requirements are specified and verified independently of any specific application, so that integrating systems can inherit its safety evidence without repeating the element-level verification work.

The safety evidence package for go-DDS consists of:

| Artefact | File |
|---|---|
| Requirements manifest (238 requirements) | `.fusa-reqs.json` |
| Traceability matrix | `gofusa trace` output |
| FMEA | `fmea.json` / `fmea.csv` |
| Safety case | `safety-case.json` / `safety-case.md` |
| SBOM | `sbom.json` |
| Provenance | `provenance.json` |
| **HARA** | **`HARA.md`** |
| **GC latency evidence** | **`GC_LATENCY.md`** |
| **Software Safety Plan** | **`SAFETY_PLAN.md`** |
| **Coding Standard** | **`CODING_STANDARD.md`** |
| **Cert package** (PSAC, SDP, SVP, SCMP, SQAP, LLR, SCR, DCA, TQPs) | **`cert/`** |

The following subsystems carry ASIL-B claims:

- **E2E protection** (`safety/`) — REQ-SAFETY-001 through REQ-SAFETY-019
- **Rate monitoring** (`safety/rate.go`) — REQ-SAFETY-018, REQ-SAFETY-019
- **Fragment reassembly** (`rtps/fragment.go`) — REQ-FRAG-001 through REQ-FRAG-005
- **Goroutine lifecycle** — REQ-RT-001 (all packages)
- **Subscriber overflow policies** — REQ-QOS-005 through REQ-QOS-007
- **Payload size enforcement** — REQ-PART-007
- **Transport drop-under-load** — REQ-TRANS-003
- **Persistent history bounds** — REQ-REL-011

Security subsystems carry CAL-2/CAL-3 claims under ISO 21434:

- **AES-256-GCM** — REQ-SEC-008 through REQ-SEC-010, REQ-SEC-022  
- **HMAC-SHA-256 discovery** — REQ-SEC-001, REQ-SEC-016 through REQ-SEC-018  
- **Replay protection** — REQ-SEC-003, REQ-SEC-011 through REQ-SEC-012  
- **Certificate authentication** — REQ-SEC-014, REQ-SEC-024 through REQ-SEC-025  

---

## 2. Assumptions of Use (AoU)

The following assumptions must be satisfied by the integrating item. If any AoU is violated, the ASIL-B claims of go-DDS are invalidated for that deployment.

### AoU-001 — Participant.Close before process exit (REQ-SEOOC-001)

The integrating application **shall** call `Participant.Close()` before process termination. Failure to close may leave goroutines blocked on network I/O and prevent orderly resource release.

```go
p, err := rtps.New(dds.Domain(0))
defer p.Close()  // mandatory
```

### AoU-002 — E2E protection for ASIL-B topics (REQ-SEOOC-002)

For any topic that carries ASIL-B data, the integrating application **shall** wrap the publisher and subscriber with `safety.E2EPublisher` and `safety.E2ESubscriber`. The raw DDS transport provides no end-to-end data integrity; the CRC, sequence counter, and freshness checks are only active when the E2E wrappers are used.

```go
// Correct — E2E protection enabled
pub, _ := p.NewPublisher("vehicle/speed", dds.DefaultQoS)
e2ePub := safety.NewE2EPublisher(pub, safety.E2EOptions{DataID: 0x0042, SourceID: 0x0001})

sub, _ := p.NewSubscriber("vehicle/speed", dds.DefaultQoS)
e2eSub := safety.NewE2ESubscriber(sub, safety.E2EOptions{DataID: 0x0042})
```

### AoU-003 — DiscoveryPlugin in untrusted networks (REQ-SEOOC-003)

When the network segment may contain untrusted participants, the integrating application **shall** configure an `HMACDiscoveryPlugin` with a shared secret of at least 16 bytes. Without a discovery plugin, any host on the multicast group can inject fake participant announcements.

```go
plugin, _ := security.NewHMACDiscoveryPlugin([]byte("my-32-byte-shared-secret-here!!!"))
p, err := rtps.New(dds.Domain(0), rtps.WithDiscoveryPlugin(plugin))
```

### AoU-004 — No payload mutation after Write (REQ-SEOOC-004)

The integrating application **shall not** modify the payload byte slice passed to `Publisher.Write()` or `Publisher.WriteCtx()` after the call returns. On Reliable topics, go-DDS retains the slice for potential retransmission.

### AoU-005 — Cryptographically secure entropy (REQ-SEOOC-005)

The operating platform **shall** provide a cryptographically secure random source via Go's `crypto/rand` package. go-DDS uses this for GUID prefix generation (REQ-GUID-001) and AES-GCM nonce generation (REQ-SEC-008). A weak entropy source invalidates both GUID uniqueness and AEAD security.

### AoU-006 — Context deadline discipline (REQ-SEOOC-006)

The integrating application **shall** supply `context.Context` values with appropriate deadlines to all blocking operations: `Publisher.WriteCtx()`, `WaitSet.Wait()`, and participant creation via `WithContext`. An unbounded block on a Reliable write violates real-time guarantees.

### AoU-007 — DeterministicQueue for bounded-latency paths (REQ-SEOOC-007)

For any data path with a hard latency deadline, the integrating application **shall** use `safety.DeterministicQueue` rather than an unbuffered Go channel. Standard channel sends are subject to runtime scheduler jitter; DeterministicQueue provides O(1) allocation-free enqueue/dequeue with synchronous overflow reporting.

### AoU-008 — Application-level panic handling (REQ-SEOOC-009)

The integrating application **shall** install `recover()` handlers in every goroutine it owns that calls DDS APIs. go-DDS recovers panics on its own forwarding goroutines (REQ-SAFETY-003), but panics in application goroutines calling Write or receiving from Subscriber.C() will propagate normally.

### AoU-009 — Participant re-creation on failure (REQ-SEOOC-008)

The integrating application is responsible for detecting participant failure (via the `HealthProvider` interface or by observing `ErrClosed` returns) and re-creating the participant when an unrecoverable error occurs. go-DDS does not automatically restart a closed participant.

### AoU-010 — Single participant per domain (REQ-SEOOC-010)

For simplicity and to avoid port contention, the integrating application **should** create at most one `Participant` per DDS domain per process. Multiple participants in the same domain are permitted but incur additional resource overhead.

---

## 3. Safety-relevant configuration

The table below maps each safety-relevant configuration option to the requirements it satisfies.

| Option | Package | Requirements satisfied |
|---|---|---|
| `safety.NewE2EPublisher / NewE2ESubscriber` | `safety` | REQ-SAFETY-001 through REQ-SAFETY-014 |
| `safety.NewDeterministicQueue` | `safety` | REQ-SAFETY-002, REQ-SAFETY-010 through REQ-SAFETY-014 |
| `security.NewHMACDiscoveryPlugin` | `security` | REQ-SEC-001, REQ-DISC-006, REQ-DISC-010 |
| `security.NewAESGCMPlugin` | `security` | REQ-SEC-002, REQ-SEC-008 through REQ-SEC-010 |
| `security.NewReplayGuard` | `security` | REQ-SEC-003, REQ-SEC-011 through REQ-SEC-012 |
| `rtps.WithContext(ctx)` | `rtps` | REQ-PART-004, REQ-REL-004 |
| `dds.ReliableQoS` | `dds` | REQ-REL-001, REQ-QOS-002 |
| `dds.QoS{Durability: dds.TransientLocal}` | `dds` | REQ-REL-002, REQ-SUB-005 |

---

## 4. Known limitations and deviations

1. **HARA is tabletop, not system-level**: `HARA.md` provides a formal tabletop HARA for the ADAS sensor-fusion use case (ISO 26262-3 methodology), confirming ASIL-B for the H-01 late-delivery hazard. A system-level HARA for a specific vehicle application remains the integrator's responsibility.

2. **No MISRA-Go compliance**: go-DDS uses standard Go idioms (goroutines, interfaces, generics) that are not evaluated against MISRA-Go. Integrators targeting MISRA-Go environments must perform their own conformance review.

3. **Goroutine scheduler is non-deterministic**: Go's M:N goroutine scheduler does not provide real-time scheduling guarantees. For hard real-time requirements, go-DDS should be wrapped with `DeterministicQueue` and the application should be profiled under worst-case load.

4. **No hardware security module (HSM) integration**: Cryptographic key material is held in process memory. Integrators with HSM requirements must implement a custom `Plugin` that delegates to the HSM.

5. **Transport is UDP**: The RTPS transport does not provide network-level reliability guarantees independent of DDS QoS. In environments with high packet loss rates, the Reliable QoS retransmission mechanism will cause increased latency. Configure appropriate heartbeat periods via `rtps.WithHeartbeatPeriod`.

6. **IPv6 is opportunistic**: IPv6 socket failure is silently tolerated (REQ-PART-010). Systems that require IPv6 for a specific subnet should verify socket creation succeeded by checking participant health.

---

## 5. Verification summary

All 238 requirements (228 functional + 10 SEOOC AoUs) are covered by at least one `//fusa:req` implementation annotation and one `//fusa:test` test annotation. Run:

```bash
gofusa trace          # traceability matrix — must show zero orphan/untested
gofusa check ./...    # safety analysis — must show 0 errors (v0.21.0)
gofusa safety-case    # regenerate safety-case.md / .json / .mermaid
gofusa fmea -cyber    # regenerate fmea.csv / fmea.json
gofusa release        # regenerate sbom.json, provenance.json, artifact-manifest.json
```

All commands except `release` must complete without error before any release.
See `SAFETY_PLAN.md §9` for the full release criteria checklist.
