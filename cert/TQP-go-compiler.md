# Tool Qualification Plan — Go Compiler (`gc`)
## go-DDS

**Document ID:** TQP-GC-001  
**Version:** 1.0  
**Date:** 2026-06-09  
**Standard:** DO-330:2011 (Software Tool Qualification Considerations)  
**Tool:** Go compiler (`gc`) version ≥ 1.25  

---

## 1. Tool identification

| Attribute | Value |
|---|---|
| Tool name | `gc` (Go compiler) |
| Tool version | ≥ 1.25 (pinned per `.github/workflows/ci.yml`) |
| Source | https://go.dev/dl |
| Vendor | Google LLC / Go Authors (open source, BSD licence) |
| Output type | Native binaries, object files, test executables |

---

## 2. Tool classification (DO-330 §5)

**TQL-5 (Tool Qualification Level 5):** The Go compiler is a **development
tool** (not a verification tool). Its output (compiled binaries) is verified
by the test suite. Errors introduced by the compiler would be detected during
functional testing.

**Rationale for TQL-5:**
- The compiler does not automatically insert code into the final product
  without the developer's intent
- The compiler's output is independently verified by `go test -race` running
  the compiled binary against known expected outputs
- Compiler bugs that escape testing would need to produce incorrect output
  for specific constructs while passing all existing tests — this is detected
  by the diversity of the test suite (3 platforms, 2 Go versions in CI)

**DO-330 criteria applied:** Criteria 2 (Established usage — broad use by
safety-critical projects with documented errata review).

---

## 3. Established usage evidence

The Go compiler is used by the following safety-relevant projects (publicly
documented):

| Project | Domain |
|---|---|
| Kubernetes (CNCF) | Cloud infrastructure |
| Cilium (CNCF, eBPF networking) | Network safety functions |
| CockroachDB (financial systems) | High-availability databases |
| Cloudflare edge network | Critical internet infrastructure |
| Multiple automotive OEMs (internal tools) | ADAS data pipelines |

The Go compiler has been in production use since 2009. The Go team publishes
release notes and security advisories at https://go.dev/doc/devel/release.

---

## 4. Errata review

The Go team publishes security fixes at https://go.dev/security. Before each
go-DDS release, the responsible engineer shall:

1. Review Go release notes for the version used in CI
2. Note any compiler bugs that could affect safety-critical constructs in
   go-DDS (goroutine scheduling, atomic operations, interface dispatch)
3. Record the review outcome in `cert/RELEASE_LOG.md`

No known compiler errata affect the constructs used in go-DDS safety-critical
packages as of the date of this document.

---

## 5. Tool usage constraints

To keep the compiler within the bounds of this TQP:

| Constraint | Reason |
|---|---|
| No `-gcflags='-e'` (ignore all errors) | Must not suppress diagnostic output |
| No `-trimpath` on safety packages | Preserves file paths in stack traces |
| CGo disabled in safety packages (`CGO_ENABLED=0` for safety builds) | Avoids C compiler chain |
| Version pinned in CI to a specific `go-version` | Prevents accidental upgrade |

---

## 6. Additional verification

Given TQL-5 qualification, no additional tool testing is required beyond the
functional test suite. However, the following provide additional confidence:

- CI runs on 3 distinct operating systems — compiler output verified on each
- CI runs on 2 Go versions simultaneously — compiler behavioural consistency verified
- Race detector is a distinct verification mode that independently checks
  concurrent correctness of compiled output

---

## 7. Revision history

| Version | Date | Author | Changes |
|---|---|---|---|
| 1.0 | 2026-06-09 | Matt Jones | Initial release |
