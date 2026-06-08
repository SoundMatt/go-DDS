# Safety Case: github.com/SoundMatt/go-DDS

Generated: 2026-06-08T19:37:14Z  
Standard: iso26262

## Top Claim

**G1:** The software `github.com/SoundMatt/go-DDS` is acceptably safe for use in `iso26262` context,
argued by demonstrating compliance with the safety development lifecycle.

## Evidence Summary

| ID | Description | Status | Detail |
|---|---|---|---|
| Sn1 | Coding standard and static analysis checks | ✅ present |  |
| Sn2 | Requirements traceability matrix | ✅ present | 15 requirements |
| Sn3 | Test evidence bundle | ✅ present | 786/829 tests passed |
| Sn4 | Tool qualification report | ✅ present | 44/44 cases passed |
| Sn5 | SBOM (SPDX 3.0.1) | ✅ present |  |
| Sn6 | Build provenance | ✅ present |  |

## Compliance Mapping

| Standard | Clause | Title | Evidence |
|---|---|---|---|
| ISO 26262 | Part 6 §6.4 | Software requirements specification | trace |
| ISO 26262 | Part 6 §6.6–6.7 | Software unit design and implementation | check |
| ISO 26262 | Part 6 §6.8 | Software unit verification | verify |
| ISO 26262 | Part 8 §8.4.5 | Tool qualification | qualify |
| ISO 26262 | Part 8 §8.3 | Release management | sbom, provenance |

## Gaps

None — all evidence present.
