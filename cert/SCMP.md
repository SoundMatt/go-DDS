# Software Configuration Management Plan (SCMP)
## go-DDS

**Document ID:** SCMP-001  
**Version:** 1.0  
**Date:** 2026-06-09  
**Standard:** DO-178C §11.4, IEC 61508-3 §6, ISO 26262-8 §7  

---

## 1. Purpose

This SCMP defines how go-DDS software items are identified, controlled,
tracked, and archived to ensure reproducibility of every release and traceability
of changes to safety requirements.

---

## 2. Configuration items

| ID | Item | Location | Controlled by |
|---|---|---|---|
| CI-SRC | Source code | `*.go` files | Git |
| CI-TEST | Test code | `*_test.go` files | Git |
| CI-REQ | Requirements manifest | `.fusa-reqs.json` | Git |
| CI-PLANS | Certification plans | `cert/*.md` | Git |
| CI-HARA | Hazard analysis | `HARA.md` | Git |
| CI-SC | Safety case | `safety-case.md`, `fmea.csv` | Git + gofusa |
| CI-SBOM | Software Bill of Materials | `sbom.json` | Git + gofusa release |
| CI-PROV | Build provenance | `provenance.json` | Git + gofusa release |
| CI-ARTMF | Artefact manifest | `artifact-manifest.json` | Git + gofusa release |
| CI-CI | CI pipeline definition | `.github/workflows/*.yml` | Git |
| CI-MOD | Go module definition | `go.mod`, `go.sum` | Git |

---

## 3. Versioning scheme

go-DDS uses **semantic versioning** (`vMAJOR.MINOR.PATCH`):

| Increment | When |
|---|---|
| PATCH | Bug fixes, documentation updates, no API change |
| MINOR | Backward-compatible new features |
| MAJOR | Breaking API changes |

Pre-release identifiers (`-alpha.1`, `-rc.1`) may be used for internal milestones.
Only tags matching `v[0-9]*.[0-9]*.[0-9]*` are treated as releases by CI.

---

## 4. Baseline identification

A **baseline** is a tagged commit in the `main` branch. The baseline is
identified by:

- Git commit SHA (40 hex characters), embedded in `provenance.json`
- Version tag (e.g. `v0.14.3`)
- Build timestamp in `provenance.json`

The combination of tag + commit SHA is the unique identifier for any released
artefact. Reproducible builds: `go build` with the same Go toolchain version
and `go.sum` produces byte-identical binaries.

---

## 5. Change control

### 5.1 All changes

1. Author creates a branch from `main`
2. Author implements and tests the change
3. Author opens a pull request with description referencing affected requirements
4. Automated CI must be green before merge
5. For safety-critical changes: independent reviewer approval required
6. Merge to `main` uses squash merge to preserve linear history

### 5.2 Safety-critical changes (affecting `safety/`, `rtps/`, `security/`)

In addition to §5.1:
- The PR description must identify which requirements in `.fusa-reqs.json`
  are affected and how
- `gofusa trace` must show no new orphan tags after the change
- Verifier sign-off is recorded in the PR before merge

### 5.3 Emergency fixes (safety defects in production)

1. Create `fix/` branch from the affected release tag
2. Implement fix with regression test
3. Obtain expedited verifier review (within 24 hours for S2+ defects)
4. Tag as `vX.Y.Z+1` (PATCH increment)
5. Backport to `main` if applicable

---

## 6. Baseline archiving

Every release tag is:

1. Pushed to `github.com/SoundMatt/go-DDS` (permanent, audit-logged)
2. Recorded in the Go module proxy (`proxy.golang.org`) — immutable once published
3. Accompanied by `sbom.json` and `provenance.json` in the repo root

Integrators shall pin go-DDS using `go get github.com/SoundMatt/go-DDS@vX.Y.Z`
to ensure a reproducible build. Using `@latest` or `@main` in safety contexts
is not permitted.

---

## 7. Tool version control

Tool versions used in CI are pinned in `.github/workflows/ci.yml`:

```yaml
go-version: "1.25"          # Go toolchain
gofusa: @v0.19.0             # Safety analysis
actions/checkout@v5          # CI checkout
actions/setup-go@v6          # Go install
```

Tool version changes require a PR with justification and re-run of full
verification.

---

## 8. Problem reporting

All defects are tracked as GitHub Issues. Safety-relevant issues are labelled
`safety`. See `cert/PRP.md` for the full Problem Reporting Procedure.

---

## 9. Revision history

| Version | Date | Author | Changes |
|---|---|---|---|
| 1.0 | 2026-06-09 | Matt Jones | Initial release |
