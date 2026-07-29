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

Nothing yet.

## [v0.64.0] - 2026-07-28

### Fixed (root)

- Cross-topic sample delivery: a self-looped multicast DATA packet (a
  writer's own transmission, received back on the multicast socket it
  shares with every other participant on the segment — normal once that
  writer has at least one remote-matched reader) was reprocessed by
  `participant.handleDataPacket` with topic filtering disabled
  (`dispatchToReaders`'s `topicFilter == ""` path, which defers entirely to
  `rtpsReader.acceptsSource`). `acceptsSource`'s own-participant-prefix
  bypass — needed for `rtpsWriter.Write`'s in-process delivery call, where
  the topic has already been filtered — accepts a same-prefix source
  unconditionally with no topic check, so the self-looped copy fanned out
  to *every* reader on that participant regardless of topic, not just ones
  actually matched to the writer that sent it. Concretely: `ros2`'s
  internal `ros_discovery_info` graph writer could deliver a
  `ParticipantEntitiesInfo` sample into an unrelated application
  subscriber's channel (surfaced by an intermittent CI failure in
  `ros2.TestTwoNodes_SeeEachOtherInGraph`, reproduced reliably under CPU
  load in a Linux container after the fixed test-timing flake in #128 was
  ruled out). `handleDataPacket` now drops any packet whose header
  GuidPrefix matches the participant's own, mirroring the identical
  self-origin guard SPDP (`spdpService.handlePacket`), SEDP
  (`sedpService.handlePacket`), and the TCP transport
  (`dispatchTCPPacket`) already had — this was the one DATA/HEARTBEAT/
  ACKNACK receive path missing it. General fix: affects any
  `rtps.Participant` (not just `ros2`) with two or more topics and at
  least one writer whose packets reach the shared multicast group. (#129)
- `participant.mu` / `sedpService.mu` lock-order inversion:
  `sedpService.onRemoteWriter` held `sedp.mu` across a call into
  `participant.readerByEID` (which needs `participant.mu`), while
  `newPublisherLocked`/`newSubscriberLocked` held `participant.mu` across a
  call into `sedpService.registerWriter`/`registerReader` (which needs
  `sedp.mu`) — the reverse order, a real deadlock reproduced reliably under
  sustained participant creation concurrent with incoming SEDP endpoint
  announcements under heavy load. Fixed with a consistent lock order and
  covered by a new regression test that proves the deadlock is gone.
  (#131, closes #130)
- `ros2` graph-visibility test relied on a fixed sleep instead of polling,
  flaky under CI load; also fixed an unrelated docker-compose profile leak
  in the ROS2 interop CI job. (#128)

### Added (root, `tools`)

- ROS 2 / rmw Compatibility (ROADMAP.md, Milestone 17 "Robotics
  Integration", first sub-phase): new `ros2` package —
  `NewROS2Participant` wraps an `rtps.Participant` with ROS 2's topic/type
  naming (`ToDDSTopicName`/`TypeSupportName`, matching the "rt"-prefixed,
  `"pkg::msg::dds_::Type_"` conventions rmw_fastrtps/rmw_cyclonedds use)
  and its `rmw_dds_common` "ros_discovery_info" graph protocol
  (`Participant.Nodes()`/`Topics()`), so a go-DDS process shows up in a
  real ROS 2 graph without a bridge process. New `dds.TypedEndpointFactory`
  (`NewPublisherWithType`/`NewSubscriberWithType`, implemented by `rtps`)
  lets a caller announce a real DDS type name over SEDP instead of the
  default `"CDR_BLOB"` sentinel; new `rtps.EndpointDiscoveryProvider`/
  `rtps.GUIDProvider` expose the discovered-endpoint table and per-entity
  GUIDs the `ros2` package needs. New `ddstool ros2-list` CLI subcommand
  lists live ROS 2 nodes/topics visible from go-DDS. New
  `interop/ros2_test.go` (`-tags interop`) and a `test-interop-ros2` CI job
  prove it against live `ros:jazzy`/`ros:rolling` `demo_nodes_cpp talker`
  containers, alongside the existing CycloneDDS interop tests. (#127)

## [v0.63.0] - 2026-07-28

### Added (root, `examples`)

- WebAssembly Target (ROADMAP.md, Milestone 16 "QUIC + WebSocket
  Transports", "WebAssembly Target" — completes Milestone 16): `mock` and
  `rtps/transport_ws.go` build and run under `GOOS=wasip1 GOARCH=wasm` (a
  new `wasm-build` CI job proves it across every module); a new
  `rtps/transport_ws_browser.go` (`GOOS=js GOARCH=wasm`) dials out via
  `syscall/js` against the browser's own `WebSocket` object instead of a
  real `net.Conn`, since the stdlib's js/wasm `net` port never reaches an
  actual remote host. `WithWSPeers` alone (no `WithWSAddr`) now enables the
  RTPS-over-WebSocket transport in a new listener-less ("dial-only") mode
  — the shape a browser tab or serverless edge function needs, since
  neither can ever accept an inbound connection — with replies and further
  traffic routed back over the already-open connection
  (`wsSocket.cachedConnForIP`) and a one-time reply-to-new-peer
  (`participant.wsReplyToNewWSPeer`) closing the one-sided discovery gap a
  listener-less peer would otherwise create. `examples/wasm-subscriber/`
  demonstrates the browser half end to end (a real RTPS participant in a
  browser tab, not `bridge/ws`'s JSON gateway); `docs/WASM_DEPLOYMENT.md` is
  the Fastly/Cloudflare Workers deployment guide for the cloud-function
  half, including a real bug found and fixed while proving `rtps.New`
  starts under Wasmtime with `wasi:sockets` enabled:
  `newMulticastReceiveSocket`/`V6` now fall back to a plain unicast bind,
  rather than failing outright, when no network interface can even be
  enumerated (as under a WASI runtime), not just when enumeration succeeds
  but the multicast join itself fails. (#126)

Milestone 16 ("QUIC + WebSocket Transports", `v1.2`) is now complete.

## [v0.62.0] - 2026-07-28

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
  "WebAssembly Target" sub-phase. (#125)

## [v0.61.0] - 2026-07-28

### Added (root)

- QUIC Transport (ROADMAP.md, Milestone 16 "QUIC + WebSocket Transports"):
  `rtps/transport_quic.go`, built on `github.com/quic-go/quic-go`. Each
  peer connection carries two independent channels rather than one shared
  byte stream — a single reliable, length-prefixed bidirectional stream
  (the same 4-byte framing `transport_tcp.go` uses) for SPDP/SEDP discovery
  and reliable-QoS traffic, and unreliable QUIC DATAGRAM frames (RFC 9221)
  for best-effort user DATA — so a lossy run of best-effort samples can
  never head-of-line-block a concurrent discovery exchange or reliable
  retransmission on the same QUIC session. `rtps.WithQUICAddr(addr)`,
  `WithQUIC(cfg *tls.Config)` (same stdlib `*tls.Config` type
  `WithTCPTLSConfig`/`WithDTLS` accept, ALPN/TLS 1.3/session cache filled
  in automatically), and `WithQUICPeers(addrs...)` mirror the existing TCP
  transport options; 0-RTT reconnection via `quic.DialAddrEarly`/
  `quic.ListenAddrEarly` plus a default client session cache lets a redial
  to a previously-contacted peer resume without a full handshake. New
  `pidQUICLocator` SPDP parameter (`LocatorKindQUICv4`) lets peers learn
  each other's QUIC listen address. `participant.sendUnicast` gained a
  `reliable bool` parameter so best-effort samples ride the datagram path
  while discovery, HEARTBEAT/ACKNACK, and reliable DATA always use the
  stream (falling back to the stream automatically if a best-effort
  message is too large for one datagram). In the transport-priority chain,
  QUIC is checked right after DTLS, falling back to TCP, then relay, then
  plain UDP. Fully additive — with no `WithQUICAddr`, `quicSock` is always
  nil and every send path is byte-for-byte unchanged. FastDDS has no
  published fixed ALPN token or wire-framing spec for QUIC to conform to
  as of this writing (the same kind of gap v0.57.0 documented for DTLS
  1.3); this transport's framing is a defensible reading of "RTPS messages
  over QUIC streams and datagrams," not a byte-for-byte conformance claim.
  Requirements `REQ-TRANS-012/013/014`, traced and tested. (#124)

## [v0.60.0] - 2026-07-28

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
  composing directly with Milestone 11's wildcard subscriptions. (#123)

Milestone 15 ("Cloud-Native Runtime", `v1.1`) is now complete.

## [v0.59.0] - 2026-07-28

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
  `rtps/transport_tcp.go` and `bridge/wan` already set. (#122)

## [root v0.58.2] / [k8s/operator v0.1.0] - 2026-07-28

Docs/CI-only at the root (no root Go code changed).

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
  `docker/Dockerfile` and `.github/workflows/docker-publish.yml`, and a new
  `test-k8s-operator` CI matrix leg. (#121)

## [root v0.58.1] / [observability v0.2.0] - 2026-07-28

Docs-only at the root (no root Go code changed).

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
  behind them yet. (#120)

## [v0.58.0] - 2026-07-28

Completes Milestone 14 ("Transport Completeness", `v1.0`): TCP/TLS
(v0.56.0) and DTLS (v0.57.0) shipped the transports; this release adds the
four QoS policies that were previously only passively modeled (Liveliness,
Partition) or entirely absent (Ownership, Time-Based Filter), implemented
identically in both the `rtps` and `mock` backends.

### Added (root)

- `dds.QoS.Liveliness` / `LivelinessLeaseDuration` — `AutomaticLiveliness`
  (default) drives a writer-owned ticker that asserts on a schedule; in
  `rtps` via a dedicated liveliness-only HEARTBEAT carrying the RTPS "L"
  flag, kept strictly separate from the reliable retransmission tracker.
  `ManualByTopicLiveliness` relies solely on real `Write` traffic, so a
  writer that stops publishing is correctly declared lost.
  `dds.WithLivelinessLost` registers a per-writer callback, invoked with
  `dds.ErrLivelinessLost` semantics when a matched writer's lease expires.
- `dds.QoS.Ownership` / `OwnershipStrength` — `ExclusiveOwnership` writers
  compete per topic by strength (ties broken deterministically by GUID);
  only the active writer's samples reach subscribers. Automatic failover
  on `Close`, SPDP peer eviction, or (in `rtps`) liveliness loss.
- `dds.QoS.Partition` — standard DDS partition-intersection matching; the
  empty set is the default partition `""`, not a wildcard. Enforced for
  same-process pairs and, in `rtps`, propagated over SEDP via new vendor
  `PL_CDR` parameters.
- `dds.QoS.MinSeparation` — the Time-Based Filter QoS: drops samples
  arriving faster than the configured rate, tracked independently per
  (reader, writer) pair.
- `dds.ErrLivelinessLost` sentinel error.

Fully additive: every new `QoS` field's zero value reproduces pre-v0.58
behaviour exactly (`SharedOwnership`, no partition filtering,
`AutomaticLiveliness` with no lease, `MinSeparation` disabled).

### Fixed (root, `bridge`)

- `mock`: `newMockGUID` could produce colliding GUIDs for two entities
  created within the same effective clock tick (observed on Windows CI
  runners), since only the high 8 bytes were ever filled from
  `time.Now().UnixNano()`. Now mixes in a process-wide monotonic counter —
  collision-proof regardless of clock resolution. Latent since the mock
  backend's inception; surfaced by this release's Ownership QoS, which is
  the first feature to key state by GUID identity.
- `bridge/rest`, `bridge/grpc`: `dds.QoS` gained a `[]string` field
  (`Partition`), making it non-comparable with `==`. Switched the "was QoS
  explicitly set" zero-value check to `reflect.DeepEqual`.

(#119)

## [v0.57.0] - 2026-07-28

### Added (root)

- RTPS-over-DTLS transport (Milestone 14, "DTLS (Encrypted UDP)"): adds
  `rtps/transport_dtls.go`, the DTLS analogue of the RTPS-over-TCP
  transport shipped in v0.56.0 — a `dtlsSocket` wrapping the existing UDP
  transport in DTLS-secured datagram sessions, so one RTPS message maps
  1:1 to one DTLS record, no length-prefix framing needed unlike TCP.
  `rtps.WithDTLSAddr(addr)` binds a DTLS 1.2 listener alongside the
  existing UDP transport; `rtps.WithDTLS(cfg *tls.Config)` secures it with
  certificate-based peer authentication (same stdlib `*tls.Config` type as
  `WithTCPTLSConfig`), enforcing mutual auth by default whenever
  `ClientCAs` is set and `ClientAuth` is left unset; `rtps.WithDTLSPeers(addrs...)`
  names peer addresses whose unicast traffic (SEDP/DATA/HEARTBEAT/ACKNACK)
  must be encrypted over DTLS instead of plain UDP. New
  `security.CertPlugin.TLSCertificate()`/`CertPlugin.CAPool()` export a
  `CertPlugin`'s certificate identity as stdlib `tls.Certificate`/
  `*x509.CertPool`, so the same identity configures both the TCP/TLS and
  DTLS transports. Requirements `REQ-TRANS-007/008/009` and `REQ-SEC-026`,
  traced and tested. Fully additive — with no `WithDTLSAddr`, behaviour is
  byte-for-byte unchanged. Satisfies OMG DDS Security spec §9.5 "Secure
  Transport". (#118)

Go's standard library `crypto/tls` has no DTLS support at all, and DTLS 1.3
(RFC 9147) has no production-ready implementation anywhere in the Go
ecosystem yet — including `github.com/pion/dtls`, the most mature Go DTLS
library, whose v3 README still lists DTLS 1.3 under "Planned Features".
This transport therefore ships DTLS 1.2 (RFC 6347) via
`github.com/pion/dtls/v3`, the one exception to go-DDS's "stdlib only"
transport policy, and will move to DTLS 1.3 once a stable implementation
ships.

## [v0.56.0] - 2026-07-28

### Added (root)

- RTPS-over-TCP transport with TLS 1.3 (Milestone 14, "Transport
  Completeness", TCP/TLS sub-phase): `rtps/transport_tcp.go` adds
  length-prefixed TCP framing of the same RTPS message bytes the UDP
  transport already produces, TLS 1.3 wrapping via `crypto/tls`, automatic
  UDP→TCP unicast fallback when UDP multicast is unavailable, and
  SPDP/SEDP discovery over TCP via a new `pidTCPLocator` vendor parameter.
  `WithTCPAddr(addr)`, `WithTCPTLSConfig(cfg)`, and `WithTCPPeers(addrs...)`
  participant options join the existing UDP options; `sendUnicast` (now
  the shared unicast-send path for SEDP, ACKNACK/HEARTBEAT, and writer
  DATA) prefers a peer's known TCP locator over UDP whenever no
  multicast-capable interface was detected at startup, falling back to
  UDP only if the TCP send itself fails. Fully additive: with no
  `WithTCPAddr`, every send path is byte-for-byte the pre-Milestone-14
  UDP-only behaviour. (#117)

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
