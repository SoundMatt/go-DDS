# Changelog

All notable changes to go-DDS are documented in this file.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).
go-DDS is a multi-module repository (ROADMAP.md, "Architecture Initiative —
Multi-Module Repository Split", #71): the root module tags as `vX.Y.Z`, and
each submodule (`bridge`, `tools`, `observability`, `safety`, `examples`,
`k8s/operator`) tags independently as `<module>/vX.Y.Z`. Entries below note
which module(s) a change applies to.

This file starts tracking from the Architecture Initiative's Phase F
("Root cleanup") release onward. For the full release history before that
point, see ROADMAP.md's per-milestone "Released" sections and the
[GitHub Releases](https://github.com/SoundMatt/go-DDS/releases) page — both
predate this file and are not being backfilled here.

Versioning: go-DDS is pre-`v1.0`, so per Go's own module conventions,
breaking changes may occur between `v0.x` minor releases. Each submodule's
`v0.x` carries the same caveat independently of the others.

## [Unreleased]

### Added (`observability`)

- Prometheus metrics endpoint (ROADMAP.md, Milestone 15 "Cloud-Native
  Runtime", "Prometheus Metrics"): `GET /metrics` on the monitor's existing
  HTTP server, in Prometheus text exposition format. Gauges
  (`dds_active_topics`, `dds_matched_readers`, `dds_matched_writers`,
  `dds_participant_count`), counters
  (`dds_samples_{published,received,dropped}_total`,
  `dds_bytes_{out,in}_total`, `dds_cdr_{encode,decode}_errors_total`), and
  histograms (`dds_latency_seconds`, `dds_queue_depth`). New
  `Monitor.WithPrometheus(addr)` starts a dedicated second server for the
  same endpoint on its own address; new `Monitor.SetMatched`,
  `IncCDREncodeError`, `IncCDRDecodeError`, `ObserveLatency`, and
  `ObserveQueueDepth` feed the metrics with no existing provider interface
  behind them yet.

### Added (`k8s/operator`, new module)

- go-DDS Kubernetes operator (ROADMAP.md, Milestone 15 "Cloud-Native
  Runtime", "Kubernetes Operator"): a new independent Go module,
  `github.com/SoundMatt/go-DDS/k8s/operator`, with two CRDs
  (`dds.soundmatt.io/v1alpha1`) — `DDSParticipant` (declarative participant
  config: domain, QoS profile, transport, static peers) and `DDSDomain`
  (domain-per-namespace isolation) — plus a mutating admission webhook that
  injects `DDS_DOMAIN_ID`/`DDS_QOS_PROFILE`/`DDS_TRANSPORT`/
  `DDS_TRANSPORT_PORT`/`DDS_PEERS` env vars into pods annotated
  `dds.soundmatt.io/participant: <name>`, a controller that renders
  `DDSDomain` objects with `spec.isolateNamespace: true` into a
  `NetworkPolicy` scoping that domain's RTPS UDP ports to the namespace,
  and a Helm chart (`deploy/helm/go-dds-operator/`) with self-signed
  webhook TLS by default. A fourth Docker image, `operator`
  (`ghcr.io/soundmatt/go-dds-operator`), joins `monitor`/`pub`/`sub` in
  `docker/Dockerfile` and `.github/workflows/docker-publish.yml`.

### Added (root, `bridge`)

- NAT Traversal / Cloud Gateway (ROADMAP.md, Milestone 15 "Cloud-Native
  Runtime", "NAT Traversal / Cloud Gateway"): a TURN-style relay server,
  `bridge/relay.Serve`, forwards opaque length-prefixed frames between
  participants that register under a stable ID — the case RTPS-over-TCP/
  DTLS (Milestone 14) can't cover, since both of those still require one
  side to accept an inbound connection. `bridge/relay.Discover` adds a real
  RFC 5389 STUN Binding Request/Response client for server-reflexive
  address discovery. On the root module side, new participant options
  `rtps.WithRelayAddr`, `WithRelayTLSConfig`, `WithRelayPeers`, and
  `WithSTUNServer` (plus the new `dds.PublicAddresser` optional interface)
  make the relay transport transparent to application code: once two
  participants discover each other, every unicast SEDP/DATA/ACKNACK/
  HEARTBEAT send that needs it is automatically routed over the relay,
  including bypassing the UDP-multicast fast path for a relay-only matched
  reader. The relay never parses or decrypts DDS payload — only a 1-byte
  frame type and a short ID field — so end-to-end payload confidentiality
  via `WithSecurity` is unaffected by the relay hop. The root module
  implements the relay client side of the wire protocol independently
  (no new dependency on the `bridge` submodule), the same precedent
  `rtps/transport_tcp.go` and `bridge/wan` already set.

### Added (root, new `cfilter` package)

- Content-Filtered Topics (ROADMAP.md, Milestone 15 "Cloud-Native Runtime",
  "Content-Filtered Topics" — completes Milestone 15): `dds.NewFilteredSubscriber(p, topic, expr, params, qos, opts...)`
  creates a server-side content-filtered subscription using a small
  DDS-SQL-like predicate (`x > 42 AND status = 'active'`, with `%0`, `%1`,
  ... parameter placeholders), implemented by a new dependency-free
  `cfilter` package (`cfilter.Parse`/`cfilter.Expr`) shared identically by
  `mock`, `rtps`, and `shmem` via the new optional
  `dds.ContentFilteredSubscriberFactory` interface. Unlike the existing
  `dds.WithFilter` (a `func(Sample) bool` checked only after a sample has
  already been delivered or received), the rtps backend propagates the
  compiled predicate to every matched remote writer over two new SEDP
  vendor PL_CDR parameters and evaluates it *before* transmitting DATA, so
  a non-matching sample never crosses the network — the actual
  network-load reduction this sub-phase targets; RTPS multicast is
  disabled in favour of per-reader unicast whenever a matched reader has a
  content filter registered, mirroring the existing relay-only-reader
  fallback. `topic` may itself be an MQTT-style `+`/`#` wildcard pattern,
  composing directly with Milestone 11's wildcard subscriptions.

### Added (root, `bridge`, new `js/dds-client` package)

- WebSocket Transport (ROADMAP.md, Milestone 16 "QUIC + WebSocket
  Transports", "WebSocket Transport"): `rtps/transport_ws.go` adds a
  `wsSocket` — the WebSocket analogue of `tcpSocket`/`quicSocket` —
  implementing the RFC 6455 opening handshake and frame codec with nothing
  but the standard library. New participant options `rtps.WithWSAddr`,
  `WithWSTLSConfig` (TLS/`wss://` is optional, unlike QUIC/DTLS), and
  `WithWSPeers` mirror the existing `WithTCPAddr`/`WithTCPTLSConfig`/
  `WithTCPPeers` convention; a new `pidWSLocator` SPDP parameter
  (`LocatorKindWSv4`) lets peers learn each other's WS listen address. Each
  RTPS message maps to one WebSocket message, sent as either a binary
  frame (raw bytes, the default) or a JSON text frame
  (`{"data":"<base64 CDR>"}`, `WithWSFraming`) — inbound decoding always
  follows the received frame's own opcode regardless of the receiver's own
  setting, so the two modes interoperate freely. `bridge/ws` adds a second,
  optional gateway (mirroring `bridge/rest`'s HTTP/SSE gateway) exposing a
  small JSON subscribe/unsubscribe/publish protocol over one WebSocket
  connection per client, for clients that would rather not implement RTPS
  discovery; its token auth accepts a `?token=` query parameter as well as
  the standard header, since browser JavaScript cannot set arbitrary
  headers on a WebSocket handshake. New `js/dds-client` npm package is the
  TypeScript/JavaScript client for that gateway: a dependency-free
  `DDSClient` (automatic reconnection with resubscription) plus
  `TypedPublisher<T>`/`TypedSubscriber<T>` with a pluggable `Codec<T>`
  (JSON by default). `js/dds-client`'s README documents the scope boundary
  against `rtps.WithWSAddr`: it speaks `bridge/ws`'s gateway protocol, not
  raw RTPS — a genuine no-bridge browser RTPS participant needs a
  Wasm-compiled go-DDS build dialling `WithWSAddr` directly, the sibling
  "WebAssembly Target" sub-phase.

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
