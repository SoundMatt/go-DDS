# Software Development Plan (SDP)
## go-DDS

**Document ID:** SDP-001  
**Version:** 1.0  
**Date:** 2026-06-09  
**Standard:** DO-178C §11.2, IEC 61508-3 §5, ISO 26262-6 §5  

---

## 1. Purpose

This Software Development Plan describes the environment, methods, tools, and
standards used to develop go-DDS software. It is a required plan under
DO-178C §11.2 and satisfies the equivalent planning requirement of IEC 61508-3.

---

## 2. Development environment

### 2.1 Host platforms

| Platform | Use |
|---|---|
| macOS 14+ (arm64 / x86_64) | Primary development |
| Ubuntu 22.04 LTS (x86_64) | CI, release builds |
| Windows Server 2022 | CI cross-check |

### 2.2 Go toolchain

| Component | Version | Source |
|---|---|---|
| Go compiler (`gc`) | ≥ 1.25 | https://go.dev/dl |
| Go standard library | matches compiler | bundled |
| golangci-lint | latest stable | GitHub Actions |
| gofusa | v0.21.0 (pinned) | `go install` in CI |

### 2.3 Development tools

| Tool | Purpose | Qualification status |
|---|---|---|
| Go compiler | Compilation, linking | See `cert/TQP-go-compiler.md` |
| `go test` | Test execution | See `cert/TQP-go-compiler.md` |
| `golangci-lint` | Static analysis | See `cert/TQP-golangci-lint.md` |
| `gofusa` | Safety analysis | See `cert/TQP-gofusa.md` |
| `git` | Version control | CM tool — no TQP required |
| GitHub Actions | CI/CD | CM tool — no TQP required |

---

## 3. Development standards

| Standard | Document |
|---|---|
| Coding standard | `CODING_STANDARD.md` |
| Error handling | `CODING_STANDARD.md §3` |
| Concurrency | `CODING_STANDARD.md §4` |
| Complexity | `CODING_STANDARD.md §5` |
| Requirements traceability | `.fusa-reqs.json` + `//fusa:req` annotations |

---

## 4. Software development process

### 4.1 Branching strategy

| Branch prefix | Purpose |
|---|---|
| `main` | Releasable baseline; protected |
| `feat/<scope>-<summary>` | New features |
| `fix/<scope>-<summary>` | Bug fixes |
| `chore/…` | Non-functional changes |

No commit is made directly to `main`. All changes enter via pull request with
at least one reviewer approval (see `SAFETY_PLAN.md §3`).

### 4.2 Commit requirements

Each commit must:
- Build cleanly (`go build ./...`)
- Pass `go vet ./...`
- Include a DCO sign-off (`Signed-off-by:` trailer)
- Reference any modified requirements via `//fusa:req` annotations

### 4.3 Release process

1. Create release branch from `main`
2. Run full release criteria checklist (see `SAFETY_PLAN.md §9`)
3. Create and push version tag `vX.Y.Z`
4. CI generates and commits safety artefacts (`gofusa release`)
5. Verifier signs `cert/RELEASE_LOG.md`

### 4.4 Traceability

Requirements in `.fusa-reqs.json` trace to:
- Implementation: `//fusa:req REQ-NNN` annotation in source
- Tests: `//fusa:test REQ-NNN` annotation in test file
- Verification: `gofusa trace` confirms no orphan or untested requirements

---

## 5. Software partitioning

| Partition | ASIL/SIL level | Packages |
|---|---|---|
| Safety-critical | ASIL-B / SIL 2 | `safety/`, `rtps/`, `security/` |
| Safety-related | ASIL-A / SIL 1 | `mock/`, root `dds` package |
| Non-safety | QM | `monitor/`, `bridge/`, `cmd/`, `examples/` |

Freedom from interference between partitions is achieved by:
- OS process isolation (see AoU-06 in `HARA.md`)
- Go type system preventing unsafe memory access across package boundaries
- No shared mutable state between safety-critical and non-safety packages

---

## 6. Deactivation of dead code

go-DDS contains no intentional dead code. Unreachable branches detected by
`go vet` or the compiler are treated as defects and removed. Fuzz targets
exercise error paths that are unreachable in normal operation.

---

## 7. Revision history

| Version | Date | Author | Changes |
|---|---|---|---|
| 1.0 | 2026-06-09 | Matt Jones | Initial release |
