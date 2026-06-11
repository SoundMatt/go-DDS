# Tool Qualification Plan — go-FuSa (`gofusa`)
## go-DDS

**Document ID:** TQP-GF-001  
**Version:** 1.0  
**Date:** 2026-06-09  
**Standard:** DO-330:2011  
**Tool:** `gofusa` v0.25.1 (`github.com/SoundMatt/go-FuSa/cmd/gofusa`)  

---

## 1. Tool identification

| Attribute | Value |
|---|---|
| Tool name | `gofusa` (go-FuSa safety analyser) |
| Tool version | v0.25.1 (pinned in CI) |
| Source | `github.com/SoundMatt/go-FuSa` |
| Output type | Static analysis report; `fmea.csv`, `safety-case.md`, `sbom.json`, `provenance.json` |

---

## 2. Tool classification (DO-330 §5)

**TQL-4 (Tool Qualification Level 4):** gofusa is a **verification tool**.
Its output (check results, traceability reports) is used as verification
evidence. Failures in gofusa could allow non-compliant code to pass undetected.

**DO-330 criteria applied:** Criteria 1 (Tool developer qualifies the tool)
supplemented by Criteria 2 (Established usage within this project).

**Classification rationale:**
- `gofusa check` output is used as evidence that coding rules are followed
- `gofusa trace` output is used as evidence that all requirements are tested
- Both are cited in `cert/SQAP.md §3.1` as objective quality gates
- An undetected false-negative in gofusa could allow a code defect to bypass
  the CI gate

---

## 3. Tool testing

### 3.1 In-project validation

gofusa's output is validated within go-DDS by:

1. **Known-good corpus:** Every existing `*_test.go` file has been reviewed to
   confirm that `gofusa check` produces zero errors (not just warnings) on
   compliant code
2. **Known-bad corpus:** The `idl/testdata/fuzz/` directory contains inputs
   that previously triggered infinite loops in the IDL parser. `gofusa check`
   was verified to flag the relevant code constructs before and after fix
3. **Traceability validation:** `gofusa trace` is run after each requirement is
   added. The output was manually verified against `.fusa-reqs.json` for
   completeness on the v0.14.x baseline

### 3.2 Upgrade validation procedure

When gofusa is upgraded (e.g. v0.25.1 → vX.Y.Z):

1. Install new version in a separate environment
2. Run `gofusa check ./...` and compare results against v0.25.1 baseline
3. Investigate any new findings — fix or document as accepted deviation
4. Update version pin in `.github/workflows/ci.yml` and `release.yml`
5. Record the upgrade in `cert/RELEASE_LOG.md`

### 3.3 Limitations and mitigations

| Known limitation | Mitigation |
|---|---|
| Static analysis only — does not detect runtime defects | Complemented by `go test -race` |
| ANA001 goroutine warnings are sometimes false positives when ctx is passed | Reviewed manually; false positives documented in `cert/DEVIATIONS.md` |
| COMP001 complexity metric is McCabe cyclomatic, not path coverage | Decision coverage measured separately by `go test -covermode=count` |

---

## 4. Version control

gofusa is version-pinned in CI:

```yaml
run: go install github.com/SoundMatt/go-FuSa/cmd/gofusa@v0.25.1
```

`go.sum` equivalence is verified by `go install` fetching the exact module hash.
Any version change requires the upgrade validation procedure (§3.2).

---

## 5. Revision history

| Version | Date | Author | Changes |
|---|---|---|---|
| 1.0 | 2026-06-09 | Matt Jones | Initial release |
