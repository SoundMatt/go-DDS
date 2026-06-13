# Incident Response Plan

**Project:** go-DDS  
**Standard:** IEC 62443-4-2 CR 6.2.1  
**Owner:** Matt Jones <matt@jellybaby.com>

## 1. Scope

This plan covers security incidents affecting the go-DDS library and its downstream
integrations (automotive, aerospace, medical, industrial, robotics deployments).

## 2. Incident Categories

| Severity | Description                                          | Response SLA |
|----------|------------------------------------------------------|--------------|
| Critical | Remote code execution, authentication bypass, data   | 24 hours     |
|          | corruption in safety-critical path                   |              |
| High     | Privilege escalation, confidentiality breach,        | 72 hours     |
|          | denial of service in transport layer                 |              |
| Medium   | Local information disclosure, non-critical DoS       | 14 days      |
| Low      | Configuration weaknesses, documentation gaps         | 90 days      |

## 3. Detection

Incidents may be detected via:
- Private vulnerability reports to `matt@jellybaby.com`
- GitHub Dependabot / OSV alerts (`gofusa vuln` in CI)
- User-reported operational anomalies
- Internal code review or `gofusa cyber` findings

## 4. Response Procedure

### 4.1 Triage (within SLA above)
1. Acknowledge receipt to the reporter.
2. Reproduce the issue in an isolated environment.
3. Assign a severity level using the table above.
4. Create a private tracking entry in `.fusa-problems.json` (`gofusa pr add`).

### 4.2 Containment
1. Determine whether a workaround can be documented immediately.
2. For Critical/High: prepare a hotfix branch (`fix/security-<cve-or-short>`).
3. Notify downstream integrators under embargo if impact is systemic.

### 4.3 Remediation
1. Implement fix; add regression test tracing to the relevant requirement.
2. Run full CI matrix (`go test -race ./...`, `golangci-lint`, `gofusa check ./...`).
3. Regenerate safety artifacts: `gofusa fmea -cyber && gofusa safety-case`.

### 4.4 Disclosure
1. Merge fix to `main`; tag a patch release.
2. Publish a GitHub Security Advisory with CVE if applicable.
3. Credit the reporter (unless they opt out).
4. Close the problem report entry (`gofusa pr close <id>`).

## 5. Contact

Security-sensitive communications: **matt@jellybaby.com**  
Public issue tracker: https://github.com/SoundMatt/go-DDS/issues (non-security only)
