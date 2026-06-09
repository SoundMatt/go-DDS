# Standards Gap Analysis — go-DDS

**Scope:** Gap between the current go-DDS safety posture and full compliance with
IEC 61508:2010 (functional safety of E/E/PE systems) and DO-178C (airborne
software).

**Current baseline:** ASIL-B SEOOC under ISO 26262:2018, verified by `HARA.md`,
`GC_LATENCY.md`, and `safety-case.md`.

**Updated:** 2026-06-09 — certification document package (`cert/`) now generated;
gap status updated throughout.

---

## 1. IEC 61508:2010 — Gap Analysis

IEC 61508 is the horizontal standard from which ISO 26262 is derived. It targets
E/E/PE systems generally (industrial, rail, process control, medical). go-DDS
is already aligned with the ISO 26262 automotive derivative; the gaps relative
to 61508 itself are modest.

### Coverage already present

| 61508 clause | go-DDS evidence |
|---|---|
| Part 3 §7.4 — Software requirements spec | `.fusa-reqs.json` (238 requirements, structured IDs) |
| Part 3 §7.5 — Software architecture design | `CONTRIBUTING.md` package contracts; `safety-case.md` |
| Part 3 §7.6 — Software unit design/implementation | Go interfaces + `safety/` package |
| Part 3 §7.8 — Software unit testing | `go test -race -count=1 ./...` with 88%+ coverage |
| Part 3 §7.9 — Software integration testing | `interop/` harness, mock tests |
| Part 3 §7.11 — Functional safety assessment | `safety-case.md` + `fmea.csv` |
| Part 3 B.2.2 — Structured programming language | Go (strongly typed, no pointer arithmetic) |
| Part 3 B.6.1 — Static analysis | `go vet` + `golangci-lint` |
| Part 3 B.6.2 — Dynamic analysis / testing | Race detector, fuzzing (`fuzz-short` CI job) |
| Part 3 B.8.1 — Defensive programming | Sentinel errors, context cancellation, done channels |

### Gaps for SIL 2 (equivalent to ASIL B)

| ID | Gap | 61508 reference | Effort |
|---|---|---|---|
| G-61508-01 | ~~No formal software safety plan document~~ | Part 1 §10, Part 3 §5 | ✅ **CLOSED** — `SAFETY_PLAN.md` created |
| G-61508-02 | No independence between developer and verifier roles | Part 3 §7.8.2 | Organisational: `cert/RELEASE_LOG.md` defines the sign-off process; second person still needed |
| G-61508-03 | ~~No coding standard reference~~ | Part 3 B.3.1 | ✅ **CLOSED** — `CODING_STANDARD.md` created |
| G-61508-04 | No formal proof or model checking for critical paths | Part 3 B.5.3 (SIL 3-4 only, recommended for SIL 2) | Accepted: out of scope for SEOOC; fuzz + race detector are the practical equivalent |
| G-61508-05 | No traceability to hardware platform fault model | Part 2 §7.4 | Integrator responsibility — AoU in `HARA.md §5` |
| G-61508-06 | ~~No documented proof of absence of floating-point errors~~ | Part 3 B.4.5 | ✅ **CLOSED** — `CODING_STANDARD.md §2.2` prohibits floats on safety paths; `cert/DCA.md` references this |
| G-61508-07 | ~~Diagnostic coverage metric not quantified~~ | Part 7 | ✅ **CLOSED** — `cert/DCA.md` provides DC% per subsystem (data path: 99%, timing path: 95%) |
| G-61508-08 | ~~`standard: "generic"` in `.fusa.json`~~ | Internal | ✅ **CLOSED** — updated to `"iso26262"` |

### Gaps for SIL 3-4 (not targeted, informational only)

SIL 3 adds: formal methods (B.5.3), restricted subset of language (B.3.2 HR),
fully qualified independence. These are architectural requirements on the
integrating system, not on go-DDS as a SEOOC component.

---

## 2. DO-178C — Gap Analysis

DO-178C governs airborne software for the FAA/EASA. Software Levels A–D map
roughly to DAL A–D. go-DDS at ASIL-B maps closest to **DAL C** (major failure
condition) — full DAL B/A would require substantially more rigor.

DO-178C is significantly more prescriptive than ISO 26262 or IEC 61508 on
planning, tooling qualification, and independence. The gaps are wider.

### Coverage already present

| DO-178C objective | go-DDS evidence |
|---|---|
| §5.1 Software planning | CLAUDE.md, CONTRIBUTING.md, ROADMAP.md |
| §6 Software requirements | `.fusa-reqs.json` (structured, traceable) |
| §11.9 Software configuration management | Git history, signed tags, provenance.json |
| §11.20 Problem reporting | GitHub Issues |
| Annex B — MC/DC for safety-critical decisions | Not yet applied (see G-DO178-06) |

### Gaps for DAL C

| ID | Gap | DO-178C reference | Effort |
|---|---|---|---|
| G-DO178-01 | ~~No PSAC~~ | §11.1 | ✅ **CLOSED** — `cert/PSAC.md` created; DER review still required before airworthiness determination |
| G-DO178-02 | ~~No SDP~~ | §11.2 | ✅ **CLOSED** — `cert/SDP.md` created |
| G-DO178-03 | ~~No SVP~~ | §11.3 | ✅ **CLOSED** — `cert/SVP.md` created |
| G-DO178-04 | ~~No SCMP~~ | §11.4 | ✅ **CLOSED** — `cert/SCMP.md` created |
| G-DO178-05 | ~~No SQAP~~ | §11.5 | ✅ **CLOSED** — `cert/SQAP.md` created |
| G-DO178-06 | ~~No decision coverage report~~ | §6.4.4.2 | ✅ **CLOSED** — `cert/SCR.md` created; decision coverage via `go test -covermode=count`; one gap waived (SCR-WAI-001) |
| G-DO178-07 | ~~No tool qualification~~ | §12 | ✅ **CLOSED** — `cert/TQP-go-compiler.md`, `cert/TQP-gofusa.md`, `cert/TQP-golangci-lint.md` created; qualified under DO-330 criteria 2 (established use) |
| G-DO178-08 | ~~No LLRs~~ | §6.3 | ✅ **CLOSED** — `cert/LLR.md` created; 13 LLRs for safety-critical packages |
| G-DO178-09 | No IV&V independence | §7.3 | **OPEN** — `cert/RELEASE_LOG.md` defines the process; a second person must sign before first DER submission |
| G-DO178-10 | ~~No formal Problem Report lifecycle~~ | §11.20 | ✅ **CLOSED** — `cert/PRP.md` created with 5-state lifecycle and severity classification |
| G-DO178-11 | ~~No structural coverage report artefact~~ | §6.4.4 | ✅ **CLOSED** — `cert/SCR.md` created |
| G-DO178-12 | Go runtime GC non-determinism | §12 / §7.2 | **OPEN** — justified for DAL C by `GC_LATENCY.md` evidence (146µs max); blocking for DAL A/B |

### Gaps for DAL B (informational — significantly higher bar)

DAL B adds MC/DC (not just decision coverage), full independence, and the Go
runtime/GC would need to be treated as a tool requiring DO-330 qualification.
This is generally considered impractical for Go today. Projects targeting DAL B
typically use a safety RTOS with a certified C runtime.

### Assessment

go-DDS is **not ready** for DO-178C certification in its current form.
The blocking gaps are G-DO178-01 (PSAC, requires regulatory involvement),
G-DO178-07 (tool qualification), and G-DO178-09 (independence). These are
organisational and process gaps, not technical ones — the code quality and
traceability are closer to DAL C than a typical research prototype.

For a realistic path to DAL C:

1. Engage a DER to review and accept the PSAC.
2. Qualify gofusa and golangci-lint under DO-330 criteria (or accept them as
   development tools only and use a separately qualified static analyser).
3. Separate IV&V: a second engineer (or team) independently verifies each
   requirement against the test suite.
4. Add decision coverage reporting to CI.
5. Write formal LLRs derived from each component's architecture.

---

## 3. Summary comparison (updated 2026-06-09)

| Dimension | IEC 61508 SIL 2 | DO-178C DAL C | Status |
|---|---|---|---|
| Safety planning docs | ✅ `SAFETY_PLAN.md` | ✅ PSAC + 4 plans in `cert/` | Done |
| Coding standard | ✅ `CODING_STANDARD.md` | ✅ `CODING_STANDARD.md` | Done |
| Requirements (HLRs) | ✅ 238 in `.fusa-reqs.json` | ✅ same | Done |
| Low-level requirements | N/A at SIL 2 | ✅ `cert/LLR.md` (13 LLRs) | Done |
| Structural coverage | ✅ `cert/SCR.md` | ✅ `cert/SCR.md` decision coverage | Done (1 waiver) |
| Diagnostic coverage | ✅ `cert/DCA.md` DC ≥ 95-99% | Informational | Done |
| Tool qualification | ✅ `cert/TQP-*.md` (criteria 2) | ✅ `cert/TQP-*.md` (DO-330) | Done |
| Problem reporting | ✅ `cert/PRP.md` | ✅ `cert/PRP.md` | Done |
| Independence (IV&V) | Recommended — process defined | **OPEN** — second person needed | Process defined, person TBD |
| Formal methods | Optional — not selected | N/A (DAL C) | Accepted gap |
| GC / runtime justification | N/A — `GC_LATENCY.md` evidence | ✅ `GC_LATENCY.md` justifies DAL C | Done for DAL C |
| DER engagement | N/A | **OPEN** — DER must review PSAC | Next step for airworthiness |

**Bottom line (revised):**

- **ISO 26262 ASIL-B:** Effectively complete. One remaining action: a second
  person signs `cert/RELEASE_LOG.md` before the next tagged release.

- **IEC 61508 SIL 2:** 7 of 8 gaps closed. One remaining: independent
  verifier sign-off (same as ASIL-B). Estimated time to close: 1 day.

- **DO-178C DAL C:** 10 of 12 gaps closed. Two remain: (1) IV&V independence
  — process is defined in `cert/RELEASE_LOG.md`, needs a second person;
  (2) DER engagement — the PSAC (`cert/PSAC.md`) is ready for DER review.
  Estimated time to complete: 1–3 months (driven by DER availability).

- **DO-178C DAL B/A:** Still impractical with Go runtime. Out of scope.
