# Hazard Analysis and Risk Assessment (HARA)
## go-DDS — ASIL-B SEOOC

**Document type:** Tabletop HARA (ISO 26262 Part 3, Clause 6)  
**Version:** 0.1  
**Date:** 2026-06-09  
**Status:** Draft — review before production use  
**Scope:** go-DDS used as a DDS middleware component in an ADAS sensor-fusion node  
**Standard reference:** ISO 26262:2018, Part 3 (Concept phase), Clause 6

---

## 1. Item definition

**Item:** go-DDS middleware library (`github.com/SoundMatt/go-DDS`)

**Use case (Intended Function):** Real-time publish/subscribe data distribution
between software components in an ADAS sensor-fusion ECU. The library routes
timestamped sensor payloads (camera, LiDAR, RADAR, IMU) from publisher goroutines
to subscriber goroutines with configurable QoS (BestEffort or Reliable).

**Boundary:**
- **In scope:** go-DDS mock and RTPS backends, QoS enforcement, security layer
- **Out of scope:** sensor drivers, actuator control, OS scheduling, network stack
- **Deployment assumption:** Linux-based automotive ECU (e.g. Nvidia Orin, Renesas
  R-Car H3), >= 4 CPUs, GOMEMLIMIT set to 70% of available RAM

**Operational modes:**
| Mode | Description |
|---|---|
| Normal | Continuous sensor data flow at rated Hz |
| Degraded | One sensor stream stopped; others continue |
| Shutdown | Participant/participant close sequence |

---

## 2. Hazard identification

Hazards are identified for the sensor-fusion ADAS use case. The item boundary
means go-DDS is a **component** — only hazards that can be **caused or exacerbated**
by go-DDS are in scope.

| ID | Hazard description | Failure mode in go-DDS |
|---|---|---|
| H-01 | Late delivery of sensor data causes fusion algorithm to use stale values | Message not delivered within deadline (GC pause, channel full, topic mismatch) |
| H-02 | Corrupt sensor payload used in fusion without detection | Silent bit corruption during in-process copy; missing CRC/HMAC in security layer |
| H-03 | Loss of a complete sensor stream without notification | Publisher closed; subscriber channel drained; no liveliness event issued |
| H-04 | Wrong topic routing (data from sensor A delivered to handler for sensor B) | Topic wildcard match error; SEDP routing table corruption |
| H-05 | Overflow of subscriber channel causes head-of-line blocking | Slow consumer; back-pressure policy drops or blocks publisher goroutine |
| H-06 | Memory exhaustion destabilises the ECU | Unbounded history or sequence backlog under Reliable QoS |

---

## 3. Risk assessment

For each hazard, severity (S), exposure (E), and controllability (C) are rated
per ISO 26262-3:2018 Table 4 / Table 5 / Table 6 to derive ASIL.

### H-01 — Late delivery (timing violation)

**Scenario:** go-DDS fails to deliver a camera frame within 16.7 ms (half-period
of 30 Hz). The fusion algorithm runs with stale data. At highway speed (120 km/h),
33 ms of stale perception = ~1.1 m of unobserved travel.

| Parameter | Rating | Justification |
|---|---|---|
| Severity (S) | S2 | Severe injuries possible if stale perception causes a missed obstacle; S3 requires life-threatening — ADAS with redundant path likely mitigates to S2 |
| Exposure (E) | E4 | High exposure: sensor fusion runs continuously at highway speed |
| Controllability (C) | C2 | Controllable: driver can perceive and react if ADAS gives an audible/visual warning; C3 if no fallback |

**ASIL derivation:** S2 + E4 + C2 → **ASIL B**

**Evidence from measurement (TestGCLatencyProfile, 2026-06-09):**
- GC pause MAX: 145.625 µs (< 16.7 ms budget by 114×)
- E2E latency MAX: 305 µs (< 16.7 ms budget by 54×)
- Message delivery: 1497/1500 (99.8%)

**Conclusion:** go-DDS meets the H-01 timing budget with substantial margin.

### H-02 — Corrupt payload delivered undetected

**Scenario:** A DDS sample arrives at the subscriber with corrupted payload.
go-DDS has no in-process memory protection; the OS provides process isolation.

| Parameter | Rating | Justification |
|---|---|---|
| Severity (S) | S2 | Corrupt sensor value could cause incorrect actuator command |
| Exposure (E) | E3 | Occurs only under hardware fault (RAM error, speculative execution bug) |
| Controllability (C) | C2 | Sensor plausibility checks in fusion algorithm provide a detection layer |

**ASIL derivation:** S2 + E3 + C2 → **ASIL A**

**Mitigation:** `security.HMAC256Plugin` and `security.AESGCMPlugin` provide
cryptographic integrity verification at the application layer (REQ-SECURITY-001
through REQ-SECURITY-010). For ECU-internal IPC, the OS memory model is the
primary barrier; HMAC is not required but is available.

### H-03 — Undetected sensor stream loss

**Scenario:** A publisher crashes or closes without the subscriber being notified.
The subscriber continues reading the last cached sample indefinitely.

| Parameter | Rating | Justification |
|---|---|---|
| Severity (S) | S2 | As H-01 — stale perception hazard |
| Exposure (E) | E2 | Software crashes are infrequent but possible under workload |
| Controllability (C) | C2 | Fusion algorithm can detect frozen timestamps |

**ASIL derivation:** S2 + E2 + C2 → **QM** (ISO 26262-3 Table 4)

**Mitigation:** `dds.DeadlineQoS` triggers a missed-deadline callback when a
publisher stops writing within the configured period. The fusion application
shall register a deadline callback and assert a safe-state flag when triggered
(integrator responsibility per AoU-03 below).

### H-04 — Wrong topic routing

**Scenario:** Camera data delivered to the LiDAR handler due to topic match error.

| Parameter | Rating | Justification |
|---|---|---|
| Severity (S) | S1 | Sensor type mismatch likely detected by type/length checks in consumer |
| Exposure (E) | E4 | High — persistent misconfiguration |
| Controllability (C) | C1 | Easily detected: wrong payload size/format causes immediate parse error |

**ASIL derivation:** S1 + E4 + C1 → **QM**

**Mitigation:** Topic names are application-controlled strings. Integration tests
shall verify each publisher/subscriber pair (CONTRIBUTING.md §Test modes).

### H-05 — Channel overflow / back-pressure

**Scenario:** Slow subscriber causes publisher goroutine to block or drop samples
under the DropNewest back-pressure policy.

| Parameter | Rating | Justification |
|---|---|---|
| Severity (S) | S2 | Same as H-01 — delayed delivery |
| Exposure (E) | E3 | Occurs when consumer is temporarily preempted by a higher-priority task |
| Controllability (C) | C2 | OS scheduling typically resolves transient preemption within one GC cycle |

**ASIL derivation:** S2 + E3 + C2 → **ASIL A**

**Mitigation:** `safety.DeterministicQueue` (REQ-SAFETY-002) provides an
allocation-free bounded-latency ring buffer for the final delivery hop,
decoupling publisher from subscriber without goroutine blocking.

### H-06 — Memory exhaustion

**Scenario:** Reliable QoS history grows unbounded under a slow ACKNACK path.

| Parameter | Rating | Justification |
|---|---|---|
| Severity (S) | S2 | OOM kill of the fusion process causes full sensor blackout |
| Exposure (E) | E2 | Requires both high publish rate and persistent ACKNACK loss |
| Controllability (C) | C3 | OOM kills are hard to control in real time |

**ASIL derivation:** S2 + E2 + C3 → **ASIL A**

**Mitigation:** `dds.ResourceLimitsQoS.MaxSamples` caps the in-flight history
backlog per writer. Integrator shall set this value (AoU-04 below).

---

## 4. ASIL summary

| Hazard | ASIL | Component requirement |
|---|---|---|
| H-01 — Late delivery | **ASIL B** | Timing budget met (measured); GC pause < 146 µs |
| H-02 — Corrupt payload | ASIL A | HMAC/AES-GCM security plugin available |
| H-03 — Stream loss | QM | DeadlineQoS callback; integrator safe-state |
| H-04 — Wrong routing | QM | Integration test coverage |
| H-05 — Back-pressure | ASIL A | DeterministicQueue for hot path |
| H-06 — Memory exhaustion | ASIL A | MaxSamples resource limit |

**Highest ASIL: B**

go-DDS is therefore correctly characterised as **ASIL-B SEOOC** for use in
ADAS sensor-fusion applications at up to 30 Hz camera cadence on platforms
with >= 4 CPUs and GOMEMLIMIT configured.

---

## 5. Assumptions of Use (AoU)

The following AoUs must be satisfied by the integrating system. They are
referenced in the go-DDS safety manual (safety-case.md).

| ID | Assumption |
|---|---|
| AoU-01 | The deployment platform has >= 4 logical CPUs. Single-core deployments invalidate the GC latency measurements and require re-profiling with `TestGCLatencyProfile`. |
| AoU-02 | `GOMEMLIMIT` is set to <= 70% of available physical RAM. Without a limit, the GC may defer major cycles and produce larger STW pauses. |
| AoU-03 | The application registers a `DeadlineQoS` missed-deadline callback on each safety-relevant subscriber and asserts a defined safe state (e.g. ADAS inhibit) when triggered. |
| AoU-04 | `ResourceLimitsQoS.MaxSamples` is set to a finite value on all Reliable QoS publishers to bound memory growth. |
| AoU-05 | The application does not perform synchronous allocations > 1 MiB on the DDS hot path. Large single allocations can increase individual GC pause durations beyond the measured profile. |
| AoU-06 | The application is isolated in its own OS process. go-DDS assumes OS-level memory protection between participants; in-process corruption is not defended against without the security plugin. |
| AoU-07 | go-DDS is used as a **component** (SEOOC). The integrating system is responsible for HARA at the system level; this document covers only the go-DDS contribution to system risk. |

---

## 6. ASIL decomposition rationale

ASIL B arises from H-01 alone. All other hazards are ASIL A or QM. The
decomposition argument is:

- **go-DDS (this item):** ASIL-B(d) — verified by `TestGCLatencyProfile` evidence
  that timing budget is met with 50× margin.
- **Redundant perception path (integrating system):** ASIL-B(d) — a second ADAS
  function (e.g. independent IMU dead-reckoning) provides the complementary half
  of an ASIL-B decomposition per ISO 26262-9:2018 §5.

Together, the system achieves ASIL B without requiring ASIL C or D on any
single component. This is the standard decomposition pattern for camera-based
ADAS at SAE Level 2.

---

## 7. References

| Document | Relevance |
|---|---|
| `safety-case.md` | Full safety case with 221 requirements and traceability |
| `GC_LATENCY.md` | Measured GC pause and E2E latency evidence (H-01) |
| `fmea.md` | Differential failure mode and effects analysis |
| ROADMAP.md §ASIL-B | Implementation roadmap for remaining gaps |
| ISO 26262-3:2018 | Concept phase, HARA methodology |
| ISO 26262-9:2018 §5 | ASIL decomposition rules |
