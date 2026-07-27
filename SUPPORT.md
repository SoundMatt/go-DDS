# Support

## Getting help

- **Documentation:** start with [README.md](README.md) for usage and the
  package table, [docs/MATURITY.md](docs/MATURITY.md) for which packages are
  Stable vs. Beta vs. Experimental, and [ROADMAP.md](ROADMAP.md) for planned
  work and design rationale.
- **Questions and general discussion:** open a
  [GitHub issue](https://github.com/SoundMatt/go-DDS/issues) with the
  `question` label.
- **Bug reports:** open a
  [GitHub issue](https://github.com/SoundMatt/go-DDS/issues/new) describing
  the go-DDS version (or per-module version — see CHANGELOG.md), the
  transport/implementation in use (`mock`, `rtps`, `cyclone`, `shmem`), and
  a minimal reproduction if possible.
- **Feature requests:** open a GitHub issue; check ROADMAP.md first in case
  it is already planned.

## Security vulnerabilities

**Do not report security vulnerabilities as public GitHub issues.** See
[SECURITY.md](SECURITY.md) for the private disclosure process.

## Safety-relevant issues

go-DDS targets deployment in safety-critical environments (ASIL-B / SIL 2 /
DO-178C — see `SAFETY_PLAN.md`, `docs/SEOOC.md`, `docs/HARA.md`). If you
believe you have found a defect that could affect a safety claim in those
documents (not just a functional bug), say so explicitly in the issue title
or body so it gets triaged against the relevant hazard/requirement, in
addition to the normal bug-report path above.

## Supported versions

See [SECURITY.md](SECURITY.md)'s "Supported Versions" table. Because this
is a multi-module repository (`bridge`, `tools`, `observability`, `safety`,
`examples` each tag independently — see ROADMAP.md, "Architecture
Initiative"), support questions should mention which module(s) and version(s)
are involved.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) if you'd like to submit a fix or
feature yourself.
