# Software Verification Plan (SVP)
## go-DDS

**Document ID:** SVP-001  
**Version:** 1.0  
**Date:** 2026-06-09  
**Standard:** DO-178C §11.3, IEC 61508-3 §7.8, ISO 26262-6 §9–10  

---

## 1. Purpose

This Software Verification Plan defines the verification activities, methods,
tools, and acceptance criteria for go-DDS. It covers unit, integration,
structural coverage, and safety analysis verification.

---

## 2. Verification objectives

| Objective | DO-178C ref | IEC 61508-3 ref | ISO 26262-6 ref |
|---|---|---|---|
| Requirements are correct and complete | §6.3 | §7.2 | §7.2 |
| Software implements requirements | §6.4 | §7.8 | §9.3 |
| No unintended functionality | §6.4.4.1 | §7.8 | §9.4 |
| Structural coverage achieved | §6.4.4.2 | B.6.2 | Table 10 |
| Coding standard complied with | §6.4.2.1 | B.3.1 | §8.4.4 |

---

## 3. Verification activities

### 3.1 Static analysis

**Objective:** Detect defects without execution.

| Activity | Tool | Acceptance criterion | Frequency |
|---|---|---|---|
| Compilation | `go build ./...` | Zero errors | Every commit |
| Vet | `go vet ./...` | Zero errors | Every commit |
| Linting | `golangci-lint run` | Zero errors | Every commit |
| Safety analysis | `gofusa check ./...` | Zero errors (warnings documented) | Every commit |
| Traceability | `gofusa trace` | Zero orphan or untested requirements | Every release |

### 3.2 Unit testing

**Objective:** Verify each software unit implements its LLRs.

| Parameter | Value |
|---|---|
| Test framework | `testing` (Go standard library) |
| Race detection | Enabled (`-race`) on all tests |
| Repeat count | `-count=1` (deterministic); `-count=3` for flakiness investigation |
| Platform matrix | Linux, macOS, Windows × Go 1.25 and 1.26 |
| Short mode | `-short` flag skips tests > 5s for CI speed |

**Acceptance criterion:** All tests PASS. Any FAIL is a release-blocking defect.

### 3.3 Integration testing

**Objective:** Verify inter-package interfaces and protocol correctness.

| Test suite | Package | Scope |
|---|---|---|
| Mock integration | `mock/` | Publisher/subscriber routing, QoS enforcement, liveliness |
| RTPS protocol | `rtps/` | SPDP, SEDP, HEARTBEAT/ACKNACK, GAP, DATA_FRAG |
| Security integration | `security/` | HMAC-SHA-256, AES-256-GCM end-to-end |
| IDL roundtrip | `idl/roundtrip/` | Parse → generate → compile → execute |
| Fuzz (parser) | `idl/` | FuzzIDLParse, FuzzIDLGenerate |
| Fuzz (codec) | `cdr/` | FuzzCDRRoundtrip |

### 3.4 GC latency verification

**Objective:** Demonstrate that GC pause times do not violate ASIL-B timing budget.

| Test | Package | Duration | Acceptance criterion |
|---|---|---|---|
| `TestGCLatencyProfile` | `safety/` | 30 s | GC pause MAX < half-period of slowest safety-relevant sensor (≤ 50 ms for 10 Hz) |

Evidence stored in `GC_LATENCY.md`. Re-run required if `safety/`, `rtps/`, or
the target hardware platform changes.

### 3.5 Structural coverage analysis

**Objective:** Demonstrate that test suite exercises the code sufficiently to
provide confidence in correctness.

| Package class | Minimum statement coverage |
|---|---|
| Safety-critical (`safety/`, `security/`, `rtps/`) | ≥ 90% |
| Safety-related (`mock/`, root dds package) | ≥ 85% |
| Non-safety | ≥ 70% (informational) |

See `cert/SCR.md` for the current measurement.

**Coverage gap closure:** Gaps below threshold shall be closed by additional
tests before release. Justified exclusions (e.g. error paths that require
hardware faults) are documented in `cert/DEVIATIONS.md`.

---

## 4. Test environment

Tests run in the CI matrix defined in `.github/workflows/ci.yml`:

```
test-mock:   ubuntu-latest, macos-latest, windows-latest × go1.25, go1.26
test-rtps:   ubuntu-latest, go1.25 (-short)
fuzz-short:  ubuntu-latest, 10s per fuzz target
lint:        ubuntu-latest (golangci-lint)
gofusa:      ubuntu-latest (gofusa v0.19.0)
```

No hardware-in-the-loop is required for SEOOC verification. The `safety/`
package tests simulate ADAS workload in-process using the mock broker.

---

## 5. Independence

| Activity | Independence level |
|---|---|
| Static analysis (automated) | Full independence (tools run autonomously) |
| Test review | Verifier reviews CI logs before signing `cert/RELEASE_LOG.md` |
| Coverage review | Verifier reviews `cert/SCR.md` |
| HARA review | Verifier reviews `HARA.md` against requirements |

The verifier must be a person different from the primary developer for the
code under review (see `SAFETY_PLAN.md §3.1`).

---

## 6. Verification results

Verification results are preserved as:

- CI run logs (GitHub Actions — permanent per-run artefacts)
- `cert/RELEASE_LOG.md` — per-release sign-off record
- `cert/SCR.md` — structural coverage report (regenerated each release)
- `gofusa trace` output — traceability completeness record

---

## 7. Regression

Any change to a safety-critical package (`safety/`, `rtps/`, `security/`) that
fixes a defect requires:

1. A regression test that reproduces the defect (fuzz corpus entry is acceptable)
2. Re-run of `TestGCLatencyProfile` if timing-relevant
3. Re-run of `gofusa trace` to confirm traceability is maintained
4. Verifier sign-off on the fix commit

---

## 8. Revision history

| Version | Date | Author | Changes |
|---|---|---|---|
| 1.0 | 2026-06-09 | Matt Jones | Initial release |
