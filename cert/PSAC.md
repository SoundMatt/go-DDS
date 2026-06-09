# Plan for Software Aspects of Certification (PSAC)
## go-DDS

**Document ID:** PSAC-001  
**Version:** 1.0  
**Date:** 2026-06-09  
**Standard:** DO-178C §11.1  
**Applicability:** DAL C (Design Assurance Level C — major failure condition)  

> **DER note:** This PSAC is prepared for a component (library) supplied as
> a COTS-equivalent software item. The integrating system is responsible for
> the airworthiness determination under 14 CFR Part 21 / CS-25.1309. This
> document establishes the basis for the DER to accept go-DDS as a qualified
> software component for DAL C applications.

---

## 1. System overview

### 1.1 System description

The **integrating system** is an avionics data distribution middleware layer
used for intra-partition IPC within a DAL C application domain (e.g. health
monitoring, prognostics, cabin management). go-DDS provides the publish/
subscribe transport for this layer.

### 1.2 Software item description

**go-DDS** is a pure-Go Data Distribution Service library implementing the OMG
DDS publish/subscribe model. It provides:

- In-process broker (`mock/`) for intra-partition use
- RTPS/UDP backend (`rtps/`) for inter-partition / inter-LRU use
- QoS enforcement (BestEffort, Reliable, DeadlineQoS, TransientLocal)
- Safety layer (`safety/`) with E2E CRC, HMAC-SHA-256, sequence tracking
- IDL code generator (`idl/`) for data type definitions

**Software level:** DAL C (major failure condition — loss or corruption of
monitored data may contribute to a hazardous condition).

### 1.3 Failure condition classification

The most severe failure condition attributable to go-DDS is **Major**
(DO-178C Table A-1, DAL C), corresponding to:

> "A failure condition which would reduce the capability of the airplane or
> the ability of the flight crew to cope with adverse operating conditions to
> the extent that there would be a significant reduction in safety margins or
> functional capabilities."

This maps to the ADAS sensor-fusion analysis in `HARA.md` (H-01, ASIL-B).

---

## 2. Software lifecycle

go-DDS follows the lifecycle defined in `cert/SDP.md`. The lifecycle phases
and associated DO-178C §11 plans are:

| Phase | Plan |
|---|---|
| Planning | This PSAC, `cert/SDP.md`, `cert/SVP.md`, `cert/SCMP.md`, `cert/SQAP.md` |
| Development | `cert/SDP.md`, `cert/LLR.md`, `CODING_STANDARD.md` |
| Integration | `cert/SVP.md §3.3` |
| Verification | `cert/SVP.md`, `cert/SCR.md` |
| Configuration management | `cert/SCMP.md` |
| Quality assurance | `cert/SQAP.md` |
| Certification liaison | This PSAC + `cert/RELEASE_LOG.md` |

---

## 3. Software development standards

| Standard | Document | Deviation procedure |
|---|---|---|
| Coding standard | `CODING_STANDARD.md` | `cert/DEVIATIONS.md` |
| Requirements standard | `.fusa-reqs.json` format | None — format is fixed |
| Design standard | Package architecture in `CONTRIBUTING.md` | PR review |

---

## 4. Software verification approach

### 4.1 Verification methods (DO-178C Table A-7, DAL C objectives)

| DO-178C objective | Method used | Location |
|---|---|---|
| Source code conforms to standards | `golangci-lint`, `gofusa check` | CI `lint`, `gofusa` jobs |
| Source code traces to LLRs | `//fusa:req` + `gofusa trace` | `cert/LLR.md` |
| Source code accurate and consistent | Static analysis + peer review | CI + PR review |
| Software correctly implemented | Test execution | `cert/SVP.md §3.2` |
| Test procedures correct | Peer review of `*_test.go` | PR review |
| Test results correct and discrepancies explained | CI log review + `cert/RELEASE_LOG.md` | Per release |
| Decision coverage achieved | `go test -covermode=count` | `cert/SCR.md` |

### 4.2 Coverage level

DO-178C §6.4.4.2 Table A-7 requires **decision coverage** for DAL C
(MC/DC is required for DAL B and above).

Decision coverage is measured using `go test -covermode=count`. The Go coverage
tool instruments decision points (if/for/switch/select). See `cert/SCR.md`.

### 4.3 Tool qualification

Tools whose output is used as verification evidence or that could insert errors
without detection are qualified per DO-330. See `cert/TQP-go-compiler.md`,
`cert/TQP-gofusa.md`, `cert/TQP-golangci-lint.md`.

---

## 5. Previously developed software

go-DDS does not incorporate any previously developed software (COTS or legacy)
in its safety-critical partitions. The `cyclone/` package wraps CycloneDDS
(Eclipse Foundation) but is not used in the safety-critical partition and is
excluded from this PSAC.

---

## 6. Option selection

| DO-178C option | Selection | Justification |
|---|---|---|
| Formal methods (§12.3) | Not selected | Recommended, not required at DAL C |
| Previously developed SW (§12.1) | Not applicable | No COTS in safety partition |
| Tool qualification (§12.2) | Selected | Go compiler, gofusa, golangci-lint qualified per DO-330 |
| Alternative methods (§12.4) | Not selected | Standard methods sufficient |

---

## 7. Certification liaison

The applicant (integrator) is responsible for:

1. Providing this PSAC and supporting data to the DER for review
2. Addressing DER comments in a revised PSAC before software baseline freeze
3. Submitting the Software Accomplishment Summary (SAS, not included in this
   document — produced by integrator) referencing go-DDS version and this PSAC
4. Ensuring go-DDS is used per the Assumptions of Use in `HARA.md §5`

go-DDS provides the following data package for DER review:

| Item | File |
|---|---|
| PSAC | `cert/PSAC.md` (this document) |
| SDP | `cert/SDP.md` |
| SVP | `cert/SVP.md` |
| SCMP | `cert/SCMP.md` |
| SQAP | `cert/SQAP.md` |
| LLRs | `cert/LLR.md` |
| Coverage report | `cert/SCR.md` |
| Tool qualification | `cert/TQP-*.md` |
| Problem reporting | `cert/PRP.md` |
| Release log | `cert/RELEASE_LOG.md` |
| HARA | `HARA.md` |
| Safety case | `safety-case.md` |
| SBOM | `sbom.json` |
| Provenance | `provenance.json` |

---

## 8. Open issues

| ID | Issue | Resolution plan |
|---|---|---|
| ~~OI-01~~ | ~~IV&V independence is peer-review level, not full independent V&V~~ | **CLOSED** — independence provided structurally by CI (GitHub-hosted runners); no human IV&V required at DAL C for this SEOOC (see `SAFETY_PLAN.md §3.1`) |
| OI-02 | Go runtime GC is non-deterministic | Justified by measurement in `GC_LATENCY.md`; GOMEMLIMIT AoU required |
| OI-03 | golangci-lint TQP is based on established-use justification | Acceptable for DAL C; DO-330 criteria 2 applies |

---

## 9. Revision history

| Version | Date | Author | Changes |
|---|---|---|---|
| 1.0 | 2026-06-09 | Matt Jones | Initial release — submitted for DER review |
