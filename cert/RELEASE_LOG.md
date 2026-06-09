# Release Log
## go-DDS

**Document ID:** REL-001  
**Purpose:** Records independent verifier sign-off for each release.  
Each release entry is appended before the version tag is created.

---

## Release v0.14.3

**Date:** 2026-06-09  
**Tag:** `v0.14.3` (pending)  
**Commit:** (to be filled on tag creation)  
**Go version used in CI:** 1.25 / 1.26  

### Release checklist

| # | Criterion | Result | Notes |
|---|---|---|---|
| 1 | `go build ./...` — zero errors | PASS | |
| 2 | `go vet ./...` — zero errors | PASS | |
| 3 | `golangci-lint run` — zero errors | PASS | |
| 4 | `gofusa check ./...` — zero errors | PASS | 195 warnings, 274 infos — all reviewed |
| 5 | `go test -race -count=1 ./...` — all PASS | PASS | All packages green |
| 6 | `gofusa trace` — zero orphan tags | PASS | 238 requirements traced+tested |
| 7 | Coverage ≥ 80% overall, ≥ 90% safety pkgs | PARTIAL | `rtps` at 80.5%; SCR-WAI-001 accepted |
| 8 | Independent reviewer sign-off | PENDING | See §Verifier sign-off below |
| 9 | `GC_LATENCY.md` current | PASS | Measured 2026-06-09; GC max 146µs |

### Scope of this release

- Added `TestGCLatencyProfile` with 30s workload evidence
- Generated `GC_LATENCY.md` with GSN formal argument
- Created `HARA.md` (tabletop HARA, ASIL-B derivation)
- Created `cmd/latmon` continuous monitoring tool
- Generated `STANDARDS_GAP.md` (IEC 61508 / DO-178C gap analysis)
- Fixed orphan tags REQ-SAFETY-018/019 in `.fusa-reqs.json`
- Created full certification document package (`cert/`)

### gofusa check output summary

```
Summary: 469 total  0 errors  195 warnings  274 infos
Result:  PASS
```

### Verifier sign-off

> **Instructions:** An independent reviewer (not the author of this release's
> changes) shall fill in the following section after reviewing:
> (a) the CI run logs for this commit,
> (b) `cert/SCR.md` coverage values,
> (c) `HARA.md` for correctness of ASIL derivation,
> (d) `GC_LATENCY.md` for adequacy of evidence.
>
> Then append their name, date, and statement below.

**Reviewer:** _(pending)_  
**Date:** _(pending)_  
**Statement:** _(pending — e.g. "I have reviewed the CI logs, coverage report, HARA, and GC latency profile for v0.14.3. I find the verification evidence complete and the safety claims supported. APPROVED.")_

---

## Earlier releases

Earlier releases did not use this formal log. The git tag history and
CI run logs are the verification record for those versions.

---

*Template for future releases:*

```markdown
## Release vX.Y.Z

**Date:** YYYY-MM-DD
**Tag:** `vX.Y.Z`
**Commit:** <sha>
**Go version:** X.Y

### Release checklist
[copy table from above, fill in PASS/FAIL]

### gofusa check output summary
[paste summary line]

### Verifier sign-off
**Reviewer:**
**Date:**
**Statement:**
```
