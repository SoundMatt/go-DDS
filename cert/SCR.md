# Structural Coverage Report (SCR)
## go-DDS

**Document ID:** SCR-001  
**Version:** 1.0  
**Date:** 2026-06-09  
**Standard:** DO-178C §6.4.4.2, IEC 61508-3 B.6.2, ISO 26262-6 Table 10  
**Coverage type:** Statement coverage (`go test -covermode=count`)  
**Coverage level required:** Decision coverage (DAL C minimum)  

> **Note on Go coverage tooling:** `go test -covermode=count` instruments every
> statement and counts executions. Go 1.20+ additionally supports `-covermode=atomic`
> for concurrent tests. Decision-level coverage is approximated because the Go
> coverage tool instruments at the statement level including branch targets.
> Each `if/for/switch/select` arm becomes a distinct counted statement.

---

## 1. Measurement parameters

| Parameter | Value |
|---|---|
| Go version | 1.25 |
| Coverage mode | `count` |
| Test flags | `-short -race` (excluding 30s GC profile) |
| Date measured | 2026-06-09 |
| Commit | current `main` |

---

## 2. Per-package coverage

### Safety-critical packages (threshold ≥ 90%)

| Package | Coverage | Threshold | Status |
|---|---|---|---|
| `safety` | **98.7%** | 90% | PASS |
| `security` | **96.4%** | 90% | PASS |
| `rtps` | **80.5%** | 90% | **GAP** (see §3) |

### Safety-related packages (threshold ≥ 85%)

| Package | Coverage | Threshold | Status |
|---|---|---|---|
| `github.com/SoundMatt/go-DDS` (root) | **93.6%** | 85% | PASS |
| `mock` | **97.4%** | 85% | PASS |
| `cdr` | **86.0%** | 85% | PASS |

### Other packages (threshold ≥ 70%, informational)

| Package | Coverage | Status |
|---|---|---|
| `admin` | 96.1% | PASS |
| `bridge/domain` | 95.0% | PASS |
| `bridge/grpc` | 75.6% | PASS |
| `bridge/mqtt` | 97.5% | PASS |
| `bridge/rest` | 78.8% | PASS |
| `bridge/wan` | 92.8% | PASS |
| `config` | 100.0% | PASS |
| `idl` | 87.2% | PASS |
| `idl/roundtrip` | 61.3% | GAP (see §3) |
| `monitor` | 90.0% | PASS |
| `pool` | 97.5% | PASS |
| `record` | 98.5% | PASS |
| `rpc` | 88.0% | PASS |
| `services` | 98.5% | PASS |
| `shmem` | 93.7% | PASS |
| `testutil` | 93.1% | PASS |
| `tsn` | 94.4% | PASS |
| `xtypes` | 98.4% | PASS |

---

## 3. Coverage gaps and justifications

### 3.1 `rtps` — 80.5% (below 90% threshold)

**Gap size:** 9.5 percentage points  
**Uncovered paths (representative):**

| Path | Reason uncovered |
|---|---|
| IPv6 multicast socket creation error paths | Require kernel-level fault injection not available in CI |
| RTPS fragment timeout eviction (10s timer) | Excluded by `-short` flag; covered in non-short test |
| CycloneDDS interop fallback | Requires CycloneDDS install (`-tags interop`) |
| SEDP lease expiry edge case (lease < heartbeat period) | Timing-sensitive; covered by manual test |

**Justification:** The uncovered paths are error handling for hardware/network
faults that cannot be injected in a unit test environment. They are verified by:

1. Code review (logic is straightforward error propagation)
2. `go vet` confirms no unreachable code
3. Fuzz corpus exercises the packet parser, which covers a distinct set of paths

**Waiver:** This gap is accepted for the current SEOOC ASIL-B claim. For a
standalone ASIL-C certification, fault injection testing would be required.
Waiver ID: `SCR-WAI-001` — approved in `cert/DEVIATIONS.md`.

### 3.2 `idl/roundtrip` — 61.3%

**Gap size:** 8.7 percentage points below 70% informational threshold  
**Justification:** `idl/roundtrip` is a test support package, not safety-critical.
Coverage of the roundtrip harness itself is informational. The IDL parser
(`idl/` at 87.2%) and generator are the safety-relevant components.

**Action:** Add tests for uncovered roundtrip paths in the next minor version.
No release blocker.

---

## 4. Coverage trends

| Version | `safety` | `security` | `rtps` | Overall |
|---|---|---|---|---|
| v0.2.0 | 88.7% | 94.1% | 78.2% | 86.3% |
| v0.14.x (current) | 98.7% | 96.4% | 80.5% | ~91% |

Coverage has improved across all safety packages since v0.2.0.

---

## 5. Command to reproduce

```
go test -cover -short ./...
```

For a combined HTML report:

```
go test -covermode=count -coverprofile=cover.out -short ./...
go tool cover -html=cover.out -o cover.html
```

---

## 6. Revision history

| Version | Date | Author | Changes |
|---|---|---|---|
| 1.0 | 2026-06-09 | Matt Jones | Initial release with measured values |
