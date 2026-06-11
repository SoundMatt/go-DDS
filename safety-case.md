# Safety Case: github.com/SoundMatt/go-DDS

Generated: 2026-06-11T15:18:40Z  
Standard: generic

## Top Claim

**G1:** The software `github.com/SoundMatt/go-DDS` is acceptably safe for use in `generic` context,
argued by demonstrating compliance with the safety development lifecycle.

## Evidence Summary

| ID | Description | Status | Detail |
|---|---|---|---|
| Sn1 | Coding standard and static analysis checks | ⚠ absent | run 'gofusa check --output check-report.json' to generate |
| Sn2 | Requirements traceability matrix | ✅ present | 244 requirements |
| Sn3 | Test evidence bundle | ✅ present | 832/875 tests passed |
| Sn4 | Tool qualification report | ✅ present | 44/44 cases passed |
| Sn5 | SBOM (SPDX 3.0.1) | ✅ present |  |
| Sn6 | Build provenance | ✅ present |  |

## Compliance Mapping

| Standard | Clause | Title | Evidence |
|---|---|---|---|
| Generic | CS-1 | Coding standard compliance | check |
| Generic | CS-2 | Requirements traceability | trace |
| Generic | CS-3 | Test evidence | verify |
| Generic | CS-4 | Tool qualification | qualify |
| Generic | CS-5 | Release inventory | sbom, provenance |

## Gaps

The following evidence items are absent:

- `check`
