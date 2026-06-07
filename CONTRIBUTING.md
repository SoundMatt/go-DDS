# Contributing to go-DDS

Thank you for your interest in contributing.

## Developer Certificate of Origin (DCO)

All contributions must be signed off under the
[Developer Certificate of Origin v1.1](https://developercertificate.org).
The DCO is a lightweight way to certify that you wrote or have the right to
submit the code you are contributing.

Add a `Signed-off-by` trailer to every commit:

```
git commit -s -m "feat: add awesome thing"
```

This produces:

```
feat: add awesome thing

Signed-off-by: Your Name <your@email.com>
```

If you forget to sign off, amend the commit:

```
git commit --amend -s
```

A GitHub Actions check (`DCO`) verifies every commit in a PR. PRs without
sign-offs will not be merged.

## Copyright

By contributing you agree that your contributions are licensed under the
[Mozilla Public License v2.0](LICENSE) and that copyright in go-DDS remains
with Matt Jones. The DCO sign-off transfers no copyright — it only affirms you
have the right to contribute under the existing license.

## Coding style

- `gofmt` — run `gofmt -w ./...` before pushing.
- `go vet` — must pass with zero warnings.
- `golangci-lint run` — must pass (config in `.golangci.yml`).
- Tests — new code should be accompanied by tests covering the public API.
  Run `go test -race -count=1 ./...` locally.

## Pull requests

1. Fork the repo, create a branch from `main`.
2. Make your changes with signed-off commits.
3. `go test -race -count=1 ./...` must pass.
4. Open a PR targeting `main`.

## Project structure

| Directory | What it contains |
|---|---|
| `.` | `dds` package — interfaces (`Participant`, `Publisher`, `Subscriber`, `QoS`, `WaitSet`) |
| `mock/` | In-process pub/sub. Zero dependencies. Use for unit tests. |
| `rtps/` | Pure-Go RTPS/UDP transport. No CGo. |
| `cyclone/` | CycloneDDS via CGo (`-tags cyclone`). |
| `security/` | Pluggable payload security (NullPlugin, HMAC-SHA-256, AES-256-GCM). |
| `interop/` | Wire-compatibility tests against CycloneDDS. Requires Docker and `-tags interop`. |

## Running tests

```bash
# Standard suite (all platforms, no special dependencies)
go test -race -count=1 ./...

# rtps intra-process tests (skips the 2.2 s two-participant test)
go test -race -count=1 -short ./rtps/...

# CycloneDDS CGo tests (requires libcyclonedds-dev + rebuild)
go build -tags cyclone ./...
go test -race -count=1 -tags cyclone ./cyclone/...

# RTPS interop tests (requires Docker)
docker compose -f interop/docker-compose.yml up -d cyclone-peer
go test -tags interop -v -timeout 60s ./interop/...
docker compose -f interop/docker-compose.yml down

# Fuzz targets (run as long as you like)
go test -fuzz=FuzzPublish -fuzztime=60s ./mock/...
```

## Package contracts

### mock

All mock participants in the same process share a single global broker. The
broker is never reset between tests — use topic names that are unique per test
to avoid cross-test interference.

### rtps

Participants bind UDP sockets on creation. Tests use domain `99` to avoid
collisions with any real DDS deployment on domain `0`. The two-participant
test (`TestRTPS_TwoParticipants_SameHost`) requires working multicast loopback
and is Linux-only; it is skipped with `-short` in CI.

### security

The `SecurityPlugin` interface (`Seal` / `Open`) is applied at the payload
level, not the transport level. All participants sharing a topic must use the
same plugin and key.

## Adding a new transport backend

1. Create a new sub-package (e.g. `zenoh/`).
2. Implement `dds.Participant` → `dds.Publisher` + `dds.Subscriber`.
3. Export a `New(domain dds.Domain) (dds.Participant, error)` constructor.
4. Add tests with `go test -race -count=1 ./zenoh/...`.
5. Add a row to the implementation table in `README.md`.
