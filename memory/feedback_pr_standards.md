---
name: feedback_pr_standards
description: Every PR must update docs, have maximum test coverage including fuzzing, implement features fully, and pass CI green.
metadata:
  type: feedback
---

Every PR must satisfy all four of the following before merging:

1. **Docs updated** — README quickstart, ROADMAP.md milestone status, and any other affected docs must reflect the change.
2. **Maximum test coverage** — unit tests for all new code paths, integration tests where appropriate, and fuzz targets for any new parsers, codecs, or wire-format logic.
3. **Full implementation** — no partial or stub implementations; every feature must be complete end-to-end.
4. **CI green** — all checks (tests, lint, fuzz, DCO, interop) must pass before merging.

**Why:** Matt stated this explicitly as session rules on 2026-06-08.

**How to apply:** Before opening or merging any PR, verify all four criteria. When implementing a feature, proactively add fuzz targets alongside unit tests. When touching any feature, check whether README/ROADMAP need updating.
