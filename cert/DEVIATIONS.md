# Deviations Register
## go-DDS

**Document ID:** DEV-001  
**Version:** 1.0  
**Date:** 2026-06-09  
**Standard:** CODING_STANDARD.md §8, DO-178C §11  

---

## Active deviations

### SCR-WAI-001 — `rtps` coverage below 90% threshold

| Field | Value |
|---|---|
| ID | SCR-WAI-001 |
| Type | Coverage waiver |
| Standard reference | `cert/SCR.md §3.1`, `cert/SVP.md §3.5` |
| Date raised | 2026-06-09 |
| Package | `rtps` |
| Measured coverage | 80.5% (threshold 90%) |
| Gap | 9.5% |
| Reason | Uncovered paths require kernel fault injection (IPv6 socket errors, SEDP lease timing) not achievable in portable CI environment |
| Mitigations | Code review of uncovered paths; `go vet` confirms no unreachable code; fuzz corpus covers packet parser |
| Risk | Low — uncovered paths are error handlers, not primary data paths |
| Verifier acceptance | Pending first independent review |
| Resolution plan | Fault injection tests using `net.Conn` mock planned for v0.15 |

---

### CS-DEV-001 — `recover()` in `safety/queue.go`

| Field | Value |
|---|---|
| ID | CS-DEV-001 |
| Type | Coding standard deviation |
| Standard reference | `CODING_STANDARD.md §2.2` |
| Date raised | 2026-06-09 |
| Location | `safety/queue.go:111` |
| Rule deviated | "recover() without documented justification prohibited" |
| Justification | The `recover()` is inside a deferred function within `Enqueue()` to catch panics from the user-provided callback. This is necessary to prevent a panicking callback from crashing the entire participant. The recovered panic is re-emitted as an error return. |
| gofusa annotation | `//fusa:req REQ-SAFETY-004` (defensive programming) |
| Risk | Negligible — the recover boundary is narrow and documented |
| Verifier acceptance | Pending |

---

### ANA001-WAI-001 — Latency collector goroutine in `safety/gc_latency_test.go`

| Field | Value |
|---|---|
| ID | ANA001-WAI-001 |
| Type | gofusa warning suppression |
| Standard reference | gofusa ANA001 |
| Date raised | 2026-06-09 |
| Location | `safety/gc_latency_test.go:178` |
| Warning | `goroutine launched without visible termination signal` |
| Justification | The goroutine terminates when `latCh` is closed (range-over-channel). gofusa cannot statically detect channel-close termination. This is a test-only goroutine with no safety-critical behaviour. |
| Risk | None — test code only |

---

## Closed deviations

_(none at v1.0)_

---

## Revision history

| Version | Date | Author | Changes |
|---|---|---|---|
| 1.0 | 2026-06-09 | Matt Jones | Initial register |
