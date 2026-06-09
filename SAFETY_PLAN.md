# Software Safety Plan
## go-DDS — ISO 26262 ASIL-B / IEC 61508 SIL 2

**Document ID:** SSP-001  
**Version:** 1.0  
**Date:** 2026-06-09  
**Status:** Released  
**Author:** Matt Jones (matt@jellybaby.com)  
**Standards:** ISO 26262:2018 Part 8 §7, IEC 61508-3:2010 §5  

---

## 1. Purpose and scope

This Software Safety Plan (SSP) defines the lifecycle, activities, methods, and
responsibilities for the development and verification of go-DDS
(`github.com/SoundMatt/go-DDS`) in accordance with:

- ISO 26262:2018 — Road vehicles — Functional Safety (Parts 3, 4, 6, 8)
- IEC 61508:2010 — Functional Safety of E/E/PE Safety-related Systems (Part 3)

go-DDS is developed as a **Safety Element Out Of Context (SEOOC)** targeting
ASIL-B (ISO 26262) / SIL 2 (IEC 61508). Refer to `HARA.md` for the hazard
analysis and risk assessment that derives these levels.

**Out of scope:** System-level HARA, hardware fault model (FMEDA), airworthiness
(DO-178C), AUTOSAR integration. These are the integrating system's responsibility.

---

## 2. Applicable documents

| ID | Document | Location |
|---|---|---|
| HARA | Hazard Analysis & Risk Assessment | `HARA.md` |
| SC | Safety Case | `safety-case.md` |
| SCR | Structural Coverage Report | `cert/SCR.md` |
| LLR | Low-Level Requirements | `cert/LLR.md` |
| SVP | Software Verification Plan | `cert/SVP.md` |
| SDP | Software Development Plan | `cert/SDP.md` |
| SCMP | Software CM Plan | `cert/SCMP.md` |
| SQAP | Software QA Plan | `cert/SQAP.md` |
| CS | Coding Standard | `CODING_STANDARD.md` |
| DCA | Diagnostic Coverage Analysis | `cert/DCA.md` |
| REQS | Requirements manifest | `.fusa-reqs.json` |
| FMEA | Differential FMEA table | `fmea.csv` |
| PROV | Build provenance | `provenance.json` |

---

## 3. Organisation and responsibilities

| Role | Responsibility | Person / entity |
|---|---|---|
| Software developer | Implements requirements; runs unit tests | Matt Jones |
| Software verifier | Reviews test results; signs off each release | Independent reviewer (see §8) |
| Safety manager | Maintains this plan; approves releases | Matt Jones |
| Configuration manager | Controls baselines, tags, provenance | Automated (GitHub Actions + `gofusa release`) |

### 3.1 Independence

IEC 61508-3 §7.8.2 recommends independence between developer and verifier at
SIL 2. ISO 26262 Part 6 Table 1 requires independence at ASIL C/D; at ASIL B
it is *recommended* (HR).

**Current state:** Single developer. A second person shall independently review
the verification results (test log, `gofusa check` output, coverage report) and
sign the release. The sign-off record shall be appended to `cert/RELEASE_LOG.md`
before each version tag.

**Minimum acceptable independence:** The reviewer need not be a full-time
safety engineer, but shall not be the same person who wrote the code under review.

---

## 4. Software lifecycle

go-DDS follows the V-model lifecycle:

```
Requirements (.fusa-reqs.json)
  ↓                           ↑
Architecture (CONTRIBUTING.md §Package contracts)
  ↓                           ↑
Design (package interfaces)  Unit tests (go test -race)
  ↓                           ↑
Implementation (*.go)    Integration tests (mock/, rtps/, interop/)
                              ↑
                         System test (CI matrix, fuzz)
```

Each level maps to ISO 26262-6:2018 work products:
- Software requirements spec → `.fusa-reqs.json` + `cert/LLR.md`
- Software architecture → `CONTRIBUTING.md` + package-level doc comments
- Software unit design → function/type signatures + `cert/LLR.md`
- Unit test spec + results → `*_test.go` + CI logs
- Integration test spec + results → `rtps/rtps_test.go`, `interop/`, CI logs
- Safety case → `safety-case.md`, `fmea.csv`

---

## 5. Development methods

### 5.1 Language and compiler

- Language: Go (version ≥ 1.25, see `.github/workflows/ci.yml`)
- Compiler: `gc` (the standard Go compiler)
- No CGo in safety-critical packages (`dds`, `mock`, `rtps`, `safety`, `security`)
- CGo is permitted only in the `cyclone/` package, which requires explicit `-tags cyclone`

### 5.2 Coding standard

See `CODING_STANDARD.md`. The standard is enforced by:
- `go vet` (built-in static analysis)
- `golangci-lint` (CI job `lint`)
- `gofusa check` (CI job `gofusa`)

### 5.3 Software complexity

Cyclomatic complexity threshold: 10 per function (enforced by gofusa COMP001).
Functions exceeding the threshold are flagged as warnings; no function in a
safety-critical package (`safety/`, `rtps/`, `security/`) shall exceed
complexity 15 without documented justification.

### 5.4 Error handling

All errors returned by public API wrap a sentinel from `dds.go`
(`dds.ErrClosed`, `dds.ErrNoData`, etc.) using `fmt.Errorf("…: %w", sentinel)`.
Internal packages never panic on invalid input from external callers.

### 5.5 Concurrency

- No `sync.Mutex` wrapping `sync.Map`
- Goroutines receive `done <-chan struct{}` by value at startup
- `sync/atomic` used for all shared counters in `safety/` package
- Race detector run on every CI push (`-race` flag)

---

## 6. Verification methods

See `cert/SVP.md` for the full verification plan. Summary:

| Method | Tool | Frequency | Coverage objective |
|---|---|---|---|
| Unit tests | `go test -race` | Every commit | Statement + decision |
| Integration tests | `go test ./rtps/...` | Every commit | RTPS protocol paths |
| Fuzz testing | `go test -fuzz` | 10s per CI run; extended locally | Parser/codec robustness |
| Static analysis | `go vet`, `golangci-lint` | Every commit | Coding standard |
| Safety analysis | `gofusa check` | Every commit | ASIL-B rules |
| Structural coverage | `go test -covermode=count` | Every release | ≥80% statement (safety pkgs ≥90%) |

---

## 7. Configuration management

See `cert/SCMP.md`. Summary:

- All source, tests, and safety artefacts are versioned in Git
- Releases are tagged `vX.Y.Z` using semantic versioning
- Each release tag triggers `gofusa release` (CI), regenerating `sbom.json`,
  `provenance.json`, and `artifact-manifest.json`
- The software baseline is identified by the git commit SHA embedded in
  `provenance.json`

---

## 8. Problem reporting and resolution

See `cert/PRP.md`. Summary:

- Defects are tracked as GitHub Issues with label `safety` for safety-relevant items
- Each safety defect must be assessed for ASIL impact and closed before next release
- Waivers require documented justification in the issue + sign-off from the verifier

---

## 9. Release criteria

A release (version tag) may only be created when ALL of the following are true:

1. `go build ./...` — zero errors
2. `go vet ./...` — zero errors
3. `golangci-lint run` — zero errors
4. `gofusa check ./...` — zero errors
5. `go test -race -count=1 ./...` — all PASS
6. `gofusa trace` — zero orphan tags, all requirements traced+tested
7. Structural coverage ≥ 80% overall, ≥ 90% for `safety/` and `security/`
8. Independent reviewer has signed `cert/RELEASE_LOG.md`
9. `GC_LATENCY.md` is current (re-run `TestGCLatencyProfile` if safety/ or rtps/ changed)

---

## 10. Assumptions of use (AoU)

The following AoUs are placed on the integrating system (see also `HARA.md §5`):

| ID | Assumption |
|---|---|
| AoU-01 | Deployment platform ≥ 4 logical CPUs |
| AoU-02 | GOMEMLIMIT set to ≤ 70% of available RAM |
| AoU-03 | Application registers DeadlineQoS missed-deadline callback and asserts safe state |
| AoU-04 | ResourceLimitsQoS.MaxSamples set to a finite value on Reliable publishers |
| AoU-05 | No synchronous allocations > 1 MiB on the DDS hot path |
| AoU-06 | go-DDS is isolated in its own OS process |
| AoU-07 | System-level HARA performed by integrator; this SSP covers only go-DDS |

---

## 11. Plan maintenance

This plan shall be reviewed and updated:
- When the ASIL/SIL target changes
- When a new major version is released
- When the CI matrix adds a new job that affects verification completeness
- At minimum once per calendar year

**Next scheduled review:** 2027-06-09
