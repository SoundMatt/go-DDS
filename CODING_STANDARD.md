# Coding Standard
## go-DDS — ISO 26262 / IEC 61508

**Document ID:** CS-001  
**Version:** 1.0  
**Date:** 2026-06-09  
**Standards:** IEC 61508-3:2010 B.3.1, ISO 26262-6:2018 §8.4.4  

---

## 1. Purpose

This document defines the coding standard for go-DDS. Compliance is enforced
automatically by the CI pipeline (`go vet`, `golangci-lint`, `gofusa check`).
Deviations require documented justification in the source file and sign-off
from the verifier.

The standard adopts the Go language specification, Effective Go, and the
Google Go Style Guide as its base, with additional rules for safety-critical
code.

---

## 2. Language subset

### 2.1 Permitted in all packages

- Standard Go language features (interfaces, goroutines, channels, defer)
- `sync`, `sync/atomic`, `context`, `io`, `fmt`, `errors` standard packages
- Structured error wrapping: `fmt.Errorf("…: %w", err)`
- `go vet`-clean code

### 2.2 Prohibited in safety-critical packages (`safety/`, `rtps/`, `security/`)

| Construct | Reason | Reference |
|---|---|---|
| `unsafe` package | Bypasses type safety | IEC 61508-3 B.3.2 |
| CGo | Escapes Go memory model | IEC 61508-3 B.3.2 |
| `recover()` without documented justification | Masks faults | IEC 61508-3 B.2.2 |
| `panic()` from public API | Unpredictable termination | ISO 26262-6 §8.4.2 |
| `sync.Mutex` wrapping `sync.Map` | sync.Map is self-synchronising | CLAUDE.md |
| `goto` | Unstructured control flow | IEC 61508-3 B.3.1 |
| Floating-point arithmetic on safety paths | Non-deterministic rounding | IEC 61508-3 B.4.5 |

### 2.3 Permitted with justification

- `reflect` package — only in codec/serialisation paths, not safety-critical logic
- `runtime` package — only for diagnostics (`ReadMemStats`) or test setup
- `go:linkname` — prohibited in safety packages; requires DER waiver elsewhere

### 2.4 CGo boundary

CGo is permitted only in `cyclone/` (a non-safety backend, build tag `cyclone`).
The safety-critical packages form a pure-Go layer that can be validated without
any C toolchain.

---

## 3. Error handling rules

| Rule | Enforcement |
|---|---|
| All errors returned by public API must wrap a `dds.Err*` sentinel | gofusa ANA006 (warning), code review |
| Error return values must be checked before using other returned values | gofusa ANA007 |
| `_` in LHS of multi-value call is prohibited | gofusa LINT001 |
| `panic()` in library code is prohibited | gofusa LINT002 |
| `os.WriteFile` / `os.Create` must use mode ≤ 0640 | gofusa CYBER017 |

---

## 4. Concurrency rules

| Rule | Enforcement |
|---|---|
| Goroutines must receive `done <-chan struct{}` or `ctx context.Context` at startup | gofusa ANA001 (warning) |
| Goroutines must not be spawned unboundedly inside loops | gofusa ANA002 (warning) |
| Shared state must use `sync/atomic` or channel communication | go vet (race detector) |
| `sync.Map` must not be wrapped in `sync.Mutex` | Code review |
| `close()` on a channel must happen in exactly one goroutine | Code review |

---

## 5. Complexity rules

| Metric | Threshold | Enforcement |
|---|---|---|
| Cyclomatic complexity | ≤ 10 per function | gofusa COMP001 |
| Function length | ≤ 80 lines (recommended) | Code review |
| Package coupling | No import cycles | `go build` |

---

## 6. Naming and structure

| Rule | Example |
|---|---|
| Exported types use PascalCase | `Publisher`, `QoS`, `Sample` |
| Error sentinels are package-level `var Err… = errors.New(…)` | `var ErrClosed = errors.New("dds: closed")` |
| Test files for unexported types: `package foo` | `rtps/packet_test.go` |
| Integration test files: `package foo_test` | `rtps/rtps_test.go` |
| Safety requirement annotations: `//fusa:req REQ-SAFETY-NNN` | `safety/queue.go` |

---

## 7. Documentation rules

- No multi-line comment blocks on functions (single-line max per CLAUDE.md)
- `WHY` comments are mandatory for non-obvious invariants and workarounds
- `//fusa:req` annotations are mandatory for all functions implementing a
  safety requirement from `.fusa-reqs.json`

---

## 8. Deviation procedure

A deviation from this standard is permitted when:

1. The deviation is identified by ID (e.g. `CS-DEV-001`)
2. The rationale is documented in a comment adjacent to the deviation
3. The risk introduced is assessed (none / tolerable / requires review)
4. The verifier has reviewed and accepted the deviation

Deviations are tracked in `cert/DEVIATIONS.md`.

---

## 9. Tool enforcement summary

| Tool | Rules enforced | CI job |
|---|---|---|
| `go vet` | Race conditions, invalid format strings, unreachable code | `lint` |
| `golangci-lint` | 40+ linters including `errcheck`, `gocritic`, `staticcheck` | `lint` |
| `gofusa check` | LINT001/002, ANA006/007, COMP001, CYBER017, + 30 others | `gofusa` |
| `go test -race` | Data races at runtime | `test-mock`, `test-rtps` |
| `go test -fuzz` | Input validation, parser robustness | `fuzz-short` |
