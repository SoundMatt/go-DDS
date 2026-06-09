# Problem Reporting Procedure (PRP)
## go-DDS

**Document ID:** PRP-001  
**Version:** 1.0  
**Date:** 2026-06-09  
**Standard:** DO-178C §11.20, IEC 61508-3 §6.2  

---

## 1. Purpose

This procedure defines how software problems are reported, classified,
investigated, resolved, and closed for go-DDS.

---

## 2. Problem Report lifecycle

```
OPEN → ASSIGNED → INVESTIGATING → FIX_READY → VERIFIED → CLOSED
                                                         ↓
                                              (or) DEFERRED → CLOSED
```

| State | Description | Responsible |
|---|---|---|
| OPEN | Problem reported; not yet reviewed | Reporter |
| ASSIGNED | Assigned to engineer for investigation | Maintainer |
| INVESTIGATING | Root cause analysis in progress | Assignee |
| FIX_READY | Fix implemented; awaiting verification | Assignee |
| VERIFIED | Fix verified by independent reviewer | Verifier |
| CLOSED | Fix merged to `main` and tag created | Maintainer |
| DEFERRED | Accepted as known limitation; tracked for future release | Maintainer |

---

## 3. Problem classification

| Severity | Definition | Max closure time |
|---|---|---|
| S1 — Critical | Safety goal violated; potential for hazardous event | 48 hours |
| S2 — High | Incorrect behaviour in safety-critical path; no immediate hazard | 2 weeks |
| S3 — Medium | Incorrect behaviour in non-safety path | Next minor release |
| S4 — Low | Documentation error; cosmetic issue | Next patch release |

Safety impact assessment is mandatory for S1 and S2. The assessment must
identify which requirements in `.fusa-reqs.json` are affected and whether
the safety case (`safety-case.md`) needs updating.

---

## 4. Reporting channels

| Channel | Purpose |
|---|---|
| GitHub Issues | All problem reports; primary tracking system |
| Label `safety` | Applied to all S1/S2 issues affecting safety goals |
| Label `regression` | Applied when a fix introduces a new test case |
| PR linked to issue | Each fix PR must reference the issue with `Fixes #NNN` |

---

## 5. Safety impact assessment template

For any issue labelled `safety`, the assignee completes the following in the
issue body before work begins:

```
## Safety Impact Assessment

**Affected requirements:** REQ-NNN, REQ-MMM (list from .fusa-reqs.json)
**ASIL/SIL level affected:** ASIL-B / SIL 2 / QM
**Safety case update required?:** Yes / No
**HARA update required?:** Yes / No
**Regression test:** Will be added at: [file:function]
**Verifier assigned:** [name]
```

---

## 6. Closure criteria

A problem report may be closed only when:

1. Root cause is documented in the issue
2. Fix is merged to `main` (or deferral is accepted with justification)
3. Regression test is present (for S1/S2)
4. `gofusa check` and full test suite pass
5. For S1/S2: verifier has commented "LGTM — verified" on the fix PR

---

## 7. Release hold criteria

A release shall not be created if any of the following is true:

- Any open S1 issue exists
- Any open S2 issue older than 2 weeks exists without approved deferral
- Any issue labelled `safety` is in state OPEN/ASSIGNED for more than 3 business days

---

## 8. Revision history

| Version | Date | Author | Changes |
|---|---|---|---|
| 1.0 | 2026-06-09 | Matt Jones | Initial release |
