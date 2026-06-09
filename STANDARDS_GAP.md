# Standards Gap Analysis — go-DDS

**Scope:** Gap between the current go-DDS safety posture and full compliance with
IEC 61508:2010 (functional safety of E/E/PE systems) and DO-178C (airborne
software).

**Current baseline:** ASIL-B SEOOC under ISO 26262:2018, verified by `HARA.md`,
`GC_LATENCY.md`, and `safety-case.md`.

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
| G-61508-01 | No formal software safety plan document | Part 1 §10, Part 3 §5 | Medium: create `SAFETY_PLAN.md` |
| G-61508-02 | No independence between developer and verifier roles | Part 3 §7.8.2 | Organisational: requires a second person to sign off test results |
| G-61508-03 | No coding standard reference (e.g. MISRA-Go / CERT-Go) | Part 3 B.3.1 | Low: adopt a subset; gofusa rules partially cover this |
| G-61508-04 | No formal proof or model checking for critical paths | Part 3 B.5.3 (SIL 3-4 only, recommended for SIL 2) | High: would require TLA+/SPIN; out of scope for SEOOC |
| G-61508-05 | No traceability to hardware platform fault model | Part 2 §7.4 | Medium: requires ECU-specific FMEDA from the integrator |
| G-61508-06 | No documented proof of absence of floating-point errors | Part 3 B.4.5 | Low: go-DDS uses no float in safety-critical paths |
| G-61508-07 | Diagnostic coverage metric not quantified | Part 7 (diagnostic coverage tables) | Medium: requires DC% calculation per subsystem |
| G-61508-08 | `standard: "generic"` in `.fusa.json` rather than `"iec61508"` | Internal | Trivial: update one field once gofusa supports iec61508 schema |

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
| G-DO178-01 | No Plan for Software Aspects of Certification (PSAC) | §11.1 | High: requires formal PSAC document reviewed with a DER/DAST |
| G-DO178-02 | No Software Development Plan (SDP) citing DO-178C | §11.2 | Medium: extend CLAUDE.md into a formal SDP |
| G-DO178-03 | No Software Verification Plan (SVP) | §11.3 | Medium: CI matrix partially covers this; needs formalisation |
| G-DO178-04 | No Software Configuration Management Plan (SCMP) | §11.4 | Low: git + signed tags; needs a written SCMP |
| G-DO178-05 | No Software Quality Assurance Plan (SQAP) | §11.5 | Medium: QA role currently merged with development |
| G-DO178-06 | No MC/DC (Modified Condition/Decision Coverage) | §6.4.4.2 table A-7 (DAL C requires decision coverage; DAL B requires MC/DC) | Medium for DAL C: decision coverage achievable with go test -covermode=count |
| G-DO178-07 | No tool qualification (TQ) for gofusa, golangci-lint, Go compiler | §12 (tools producing output used in certification) | High: TQ requires DO-330 criteria 1–5 for each tool |
| G-DO178-08 | No formal low-level requirements (LLRs) — only high-level | §6.3 | High: current reqs are HLRs; LLRs must derive from architecture and be individually tested |
| G-DO178-09 | No independence (IV&V) — same team writes and verifies | §7.3 | Organisational: requires a separate verification entity |
| G-DO178-10 | No Problem Report formal closure process | §11.20 | Low: GitHub Issues partially satisfy; needs formal lifecycle states |
| G-DO178-11 | No structural coverage analysis report | §6.4.4 | Medium: `go test -coverprofile` is the evidence; needs a formal report artefact |
| G-DO178-12 | Go runtime not qualified as a tool or RTOS | §12 / §7.2 | High: Go GC and scheduler are not certified for avionics; this is a blocking gap for DAL A/B |

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

## 3. Summary comparison

| Dimension | IEC 61508 SIL 2 gap | DO-178C DAL C gap | Priority |
|---|---|---|---|
| Safety planning docs | Safety Plan needed | PSAC + 4 plans needed | High for DO-178C |
| Requirements (HLRs) | Present | Present | Done |
| Low-level requirements | Not required at SIL 2 | Required | High for DO-178C |
| Structural coverage | Statement/decision (BC) | Decision coverage | Medium |
| Tool qualification | Recommended | Mandatory | Blocking for DO-178C |
| Coding standard | Recommended | Mandatory (subset) | Medium |
| Independence (IV&V) | Recommended at SIL 2 | Mandatory at DAL C | Blocking for DO-178C |
| Formal methods | Optional at SIL 2 | N/A (DAL C) | Low |
| GC / runtime cert | N/A (SEOOC) | High risk (Go not certified) | Blocking for DAL A/B |

**Bottom line:** go-DDS is close to IEC 61508 SIL 2 compliance — the gaps
(G-61508-01 through G-61508-07) are achievable with 2-4 weeks of documentation
work plus an independent review. DO-178C DAL C is feasible but requires
organisational changes (dedicated IV&V, DER engagement, tool qualification) on
top of a similar document package; budget 6-12 months for a first certification.
DO-178C DAL B/A is not practical with the current Go runtime and is out of scope.
