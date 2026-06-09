# Tool Qualification Plan — golangci-lint
## go-DDS

**Document ID:** TQP-GL-001  
**Version:** 1.0  
**Date:** 2026-06-09  
**Standard:** DO-330:2011  
**Tool:** `golangci-lint` (latest stable, per CI)  

---

## 1. Tool identification

| Attribute | Value |
|---|---|
| Tool name | `golangci-lint` |
| Tool version | Latest stable (managed by `actions/setup-go` cache in CI) |
| Source | https://github.com/golangci/golangci-lint |
| Output type | Static analysis findings (exit code 0 = clean, 1 = findings) |

---

## 2. Tool classification

**TQL-4:** golangci-lint is a verification tool whose zero-finding result is
used as evidence of coding standard compliance.

**DO-330 criteria applied:** Criteria 2 (Established usage):
- golangci-lint is the standard Go static analysis orchestrator, used by
  thousands of projects including Kubernetes, Prometheus, and Istio
- Its constituent linters (`errcheck`, `staticcheck`, `govet`) are individually
  maintained open-source tools with long track records

---

## 3. Tool testing within this project

### 3.1 Baseline validation

At each golangci-lint version change:

1. Run `golangci-lint run ./...` from a clean checkout
2. Confirm zero findings on the current `main` baseline
3. Investigate any new findings introduced by the new version
4. Either fix, or document and suppress with `//nolint:<linter>` plus justification

### 3.2 False negative detection

golangci-lint is used in combination with `gofusa check` and `go vet`. The
three tools have overlapping but non-identical rule sets. A false negative in
one tool is likely detected by another.

Additionally, the race detector (`go test -race`) provides runtime verification
that is independent of all static analysis tools.

### 3.3 Configuration

`.golangci.yml` in the repo root defines the active linter set. Changes to
this file require PR review and are treated as a change to the tool
configuration (version-controlled).

---

## 4. Revision history

| Version | Date | Author | Changes |
|---|---|---|---|
| 1.0 | 2026-06-09 | Matt Jones | Initial release |
