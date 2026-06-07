# go-DDS — Roadmap & Release Notes

Each item below covers what was built, which files are involved, and enough
context for a new session to pick up the work without re-deriving history.

---

## Released

### v0.1.0 — Foundation

**Go interface (`Participant`, `Publisher`, `Subscriber`, `QoS`)** ✅

The root `dds` package (`dds.go`) defines the entire public surface as Go
interfaces. `Participant` is the factory; `Publisher.Write` is fire-and-forget
(bytes handed to the transport, no ACK wait); `Subscriber.C()` exposes a
buffered channel of `dds.Sample`. `QoS` bundles `ReliabilityKind` and
`DurabilityKind`; `DefaultQoS` is BestEffort+Volatile and `ReliableQoS` is
Reliable+TransientLocal. The interface is intentionally narrow — sized for
vehicle-signal (VISS/VISSR) traffic. All implementation packages (`mock`,
`rtps`, `cyclone`) export a `New(dds.Domain, ...Option) (dds.Participant,
error)` function that satisfies this interface, so callers only import the
`dds` package and swap the sub-package import at the call site.

---

**In-process mock — 100% statement coverage** ✅

`mock/mock.go` implements `dds.Participant` over a process-global `broker`
struct. All mock participants in the same process share that broker regardless
of domain; a `publish` call delivers the sample synchronously to every
matching subscriber channel. The broker also stores `lastSample` per topic
(a plain `map[string]*dds.Sample` protected by the broker's `sync.RWMutex`)
for TransientLocal delivery. The fuzz suite lives in `mock/mock_test.go` and
exercises `FuzzPublish`, `FuzzPublishIsolation`, `FuzzTopicName`,
`FuzzNoRouting`, `FuzzConcurrentPubSub`, and `FuzzDropOnFullChannel`. A known
design constraint: the global broker is **never reset between tests** — each
test must use a unique topic name to avoid cross-test interference.

---

**CycloneDDS CGo implementation (`-tags cyclone`)** ✅

`cyclone/cyclone.go` wraps the CycloneDDS C library behind `//go:build
cyclone`. When the build tag is absent, `cyclone/stub.go` replaces both `New`
functions with stubs that return `errors.New("cyclone build tag required")`.
`cyclone/stub_test.go` (also `//go:build !cyclone`) confirms the stub errors.
The CGo path is intentionally thin: it delegates all DDS work to the C library
and converts between C DDS types and `dds.Sample`. This package is used for
real multi-host interop when `libcyclonedds-dev` is installed; on CI, the
`test-cyclone` job uses a probe-and-flag pattern to skip cleanly when the
library is absent.

---

**Configurable poll interval (`cyclone.Options`)** ✅

The CycloneDDS CGo wrapper polls for new samples rather than using C callbacks
(C callbacks from cgo goroutines need careful stack sizing). The poll interval
is exposed as a field on `cyclone.Options`; `NewWithOptions` accepts it.
`cyclone/stub_test.go` exercises the stub path so there is meaningful coverage
even on platforms where the CGo build is unavailable.

---

**Pure-Go RTPS/UDP — no CGo, all platforms** ✅

`rtps/` is the main implementation package: `rtps.go` (package doc + version
constants), `participant.go` (the `Participant` implementation and all socket
management), `transport.go` (UDP socket helpers: unicast, multicast, IPv4,
IPv6), `locator.go` (the RTPS 24-byte `Locator_t`, port formula), `guid.go`
(12-byte `GuidPrefix`, `EntityId`, `GUID`), `message.go` (submessage parser /
builder, header, CDR wrapping), `cdr.go` (CDR-LE payload unwrap), `spdp.go`
(SPDP participant discovery), `sedp.go` (SEDP endpoint discovery), and
`reliable.go` (HEARTBEAT / ACKNACK reliability layer). The participant binds
three UDP sockets: one multicast-receive (SPDP), one unicast meta (SPDP send
+ SEDP), one unicast data (payload + HEARTBEAT + ACKNACK). Port numbers follow
the RTPS 2.3 §9.6.1 formula exactly. The implementation targets RTPS 2.3 but
only implements the subset needed for unicast user-data delivery.

---

**Reliable QoS retransmission (HEARTBEAT / ACKNACK)** ✅

`rtps/reliable.go` contains the sender-side `sendHistory` (a `map[uint32][]byte`
capped at `maxHistoryDepth = 256`, evicting the oldest entry) and
receiver-side `recvTracker` (tracks the next expected sequence number and
builds 32-bit NACK bitmaps). The write path in `participant.go` stores each
reliable sample in `sendHistory`, sends a HEARTBEAT immediately after the
write, and starts a `heartbeatLoop` goroutine that reticks every
`heartbeatPeriod = 200ms`. `handleAckNack` (also in `participant.go`) reads
the bitmap and retransmits. A subtle but important past bug: `heartbeatLoop`
must receive `hbDone` as a value argument (not read `w.hbDone` after startup)
because `Close` nils the field under `w.mu` — passing by value avoids the
race. That fix is in commit `735de08`.

---

**WaitSet — sub-millisecond multi-topic blocking receive** ✅

`WaitSet` lives in `dds.go` (root package, not a sub-package). It builds a
`reflect.SelectCase` slice from the subscribers' channels plus `ctx.Done()`
and calls `reflect.Select` in a loop. When a channel is closed it is replaced
with a `SelectDefault` case so the loop doesn't spin; when all subscriber
channels are closed it returns with `ctx.Err()`. The reflect-based select is
the only portable way to multiplex a runtime-variable set of channels in Go.
`waitset_test.go` covers the all-channels-closed path, context cancel,
one-closed/one-pending, and normal sample delivery. The same `WaitSet` type is
exercised in `rtps/rtps_test.go` and `mock/mock_test.go` for backend coverage.

---

**DDS-Security plugin interface** ✅

`security/security.go` defines `Plugin` (two methods: `Seal`, `Open`) and
provides three built-in implementations: `NullPlugin` (identity; use for dev
and non-secured peers), `HMACPlugin` (HMAC-SHA-256 appended as a 32-byte tag;
integrity + auth, no confidentiality), and `AESGCMPlugin` (AES-256-GCM with a
fresh 12-byte nonce per call; full confidentiality + integrity + auth, 28
bytes of overhead per sample). Security is applied at the payload level in
`rtps/participant.go` — `Write` calls `Seal` before CDR wrapping and the
receive path calls `Open` after CDR unwrapping. All participants communicating
on a topic must share the same plugin and key. The `WithSecurity(plugin)` RTPS
option wires the plugin into the participant.

---

### v0.2.0 — Durability, IPv6, Coverage, Interop

**TransientLocal durability (last-value cache for late joiners)** ✅

Both `mock` and `rtps` implement this. In the mock broker, `lastSample
map[string]*dds.Sample` is updated on every `publish` when `qos.Durability ==
TransientLocal`, and new subscribers receive it before their channel is
returned from `subscribe`. In the RTPS participant, `lastSample` is a
`sync.Map` (not `sync.RWMutex` + plain map) to avoid lock-ordering issues:
`Write` holds `w.mu` and calls `dispatchToReaders` which acquires `p.mu`;
using `sync.Map` means `Write` never needs `p.mu` at all. `NewSubscriber`
loads from `sync.Map` under `p.mu` (safe — `sync.Map.Load` is always
concurrent-safe) and non-blockingly pushes the cached sample into the new
reader's channel. The `TestRTPS_TransientLocal` and
`TestTransientLocal_LateJoiner` tests in `rtps/rtps_test.go` and
`mock/mock_test.go` cover the main path, NoSample guard, and Volatile-not-
delivered invariant.

---

**IPv6 multicast transport (`WithIPv6()`, `LocatorKindUDPv6`)** ✅

`rtps/transport.go` adds `newUnicastSocketV6` (binds `udp6`) and
`newMulticastReceiveSocketV6` (joins `FF03::1`; falls back to unicast if no
IPv6 multicast interface exists). `rtps/locator.go` adds `LocatorKindUDPv6 =
2` and updates `locatorFromUDP` to auto-detect IPv6 (checks `IP.To4() == nil`)
and `udpAddr()` to handle both kinds. `participant.go` adds three optional
socket fields (`mcastSockV6`, `metaSockV6`, `dataSockV6`) set by `WithIPv6()`;
`dataReceiveLoop` multiplexes IPv4 and IPv6 receive channels with a
`select` when both are present. Socket creation failures are soft — if the OS
has no IPv6 support the participant continues with IPv4 only.
`TestRTPS_WithIPv6_StartsCleanly` verifies the participant starts without
error (the test is skipped if IPv6 is truly unavailable on the host).

---

**RTPS interop testing with CycloneDDS (Docker Compose + CycloneDDS peer)** ✅

`interop/` is a separate Go package (`package interop`) with a `doc.go` and
`interop_test.go` both gated behind `//go:build interop`. The three tests are:
`TestInterop_GoPublisher_CycloneSubscriber` (go-DDS writes, CycloneDDS reads),
`TestInterop_CyclonePublisher_GoSubscriber` (CycloneDDS writes, go-DDS reads),
and `TestInterop_BidirectionalEcho` (round-trip through the peer container).
`interop/docker-compose.yml` defines three services: `cyclone-peer` (always-on
echo relay), `cyclone-sub` (profile `sub`), `cyclone-pub` (profile `pub`), all
with `network_mode: host` so UDP multicast works. The `.github/workflows/ci.yml`
`test-interop` job probes `docker pull eclipse-cyclonedds/cyclonedds:latest`
and skips cleanly (green, not red) when the image is unavailable. Environment
variables `INTEROP_DOMAIN` and `INTEROP_TIMEOUT` configure the domain and
per-test deadline.

---

## Known Protocol Bugs

These are correctness defects in the current implementation — not missing
features. They affect wire behaviour today and should be fixed before the items
in the Planned sections below.

---

**`matchedReaderLocators` ignores topic (`participant.go:497`)**

`rtpsWriter.Write` calls `p.matchedReaderLocators(w.topic)` to get the UDP
addresses to send data packets to, but the function body contains `_ =
topicName` and returns the `defaultUnicast` locator of *every* known SPDP
peer regardless of whether they have a subscriber for that topic. With a single
topic this is harmless; with multiple topics and multiple remote participants
every write is sent to every peer, wasting bandwidth and delivering unsolicited
packets. The fix is for SEDP to maintain a `topicReaders map[string][]GUID`
index (updated in `registerReader` / `onNewPeer` in `sedp.go`) and for
`matchedReaderLocators` to look up only the participants that have an active
reader for the requested topic. The `_ = topicName` blank assignment is the
marker to search for when starting this fix.

---

**SPDP lease duration advertised but never enforced**

`spdp.go:buildParticipantData` writes `pidParticipantLeaseDuration = 10
seconds` into every SPDP announcement, but nothing in the codebase ever checks
whether a peer's lease has expired. A remote participant that crashes or loses
network connectivity stays in the `spdpService.peers` map and in
`matchedReaderLocators` output forever. Subsequent reliable writes will
keep retransmitting to the dead address until `sendHistory` overflows.
The fix is a background goroutine in `spdp.go` that scans `peers` once per
second and evicts entries whose last-seen timestamp is older than their
advertised lease duration (store `lastSeen time.Time` alongside each
`participantProxy`). Eviction must also notify SEDP so local reader
source-lists can be pruned. This is a prerequisite for the participant
liveliness callback item in the Operational section below.

---

**RTPS GAP submessage not sent**

When a reliable writer's `sendHistory` evicts old samples (the ring is capped
at `maxHistoryDepth = 256`), a reader that was offline and returns later will
send ACKNACKs requesting those evicted sequence numbers. The writer has no way
to respond other than to ignore the request, leaving the reader stalled. The
RTPS 2.3 §8.3.7.4 GAP submessage tells a reader "sequence numbers X–Y are
permanently unavailable; treat them as received." In `participant.go:handleAckNack`,
after iterating the NACK bitmap, any sequence numbers in the bitmap that are
older than `sendHistory.firstLast().first` should trigger a GAP response
rather than silence. `marshalGAP` needs to be added to `message.go`
alongside the existing `marshalHeartbeat` / `marshalAckNack`. Until this is
fixed, a reliable subscriber that reconnects after a gap in history will stall
indefinitely if the requested samples have been evicted.

---

## Planned

---

**Typed sentinel errors (`errors.Is` / `errors.As` support)**

Every error returned by `go-DDS` today is a plain `fmt.Errorf` string. Callers
cannot distinguish `ErrClosed` from `ErrTopicNotFound` without fragile string
matching. The fix is to define a small set of sentinel errors in `dds.go`
(e.g. `ErrClosed`, `ErrWriterClosed`, `ErrTopicNotFound`) and ensure all
implementations wrap them so `errors.Is` works. The `mock` and `rtps`
packages would each replace their `fmt.Errorf("mock: participant closed")` /
`fmt.Errorf("rtps: participant closed")` returns with
`fmt.Errorf("mock: %w", dds.ErrClosed)`. No wire-format change; purely a
Go error-handling improvement. The main decision to make first is which
error values live in the root `dds` package (shared contract) versus which
are package-local (e.g. `rtps.ErrNoPort`).

---

**Unicast-only / no-multicast discovery mode**

The current RTPS transport requires a multicast-capable network interface for
SPDP discovery. Containers (Docker default bridge, Kubernetes pods) and many
cloud environments block multicast. The fix is a `WithPeerLocators(addrs
...string)` option that supplies a static list of remote participant addresses
so SPDP announcements can be sent directly (unicast) without relying on the
`239.255.0.1` group. The `newMulticastReceiveSocket` fallback in
`transport.go` already degrades to a unicast bind when multicast fails; the
gap is that without multicast there is no mechanism to reach remote peers at
all. Implementation: store the peer list on `participant`; in `spdp.go`'s
announcement loop, send the SPDP DATA packet to each static address in
addition to (or instead of) the multicast group. `mcastSock` may still be
used for receive even in unicast-only mode so the participant can accept
multicast announcements from peers that do support it.

---

**Content-filtered subscriptions (server-side predicate)**

Rather than delivering every sample to a subscriber's channel and leaving
filtering to the application, a content filter applies a predicate at the
transport layer before the channel write. The API addition would be a
`WithFilter(fn func(dds.Sample) bool)` option on `NewSubscriber`, stored on
`rtpsReader` and `mock.subscriber`. In `dispatchToReaders`
(`participant.go`) the predicate is evaluated before the `select { case r.ch
<- sample: }` push; in the mock broker's `publish` loop similarly. The predicate
runs synchronously in the dispatch goroutine, so it must be fast and
non-blocking. The DDS spec calls this "content-filtered topics" (CFT); the
go-DDS version is simpler — a plain Go function rather than a SQL-like
expression language.

---

**Deadline QoS (missed-deadline callback)**

A Deadline QoS policy triggers a callback when a publisher has not written a
sample within a configured period. This is important for liveness detection in
vehicle-signal pipelines where a sensor going silent should be noticed
immediately. Implementation: add `Deadline time.Duration` to `dds.QoS` (zero
= disabled); `rtpsWriter` and `mock.publisher` start a `time.Timer` reset on
every `Write`; if the timer fires, it calls a user-supplied callback registered
via `WithDeadlineCallback(fn func(topic string))` on the participant. The
callback must be called from its own goroutine (not the write path) to avoid
blocking. Subscriber-side deadline (detecting when a publisher goes silent from
the subscriber's perspective) is a separate policy in the DDS spec but can be
implemented the same way on the reader side.

---

**Large payload fragmentation (RTPS DATA_FRAG submessage)**

UDP has an effective payload limit of ~64 KB; typical MTU is 1500 bytes, making
fragmentation necessary for payloads larger than ~1400 bytes if IP
fragmentation is to be avoided. RTPS 2.3 §8.4.14 defines the `DATA_FRAG`
submessage: the writer splits the payload into fragments, each wrapped in a
separate `DATA_FRAG` submessage with fragment number and total-fragment-count.
The receiver reassembles. In go-DDS, `marshalDataSubmessage` in `message.go`
would grow a `marshalDataFragSubmessages` variant; `handleDataPacket` in
`participant.go` would accumulate fragments in a per-writer reassembly buffer
(keyed by `writerGUID + seqNum`) and only dispatch once all fragments arrive.
The reassembly buffer needs a TTL to drop incomplete sequences. The reliable
retransmission layer in `reliable.go` must also fragment-retransmit on NACK,
which is the hardest part.

---

**Topic wildcards (`sensors/#`, `vehicle/*/speed`)**

Currently topic matching in both `mock` and `rtps` is exact string equality.
MQTT-style wildcards (`+` for one level, `#` for the remainder) would let a
single subscriber receive from many related topics. In the mock broker,
`subscribe` would store the pattern and `publish` would iterate all subscriber
patterns checking for a match. In RTPS, SEDP endpoint matching
(`registerReader`/`onRemoteWriter` in `sedp.go`) compares topic names; wildcard
matching at the SEDP level means a subscriber could match remote writers it
has not explicitly named. The main design question is whether wildcard matching
is done at the SEDP level (affects discovery) or only at the local dispatch
level (`dispatchToReaders`). Starting with local-only dispatch is simpler and
covers the majority of use cases. The mock can serve as the reference
implementation before porting to RTPS.

---

**Metrics / statistics API**

There is currently no way to observe drop counts (the `default` branch in
every `select { case r.ch <- sample: default: }` dispatch is silent),
publish counts, or delivery latency. The addition would be a
`dds.Metrics` struct returned by a `Participant.Metrics()` method, populated
by atomic counters. Each `rtpsWriter` and `rtpsReader` would hold
`atomic.Uint64` fields for `WriteCount`, `DropCount`, and `BytesIn`/
`BytesOut`. The mock broker would do the same. Latency histograms require
timestamping each sample at write and measuring at delivery; a simple
`[16]uint64` power-of-two bucket histogram (µs granularity) is enough for
diagnostic purposes. Surfacing this as a Prometheus-compatible
`/metrics` HTTP endpoint is a natural follow-on but should be kept in a
separate `metrics/` sub-package to avoid pulling in HTTP server dependencies
into the core library.

---

**RTPS persistent history (disk-backed TransientLocal)**

The current TransientLocal cache is in-memory only — a process restart
loses all cached samples. A persistent variant would write each sample to an
append-only log file (one file per topic) so a restarted participant can
replay the last N samples to late joiners. The RTPS spec calls this
`TRANSIENT` or `PERSISTENT` durability. Implementation sketch: a
`WithPersistentHistory(dir string)` RTPS option; on `Write`, serialize the
sample to a per-topic log file under `dir`; on `newParticipant`, replay the
last sample (or last N, controlled by `QoS.HistoryDepth`) from each log file
into `p.lastSample`; on `NewSubscriber` with TransientLocal, the replayed
entry is delivered just like the in-memory cache today. The log format can be
a simple length-prefixed binary file (4-byte little-endian length + payload
bytes). The mock backend would also benefit from this for integration tests
but it is lower priority there since the mock is primarily a testing vehicle.

---

## Planned — Operational

---

**Configurable subscriber channel depth and back-pressure policy**

Both `mock/mock.go:39` and `rtps/participant.go:213` create subscriber channels
with a hardcoded depth of 64 samples. When a slow consumer fills the buffer the
sample is silently dropped (the `default` branch in the dispatch `select`).
The fix is a `WithChannelDepth(n int)` `NewSubscriber` option and a
`WithBackPressure(policy BackPressurePolicy)` option where `policy` is either
`DropNewest` (current behaviour), `DropOldest` (evict the head of the channel
to make room), or `Block` (block the publisher goroutine until the subscriber
drains — only safe when the publisher controls its own goroutine). The option
is stored on `rtpsReader` and `mock.subscriber`; the dispatch path reads it
before the `select`. The metrics item below depends on this change because drop
counts are only useful if the drop policy is observable and configurable.

---

**Structured logging (`slog.Logger` injection)**

The library is completely silent. Dropped packets, SPDP failures, SEDP errors,
and ACKNACK timeouts leave no trace in production. A `WithLogger(l *slog.Logger)`
participant option (available on both `mock.New` and `rtps.New`) would route
internal log lines through the standard-library `log/slog` interface introduced
in Go 1.21. Log sites to add at minimum: SPDP peer discovered/evicted, SEDP
endpoint matched, packet dropped (with reason), HEARTBEAT sent/received,
ACKNACK sent/received, GAP sent, security seal/open errors, socket send errors.
All log calls must be guarded by `l != nil` so the zero-value behaviour (no
logging) is preserved. The logger should be stored on `participant` and
threaded through to `spdpService` and `sedpService` at construction time.
Because `slog` is in the standard library since Go 1.21, this adds no new
module dependencies.

---

**Participant liveliness detection and callback**

Related to the SPDP lease expiry bug above. Once lease expiry is enforced, the
application needs a way to react to it. A `WithLivelinessCallback(fn
func(guid dds.GUID, event LivelinessEvent))` participant option registers a
function called when a remote participant is first discovered
(`LivelinessGained`) or when its lease expires (`LivelinessLost`). The DDS
spec calls this the LIVELINESS QoS policy; go-DDS can implement the
participant-level subset without the full per-DataWriter granularity initially.
`dds.GUID` should be added to the root package as an opaque `[16]byte` type so
the callback signature does not leak the internal `rtps.GUID` type through the
public API. This is the application-facing complement to the SPDP lease expiry
fix and the structured logging item — both are prerequisites.

---

**Graceful shutdown with reliable-ACK drain**

`participant.Close()` immediately closes sockets. Any reliable `Write` calls
that have not yet been acknowledged (samples still in `sendHistory`) are
abandoned without notifying the application. For command/control topics this
is data loss. A `CloseWithDrain(ctx context.Context) error` method on
`dds.Participant` would block until: (a) all outstanding reliable ACKs have
been received or (b) the context is cancelled. Implementation: track a
`pendingAcks atomic.Int64` counter on each `rtpsWriter` (incremented on each
reliable write, decremented when an ACKNACK confirms the sample), add a
`drained chan struct{}` that is closed when the counter reaches zero, and have
`CloseWithDrain` wait on all writer drain channels before closing sockets.
The mock implementation can provide an equivalent no-op (all mock writes are
synchronous) so the interface is uniform across backends.

---

## Planned — Transport

---

**Multicast data delivery**

All user-data packets are currently sent unicast to each matched peer
individually (one UDP write per remote participant). For a topic with N remote
subscribers this means N copies of every payload on the wire. RTPS 2.3
supports topic-specific multicast groups for the data plane: the writer sends
one packet to a multicast address and the NIC/switch replicates it. The SEDP
endpoint announcement (`sedp.go:registerWriter`) already builds a
`pidMulticastLocator` parameter; the receiver side (`sedp.go:onRemoteWriter`)
already parses it but it is not used in routing. The fix is: assign each RTPS
writer a multicast group address from the 239.255.x.x range (derived from
domain + topic hash), advertise it in the SEDP publication data, and have
remote subscribers join that group when they match the writer in SEDP. Writers
can then call `dataSock.send(multicastAddr, msg)` instead of iterating
`matchedReaderLocators`. Note: this requires the `matchedReaderLocators` topic-
filtering bug to be fixed first so that unicast fallback is also correct.

---

**Shared memory transport (`shmem/` sub-package)**

For same-host inter-process pub/sub, UDP loopback adds a kernel round-trip and
two copies (write buffer → kernel → read buffer) per sample. A shared memory
transport eliminates both: writer maps a ring buffer into both processes, writes
the payload once, and signals the reader via a futex or a `sync.Mutex`+channel
combination via a named pipe. The transport would live in `shmem/` and
implement `dds.Participant` using `golang.org/x/sys/unix` for `mmap` and
`syscall.Flock` for the initial handshake. Discovery would use a well-known
file path under `/tmp/godds/<domain>/` rather than UDP multicast. The mock
backend is already effectively shared-memory for in-process use; the gap this
fills is cross-process same-host communication without UDP overhead.
Platform support: Linux and macOS. Windows would fall back to named pipes
(`\\.\pipe\godds-<domain>-<topic>`).

---

**INFO_TS submessage (source timestamps)**

The constant `submsgINFO_TS = 0x09` is defined in `message.go` but never
parsed or emitted. An INFO_TS submessage prepended to a DATA submessage carries
the writer's clock at the time of the write; the receiver uses it as
`Sample.SourceTimestamp` rather than the arrival time. This matters for
time-series topics (sensor data with late delivery should carry the original
measurement time), for TSN latency accounting (end-to-end latency =
receive_time − source_timestamp), and for TransientLocal disambiguation (if
two samples arrive out of order, the one with the earlier INFO_TS was written
first). Implementation: add a `Timestamp time.Time` field to `dds.Sample`;
add `marshalInfoTS(t time.Time) []byte` to `message.go`; prepend it before
each `marshalDataSubmessage` call in `rtpsWriter.Write`; parse it in
`handleDataPacket` and pass it through `dispatchToReaders`. The RTPS timestamp
format is two uint32s: seconds since epoch (NTP epoch, i.e. 1 Jan 1900) and
32-bit fraction. Use `CLOCK_TAI` on Linux when the TSN integration is active.

---

## Planned — Integration

---

**MQTT bridge (`bridge/mqtt/`)**

An `mqtt.Bridge` that connects a `dds.Participant` to an MQTT broker,
bidirectionally mapping DDS topics to MQTT topics. DDS → MQTT: subscribe to a
list of DDS topics and forward each sample as an MQTT `PUBLISH` to the
corresponding MQTT topic path. MQTT → DDS: subscribe to MQTT topics and publish
each received message as a DDS sample. The bridge lives in `bridge/mqtt/` and
uses `github.com/eclipse/paho.mqtt.golang` (or `paho.golang` for v5). QoS
mapping: DDS Reliable → MQTT QoS 1 (at-least-once); DDS BestEffort → MQTT QoS
0 (fire-and-forget). Topic name mapping should be configurable (e.g. strip or
add a prefix) so DDS topics like `vehicle/speed` map cleanly to MQTT paths.
The bridge is a natural integration point for VISS gateways, which are
MQTT-native; however the VISS-specific topic schema and signal tree binding
belong in the VISSR package, not here.

---

**IDL / protobuf schema binding and `go generate`**

The library is currently schema-free: `Publisher.Write` accepts raw `[]byte`
and `Sample.Payload` is `[]byte`. The CI `generate` job already runs `go
generate ./...` but no source file contains a `//go:generate` directive, so
it is a no-op. Two complementary code-gen paths are worth adding:
(1) Protobuf: a `//go:generate protoc --go_out=.` directive in a `schema/`
sub-package that produces typed marshal/unmarshal functions and thin
`TypedPublisher[T proto.Message]` / `TypedSubscriber[T proto.Message]` generic
wrappers defined in `dds.go`; (2) a lightweight standalone IDL tool (`tools/
ddstypes/`) that reads a minimal `.idl` file (field name + primitive type only,
no unions or inheritance) and emits a Go struct plus `Marshal() []byte` and
`Unmarshal([]byte) error` methods. The generics wrappers should be the stable
API; the IDL tool is optional sugar. Go 1.21 generics are sufficient for
`TypedPublisher[T]`; no new language features are required.

---

**OpenTelemetry tracing**

The planned Metrics API covers aggregate counters. OpenTelemetry spans cover
per-operation latency and distributed causality — a single DDS write can be
traced through the transport layer and into the receiving participant, linking
producer and consumer spans with a propagated trace context. A `WithOTelTracer(
t trace.Tracer)` participant option (from `go.opentelemetry.io/otel`) would
create a span in `rtpsWriter.Write` (named `dds.write`, with topic and
sequence number as attributes) and a matching span in `dispatchToReaders`
(named `dds.deliver`). Propagating the trace context across the wire requires
embedding a W3C `traceparent` header in the CDR payload or in a custom RTPS
vendor-specific parameter list entry; the former is simpler but adds bytes to
every sample. The OTel SDK is a compile-time dependency only when the option
is used; importing `go.opentelemetry.io/otel` with a no-op tracer at the
module level adds ~200 KB to the binary but zero runtime cost.

---

## TSN Support

Time-Sensitive Networking (TSN) is a set of IEEE 802.1 standards that extend
Ethernet with bounded latency, low jitter, and zero-congestion-loss guarantees.
In automotive and industrial control applications go-DDS will eventually need to
act as a TSN Talker/Listener, mapping DDS DataWriter/DataReader QoS onto TSN
stream parameters and handing scheduled frames to the Linux network stack. The
items below follow the OMG DDS-TSN specification model. They are listed in
implementation dependency order: QoS extensions first (no transport changes),
then transport-layer features (socket options, traffic classes), then
integration points (gPTP, external config). The Go code can be fully TSN-aware;
the actual end-to-end latency guarantee comes from the NIC, switch, and tc/etf
configuration outside Go.

---

**TSN-extended QoS fields**

The current `dds.QoS` struct (`dds.go`) only carries `Reliability`,
`Durability`, and `HistoryDepth`. TSN requires additional fields that the
OMG DDS-TSN spec maps to stream parameters: `TransportPriority int` (maps to
VLAN PCP and socket priority), `LatencyBudget time.Duration` (maximum tolerated
end-to-end latency; becomes the TSN stream's `maxLatency`), `Lifespan
time.Duration` (drop samples older than this; avoids delivering stale control
data after network recovery), `PublishPeriod time.Duration` (nominal write
interval; drives the TSN Talker's `interval` and `maxFramesPerInterval`
calculation), and `MaxSampleSize int` (maximum serialised payload bytes; TSN
scheduling requires bounded frame sizes so the scheduler can guarantee
bandwidth). These fields must be added to `dds.QoS` with zero values that
mean "not specified / use default." The mock and RTPS backends must both respect
`Lifespan` (drop on delivery if timestamp+lifespan < now) and surface the others
to the TSN transport layer. Adding these fields is the prerequisite for every
subsequent TSN item.

---

**DDS-to-TSN stream model**

For each TSN-capable DataWriter, go-DDS must produce a `tsn.Stream` descriptor
that a TSN switch or YANG management agent can consume. The struct lives in a
new `tsn/` sub-package and contains: `StreamID` (48-bit source MAC + 16-bit
unique ID, per IEEE 802.1CB), `VLANID uint16`, `PCP uint8` (802.1Q priority
code point 0–7), `DSCP uint8` (IP differentiated services code point),
`MaxFrameSize int` (bytes, including Ethernet header), `Interval
time.Duration` (from `QoS.PublishPeriod`), `MaxFramesPerInterval int`,
`MaxLatency time.Duration` (from `QoS.LatencyBudget`), and destination
address (multicast MAC for broadcast TSN streams). The RTPS participant would
expose a `StreamsFor(topic string) []tsn.Stream` method so an external agent
can extract the stream config and program the switch. This is purely a data
model initially; it does not require any change to the on-wire RTPS format.

---

**VLAN, PCP, and DSCP socket marking**

On Linux, per-packet priority is set via socket options that the kernel maps
into the outgoing Ethernet frame. The RTPS transport (`transport.go`) must
gain support for: `SO_PRIORITY` (Linux socket priority 0–7, maps to VLAN PCP
via the NIC queue discipline), `IP_TOS` / `IPV6_TCLASS` (sets DSCP in the IP
header for routed paths), and optionally binding the data socket to a VLAN
interface (`eth0.100`) rather than the physical interface so the kernel adds
the 802.1Q tag automatically. These options are Linux-specific and must be
gated behind a `//go:build linux` file in `rtps/transport_linux.go`. The
portable fallback (`transport_others.go`) is a no-op. A new RTPS option
`WithTSNMarking(priority int, dscp int, vlanID int)` sets these fields on the
data socket after `net.ListenUDP`. The socket must be a raw `*net.UDPConn` (not
wrapped) to allow `syscall.SetsockoptInt` calls.

---

**Scheduled transmit time (SO_TXTIME + ETF/taprio)**

Hard TSN requires frames to leave the NIC at a deterministic wall-clock time,
not merely "as soon as possible." On Linux this is achieved by setting
`SO_TXTIME` on the socket and stamping each outgoing packet with a `CLOCK_TAI`
nanosecond timestamp in a `cmsg` ancillary data block. The kernel's ETF
(Earliest TxTime First) or taprio qdisc holds the packet until the scheduled
time and then releases it to the NIC. In go-DDS, `rtpsWriter.Write` would
compute `txTime = now + txTimeOffset` (where `txTimeOffset` is a per-writer
configuration, typically 250–500 µs to give the stack time to schedule the
frame) and call `sendmsg` with the `SCM_TXTIME` cmsg. This requires a Linux-
specific socket path using `syscall.Sendmsg` rather than `conn.Write`. The
`tsn/` package should provide a `TxTimeSocket` wrapper that hides the cmsg
plumbing. This feature is only meaningful when the NIC driver supports
`SO_TXTIME` (most Intel i210/i225 and most i.MX automotive SoC Ethernet
controllers do).

---

**gPTP / IEEE 802.1AS time base integration**

TSN scheduling depends on all nodes sharing a common time reference, normally
provided by IEEE 802.1AS (gPTP). On Linux, the PTP Hardware Clock (PHC) is
synced to gPTP by `ptp4l`, and `phc2sys` then syncs `CLOCK_TAI` to the PHC.
go-DDS should use `CLOCK_TAI` (via `clock_gettime(CLOCK_TAI, ...)` exposed as
`unix.ClockGettime(unix.CLOCK_TAI, &ts)` from `golang.org/x/sys/unix`) for all
TSN timestamp calculations rather than `time.Now()` (which uses `CLOCK_REALTIME`
and may jump on NTP corrections). A `tsn.Clock` interface with a single
`Now() time.Time` method should wrap this so the clock source is swappable in
tests. The `tsn/` package should also expose a `TAIOffset() time.Duration`
helper that reads `/sys/class/ptp/ptp0/tai_offset` (or uses `adjtimex`) so
callers can convert between TAI and UTC if needed. This integration point is
Linux-only; the non-Linux fallback is `time.Now()`.

---

**Separate traffic-class sockets**

Currently all RTPS traffic — SPDP discovery, SEDP endpoint discovery, user
data, HEARTBEAT, and ACKNACK — shares the same data socket (`dataSock`).
For TSN this is wrong: discovery traffic is bursty and unpredictable, while
a scheduled TSN stream requires a private socket with its own priority and
qdisc mapping. The fix is to introduce per-flow sockets: a low-priority
socket for SPDP/SEDP discovery (can share the existing `metaSock` socket or
get its own), a medium-priority socket for best-effort user data, a
configurable-priority socket per TSN writer (created by `WithTSNMarking`),
and the existing `dataSock` as the default fallback. `rtpsWriter.Write` would
choose the socket based on `qos.TransportPriority`. Heartbeats and ACKNACKs
for a reliable TSN writer should use the same high-priority socket so they do
not queue behind best-effort traffic and delay retransmission.

---

**TSN-safe discovery**

SPDP sends a participant announcement every 2 seconds by default; a burst of
new participants joining simultaneously could congest a TSN network where
bandwidth is pre-allocated. The fixes are: a configurable `SPDPInterval`
option (currently hardcoded in `spdp.go`), a `WithStaticPeers` option that
disables multicast SPDP entirely and relies on the peer list from
`WithPeerLocators` (see unicast-only item), a jitter budget on the SPDP
ticker so all participants do not announce simultaneously after a power cycle,
and a rate limit on SEDP `onNewPeer` fan-out so a single new participant does
not trigger a flood of SEDP DATA packets. Discovery traffic should always be
routed through the low-priority socket described above, keeping it off the
scheduled queues reserved for time-critical DataWriters.

---

**Fragmentation bounds for TSN streams**

TSN schedules assume a bounded maximum frame size. If a DataWriter's
`QoS.MaxSampleSize` is set and a sample exceeds it, the writer should reject
the write with a sentinel error (see `ErrSampleTooLarge`, linked to the typed
errors item) rather than silently fragmenting at the IP layer, because IP
fragmentation is unbounded and defeats the TSN schedule. Optionally, the RTPS
DATA_FRAG path (see fragmentation item) can be used to split samples into
MTU-sized chunks deterministically, but each fragment must itself be within the
scheduled stream's frame-size budget. The simplest safe rule for a TSN DataWriter
configured with a frame-size limit is: refuse writes that exceed `MaxSampleSize`
minus the RTPS + CDR + Ethernet overhead (approximately 80 bytes). Document the
exact formula in a constant in `rtps/transport.go` so it is easy to update as
header sizes change.

---

**External TSN configuration (YAML/JSON stream config)**

Hard-coding TSN policy in Go option calls is adequate for a single application
but inconvenient for deployments where a network engineer must tune TSN
parameters without recompiling. A `tsn.LoadConfig(path string)` function should
parse a YAML or JSON file into a `[]tsn.StreamConfig` slice that can be applied
to an RTPS participant via `rtps.WithTSNConfig(cfg)`. The config format mirrors
the `tsn.Stream` struct: `topic`, `vlan_id`, `pcp`, `dscp`, `period`,
`max_latency`, `max_frame_size`, `max_frames_per_interval`, and
`txtime_offset`. The `tsn/` package should own both the config parser (using
`encoding/json` or `gopkg.in/yaml.v3`) and the struct definitions. This is the
last TSN item because it depends on all the underlying transport features
existing before there is anything useful to configure externally.

---

**Real-time web monitor**

A lightweight diagnostic UI that shows live DDS activity — topic traffic rates,
drop counts, participant health, and sample payloads — without requiring an
external observability stack. The monitor is a `monitor/` sub-package that
accepts a `dds.Participant` (wrapping it in an instrumented shim) and starts an
`net/http` server using only the Go standard library. The frontend is a single
HTML page embedded via `embed.FS` so there are no static file deployment
concerns. Live updates stream over `text/event-stream` (Server-Sent Events),
which every modern browser supports natively; no WebSocket library is needed.
Each SSE event carries a JSON object with topic name, timestamp, sample count,
byte count, drop count, and — optionally — the last payload in base64. The
`monitor.New(p dds.Participant, addr string) *Monitor` constructor wraps each
`NewPublisher` / `NewSubscriber` call to intercept writes and deliveries and
update `atomic.Uint64` counters. A `ServeHTTP` method makes the monitor
composable with any `net/http` mux. This item depends on the Metrics / statistics
API item (the counters are shared) but can be started independently using its
own inline counters. The goal is zero production overhead when the monitor is
not started — the shim only activates when `monitor.New` is called.

---

**Architecture boundary note**

go-DDS can classify, mark, and schedule DDS traffic at the socket level, but
end-to-end TSN guarantees require co-operation from hardware and kernel outside
Go's control. The full stack is:

```
dds.QoS (TransportPriority, LatencyBudget, PublishPeriod)
  ↓
tsn.StreamConfig (VLAN ID, PCP, DSCP, interval, max frame size)
  ↓
rtps.WithTSNMarking + per-flow socket (SO_PRIORITY, IP_TOS)
  ↓
SO_TXTIME + CLOCK_TAI timestamp on outgoing packets
  ↓
Linux tc: mqprio + taprio (credit-based shaper) or etf (per-stream)
  ↓
TSN-capable NIC (Intel i210/i225, NXP ENET, Renesas AVB)
  ↓
TSN-capable Ethernet switch (802.1Qbv gate tables, 802.1Qav CBS)
  ↓
gPTP (IEEE 802.1AS) time synchronisation across all nodes
```

A session starting this work should first confirm that the test environment
has a TSN-capable NIC (check `ethtool -T ethX` for `SOF_TIMESTAMPING_TX_HARDWARE`
and `SO_TXTIME` in kernel ≥ 4.19), and verify gPTP is running (`ptp4l -i ethX
-f /etc/ptp4l.conf` and `phc2sys`) before writing any transmit-time code.
