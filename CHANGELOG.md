# Changelog

All notable changes to go-DDS are documented in this file.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).
go-DDS is a multi-module repository (ROADMAP.md, "Architecture Initiative —
Multi-Module Repository Split", #71): the root module tags as `vX.Y.Z`, and
each submodule (`bridge`, `tools`, `observability`, `safety`, `examples`)
tags independently as `<module>/vX.Y.Z`. Entries below note which module(s)
a change applies to.

This file starts tracking from the Architecture Initiative's Phase F
("Root cleanup") release onward. For the full release history before that
point, see ROADMAP.md's per-milestone "Released" sections and the
[GitHub Releases](https://github.com/SoundMatt/go-DDS/releases) page — both
predate this file and are not being backfilled here.

Versioning: go-DDS is pre-`v1.0`, so per Go's own module conventions,
breaking changes may occur between `v0.x` minor releases. Each submodule's
`v0.x` carries the same caveat independently of the others.

## [Unreleased]

## [root v0.55.1] / [safety v0.1.1] — Architecture Initiative Phase F — Root cleanup

Docs/repo-hygiene reorganization only — no Go import path, API, or `go.mod`
boundary changes (ROADMAP.md, "Architecture Initiative", Phase F is
explicitly independent of the module-split mechanics proper).

### Changed

- Moved `HARA.md`, `SEOOC.md`, `STANDARDS_GAP.md`, `CODING_STANDARD.md`,
  and `GC_LATENCY.md` from the repo root into `docs/`. Updated the
  references to them in `README.md`, `ROADMAP.md`, `SAFETY_PLAN.md`,
  `SQAP.md`, and `safety/gc_latency_test.go` (the latter now writes its
  generated report to `docs/GC_LATENCY.md`) — hence the `safety` submodule
  patch bump alongside root.
- `SAFETY_PLAN.md`, `SVP.md`, `SCMP.md`, `SQAP.md`, `sas.md`, and the
  `safety-case.*` files deliberately stayed at the repo root — see
  ROADMAP.md's Phase F entry for why (go-FuSa v0.30.0 checks these at
  hardcoded root-relative paths with no configurable override, and several
  are auto-regenerated to root on every release tag by
  `.github/workflows/release.yml`; moving them would silently corrupt the
  ISO 26262 / IEC 61508 / DO-178C gap-report evidence those checks produce).
  `SECURITY.md` and `INCIDENT-RESPONSE.md` also stayed at root: they are
  GitHub community-health-file conventions, and `INCIDENT-RESPONSE.md` in
  particular is checked at a hardcoded root path by go-FuSa's DO-178C SCI
  builder (`sci.go`) with no `docs/` fallback, unlike the more flexible
  IEC 62443 check for the same file.

### Added

- `docs/MATURITY.md` — per-package maturity matrix (Stable / Beta /
  Experimental / Reference / Deprecated) covering every package across all
  six modules, per #71's secondary ask.
- `CHANGELOG.md` (this file).
- `SUPPORT.md` — where to get help, report bugs, and (for safety-relevant
  issues) how that differs from the standard bug-report path.
