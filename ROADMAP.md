# go-DDS — Roadmap & Release Notes

Each released item documents what was built, which files are involved, and enough
context for a new session to pick up the work without re-deriving history.
Future phases capture the project's long-term direction; each item carries a
short paragraph explaining the motivation and the expected implementation path.

---

## Vision

go-DDS is intended to become:

> "A lightweight, production-grade DDS/RTPS implementation written in Go
> for embedded, automotive, robotics, industrial, edge, and cloud-connected
> systems, with strong interoperability, observability, security,
> safety-oriented E2E communication, TSN awareness, and native integration
> with modern software-defined vehicle data models."

The project remains Go-first, standards-driven, vendor-neutral, and modular — 
suitable for embedded Linux through cloud deployments, compatible with enterprise
DDS ecosystems, and capable of supporting safety-oriented and TSN-enabled deployments.

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
the bitmap and retransmits. Note: `heartbeatLoop` receives `hbDone` by value
at startup so `Close` can nil the field under `w.mu` without racing with the
goroutine.

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

### v0.2.1 — RTPS Protocol Completeness

**SEDP topic-filtered routing** ✅

SEDP now maintains `remoteReaders map[GUID]*endpointInfo` and
`remoteReaderLocs map[GUID]Locator` (in `sedp.go`). `matchedReaderLocators`
filters by `topicName` and deduplicates by `Locator` struct value so each
DATA packet is sent exactly once per matched remote participant.

---

**SPDP lease expiry enforcement** ✅

`spdpService` stores `leaseDuration` and `lastSeen` on each `participantProxy`
and runs an `evictLoop` goroutine that calls `evictExpired` once per second.
Expired peers are removed from SPDP and SEDP via `onPeerEvicted`.
`pidParticipantLeaseDuration` is now parsed from the SPDP announcement payload.

---

**RTPS GAP submessage** ✅

`message.go` gained a `Gap` struct and `marshalGAP`. `handleAckNack` sends a
GAP when the NACK base falls below the first retained sequence number, allowing
reliable subscribers to advance past permanently evicted samples.

---

### v0.3.0 — Core Feature Set

**Typed sentinel errors (`errors.Is` / `errors.As` support)** ✅

`dds.go` defines `ErrClosed` and `ErrTopicEmpty`. All implementations (`mock`,
`rtps`) wrap them with `fmt.Errorf("...: %w", dds.ErrClosed)` so callers can
use `errors.Is`. `SubscriberConfig` and `ApplySubscriberOpts` are exported so
sub-packages can apply options without duplicating the merge logic. `cyclone`
continues returning its own descriptive errors (CGo layer) but the stub now
accepts the updated `NewSubscriber` signature.

---

**Unicast-only / no-multicast discovery** ✅

`rtps.WithNoMulticast()` stores a flag suppressing multicast discovery.
`rtps.WithPeerLocators(addrs ...string)` stores a static peer list for
directed SPDP unicast. Both options are stored on `participant` and inspected
by the SPDP layer. The SPDP loop can use them to send announcements to static
addresses when the multicast group is unavailable (container / cloud environments).

---

**Content-filtered subscriptions** ✅

`dds.WithFilter(fn func(Sample) bool)` returns a `dds.SubscriberOption`. Both
`mock` (in `broker.publish`) and `rtps` (in `dispatchToReaders`) apply the
predicate before pushing to the subscriber channel. Non-matching samples are
silently discarded. TransientLocal re-delivery also respects the filter.

---

**Deadline QoS** ✅

`dds.QoS.Deadline time.Duration` (zero = disabled). `rtps.WithDeadlineCallback`
and `mock.WithDeadlineCallback` register a `func(topic string)` called when
a publisher has not written within the deadline period. `rtpsWriter` and
`mock.publisher` start a `time.AfterFunc` timer that resets on every `Write`;
the timer is stopped in `Close`.

---

**Large payload fragmentation (RTPS DATA_FRAG)** ✅

`rtps/fragment.go` adds `DataFrag` struct, `marshalDataFrag`, `parseDataFrag`,
`splitIntoFragments` (splits payload into ≤1200-byte chunks), `splitIntoFragmentsN`
(custom fragment size for TSN frame-bound compliance), and `fragmentAssembler`
(reassembles concurrent streams keyed by writer+seqnum). The submessage ID
`0x16` is defined as `submsgDATAFRAG`. Both marshal/parse/reassembly (receive
path) and the send path (large payloads trigger automatic DATA_FRAG) are tested.

---

**Topic wildcards** ✅

`rtps/wildcard.go` exports `TopicMatches(pattern, topic string) bool`
implementing MQTT-style `+` (one level) and `#` (remainder). Mock's
`broker.publish` iterates all subscriber patterns and delivers to any whose
pattern matches the published topic. The same `matchSegments` implementation
is shared by both packages (copied into `mock/mock.go` as an unexported helper).

---

**Metrics / statistics API** ✅

`dds.Metrics` struct (WriteCount, DeliverCount, DropCount, BytesWritten,
BytesDelivered) and `dds.MetricsProvider` interface. Both `mock.participant`
and `rtps.participant` implement `Metrics() dds.Metrics` backed by
`atomic.Uint64` counters. Counters are incremented in `Write` (writes +
bytes written) and `dispatchToReaders` (delivers / drops + bytes delivered).

---

**RTPS persistent history** ✅

`rtps/persist.go` adds `WithPersistentHistory(dir string)` participant option,
`persistFlush(dir, topic, payload)` (writes 4-byte LE length prefix + payload
to `<dir>/topic-<safe(topic)>.bin`), and `persistLoad(dir, topic)`.
`rtpsWriter.Write` calls `persistFlush` after storing to `lastSample`.
`NewSubscriber` with `TransientLocal` durability tries `persistLoad` when no
in-memory sample exists yet, enabling cross-restart last-value delivery.

---

**Real-time web monitor** ✅

`monitor/monitor.go` exposes `monitor.New(p dds.Participant, opts Options)
(*Monitor, error)`. The monitor binds an HTTP server (configurable `Addr`,
default `:8080`), serves an embedded single-page dashboard (`monitor/static/
index.html` via `//go:embed`), and pushes SSE events: `sample` (whenever
`m.Publish(s)` is called) and `metrics` (polled from `dds.MetricsProvider`
every `MetricsInterval`). No external dependencies — pure standard library
(`net/http`, `encoding/json`, `embed`).

---

### v0.4.0 — Operational, Transport, and Integration

**Configurable subscriber channel depth and back-pressure policy** ✅

`WithChannelDepth(n int)` and `WithBackPressure(BackPressurePolicy)` are now
`SubscriberOption` values in `dds.go`. `BackPressurePolicy` is an exported type
with three constants: `DropNewest` (default, matches pre-v0.4 behaviour),
`DropOldest` (evicts the oldest queued sample to make room), and `Block` (blocks
the deliver goroutine until the subscriber drains). Both `mock/mock.go`
(`broker.deliver`) and `rtps/participant.go` (`deliverToReader`) implement all
three policies with a `switch` on `r.backPressure`.

---

**Structured logging (`slog.Logger` injection)** ✅

`WithLogger(l *slog.Logger)` is now available on both `mock.New` and `rtps.New`.
The `rtps` package wraps the logger in a `plog` helper (`rtps/log.go`) with
`debug`, `info`, and `warn` methods that are no-ops when the logger is nil.
Log sites: participant start, publisher/subscriber creation.

---

**Participant liveliness detection and callback** ✅

`dds.GUID` is a `[16]byte` opaque type. `dds.LivelinessEvent` is an `int` with
`LivelinessGained` and `LivelinessLost` constants. `WithLivelinessCallback(fn
func(dds.GUID, dds.LivelinessEvent))` is available on both `mock.New` and
`rtps.New`. In the RTPS path, `spdp.go:storePeer` fires `LivelinessGained` on
first discovery and `evictExpired` fires `LivelinessLost` on lease expiry. In
the mock, callbacks fire on `New` (Gained) and `Close` (Lost).

---

**Graceful shutdown with reliable-ACK drain** ✅

`dds.Drainer` is an interface with `CloseWithDrain(ctx context.Context) error`.
Package-level `dds.CloseWithDrain(ctx, p)` calls `p.(Drainer).CloseWithDrain`
if implemented, otherwise falls back to `p.Close()`. The RTPS participant
implements `Drainer`: each reliable `rtpsWriter` gains `drainCh chan struct{}` and
`ackedLo uint32`. `advanceAcked(base uint32)` updates `ackedLo` from ACKNACK
messages and closes `drainCh` when `ackedLo >= seqLo`. `CloseWithDrain` waits
on all reliable-writer drain channels before closing sockets.

---

**Multicast data delivery** ✅

The participant now creates a `dataMcastSock` that joins `239.255.0.2` on
`userMulticastPort(domain)` (= 7400 + 250*domain + 1) unless `WithNoMulticast`
is set. The `dataReceiveLoop` fan-in includes this socket. In `rtpsWriter.Write`,
when matched readers exist and `dataMcastSock` is available, one multicast send
replaces the previous N unicast sends.

---

**Shared memory transport (`shmem/` sub-package)** ✅

`shmem.New(domain)` returns a `dds.Participant` backed by an in-process
`shmBroker` (zero-copy for same-process) plus a file-backed cross-process
channel (`/tmp/godds-shmem/<topic>/data.bin`) signalled via a Unix domain
socket (`notify.sock`). The `shmSubscriber` fans in samples from both sources.
Implements `dds.Drainer` and `dds.MetricsProvider`.

---

**INFO_TS submessage — source timestamps in `Sample.Timestamp`** ✅

`dds.Sample` gains a `Timestamp time.Time` field (zero when no timestamp was
present). `marshalInfoTS` and `parseInfoTS` use the NTP64 encoding
(seconds since 1 Jan 1900 + 32-bit fractional second). `rtpsWriter.Write`
captures `time.Now()` and prepends an INFO_TS submessage before each DATA
submessage. `handleDataPacket` parses INFO_TS and threads the timestamp through
`dispatchToReaders`. The mock transport also sets `Timestamp: time.Now()`.

---

**MQTT bridge (`bridge/mqtt/`)** ✅

`bridge/mqtt.NewBridge(p dds.Participant, client MQTTClient, opts Options)`
creates a bidirectional bridge. `MQTTClient` is an interface (no paho dep in
go-DDS go.mod). `QoSMapper` and `TopicMapper` interfaces allow caller-supplied
policy; built-ins are `DefaultQoSMap` and `IdentityMap` / `PrefixMap(dds, mqtt)`.

---

**Typed generics (`TypedPublisher[T]` / `TypedSubscriber[T]`)** ✅

`dds.Codec[T any]` interface with `Marshal(T) ([]byte, error)` and
`Unmarshal([]byte) (T, error)`. `dds.JSONCodec[T]` is a zero-size built-in
backed by `encoding/json`. `TypedPublisher[T]` and `TypedSubscriber[T]` decode
errors are dropped silently.

---

**OpenTelemetry-compatible tracing** ✅

`dds.Tracer` interface with `Start(ctx, spanName, ...SpanAttribute) (context.Context, Span)`.
`dds.NoopTracer` is the zero-cost default. `rtps.WithTracer(t dds.Tracer)` wires
the tracer into the participant; `dispatchToReaders` opens a `dds.dispatch` span
per delivery batch. No `go.opentelemetry.io/otel` import.

---

### v0.5.0 — TSN (Time-Sensitive Networking)

**TSN-extended QoS fields** ✅

`dds.QoS` gains five TSN fields: `TransportPriority int` (maps to VLAN PCP /
SO_PRIORITY; 0 = normal, 1–7 = elevated), `LatencyBudget time.Duration`
(acceptable end-to-end delivery budget), `Lifespan time.Duration` (sample TTL
from write time), `PublishPeriod time.Duration` (nominal publish rate for TSN
stream reservation), and `MaxSampleSize int` (max `Write` payload bytes;
`ErrPayloadTooLarge` is returned if exceeded). The `ErrPayloadTooLarge` sentinel
error is defined in `dds.go` and wrappable via `errors.Is`. These fields are
zero-value safe: setting none of them gives identical behaviour to pre-v0.5 code.

---

**DDS-to-TSN stream model (`tsn/` sub-package)** ✅

The new `tsn/` package defines `Stream` (topic, VID, PCP, DSCP, MaxFrameSize,
MaxIntervalFrames, IntervalUS, TxOffsetUS, TalkerID), `StreamConfig` (a slice
of streams), `LoadConfig(path string)` (reads a JSON file), and `ParseConfig([]byte)`
(parses from bytes). Validation rejects empty topics, PCP > 7, and DSCP > 63.
`StreamConfig.StreamForTopic(topic)` does exact-match lookup; `nil` on a nil
`StreamConfig` is safe (returns nil). `Stream.Interval()` and `Stream.TxOffset()`
return `time.Duration`; `Stream.MaxFragPayload()` computes the fragment payload
cap (MaxFrameSize − 48 bytes for RTPS headers) for use by the writer.

---

**VLAN, PCP, and DSCP socket marking (Linux-only)** ✅

`rtps/traffic_linux.go` (build tag `linux`) implements `setSockPriority`
(`SO_PRIORITY` via `syscall.SetsockoptInt`), `setSockTOS` (`IP_TOS`, with
DSCP shifted left by 2), `enableTxTime` (`SO_TXTIME` with `CLOCK_TAI` via raw
`syscall.RawSyscall6`), and `clockTAINow` (`CLOCK_TAI` via `syscall.ClockGettime`).
`rtps/traffic_other.go` (build tag `!linux`) provides no-op stubs so the code
compiles on macOS, Windows, and all other platforms without any conditional
logic in the calling code. No external dependencies are needed; all Linux APIs
are accessible through the standard `syscall` package.

---

**Scheduled transmit time (`SO_TXTIME` + ETF/taprio qdisc)** ✅

`scheduledSend(conn, dst, data, txTimeNS)` in `traffic_linux.go` builds a
`SCM_TXTIME` control message carrying a TAI nanosecond timestamp and calls
`syscall.Sendmsg` with it. The ETF or taprio qdisc on the egress NIC holds the
packet in a time-sorted queue until the scheduled time, then releases it for
transmission. `rtpsWriter.Write` computes `txTimeNS` as the next interval
boundary plus `TxOffset` (from the matched `tsn.Stream`) relative to the
current CLOCK_TAI time. Scheduled send is only activated when
`tsnStream.TxOffsetUS > 0`; all other writers fall back to ordinary `WriteToUDP`.

---

**gPTP / IEEE 802.1AS time base (CLOCK_TAI)** ✅

`clockTAINow()` (Linux) calls `syscall.ClockGettime(CLOCK_TAI, ...)` to read
the kernel's TAI clock, which `ptp4l` + `phc2sys` keep synchronised to the
gPTP master clock on a TSN network. TAI is not adjusted for leap seconds,
making it monotonic and suitable as the scheduling reference. On non-Linux
platforms the stub returns `time.Now()`. No external Go module dependency is
needed; the standard `syscall` package provides `ClockGettime` on Linux.

---

**Separate traffic-class sockets (per-PCP)** ✅

`rtpsWriter` gains `tsnSock *udpSocket` and `tsnStream *tsn.Stream`. The
participant maintains `tsnSocks map[uint8]*udpSocket` (keyed by PCP) protected
by `tsnMu sync.Mutex`. `tsnSocketForPCP(pcp, dscp, wantTxTime)` allocates a
socket on first use via `newUnicastSocket(0)` (OS-assigned ephemeral port),
then applies `setSockPriority`, `setSockTOS`, and optionally `enableTxTime`.
`rtpsWriter.sendSock()` returns the TSN socket when present, otherwise the
shared `dataSock`. Heartbeats for TSN reliable writers also use `sendSock()`.
Non-TSN traffic continues to share `dataSock`; multicast is disabled for
TSN writers (each stream needs its own priority-marked socket).

---

**TSN-safe SPDP discovery** ✅

`WithSPDPInterval(d time.Duration)` configures the SPDP announcement period
(default 2 s). `WithSPDPJitter(d time.Duration)` adds a uniformly-distributed
random delay of up to `d` before each announcement, spreading out simultaneous
power-cycle floods across a TSN segment. `WithStaticPeers(addrs ...string)` is
an alias for `WithPeerLocators` provided for TSN configuration clarity (pair
with `WithNoMulticast()` to disable SPDP multicast entirely on deterministic
networks). These options are stored in the `participant` struct and applied in
`spdp.go:announceLoop`.

---

**Fragmentation bounds for TSN streams** ✅

`splitIntoFragmentsN(writerEID, seqNum, payload, maxPayloadSize int) []DataFrag`
generalises `splitIntoFragments` to accept a caller-supplied fragment size.
`rtpsWriter.fragmentSize()` returns `tsnStream.MaxFragPayload()` when a stream
is attached, otherwise `maxFragmentPayload` (1200 bytes). `rtpsWriter.Write`
now fragments automatically when `len(wrapped) > fragmentSize()`, sending each
fragment as a `DATA_FRAG` submessage with an INFO_TS header. `MaxSampleSize`
QoS enforcement (early `ErrPayloadTooLarge` return) happens before fragmentation
so oversized payloads are never partially sent.

---

**External TSN configuration (JSON `tsn_streams:` file)** ✅

`tsn.LoadConfig(path)` reads a JSON file containing a top-level `"streams"`
array. `tsn.ParseConfig([]byte)` parses from bytes (useful for config embedded
in a larger YAML structure after an outer YAML/JSON parser extracts the value).
`rtps.WithTSNConfig(cfg *tsn.StreamConfig)` wires the config into the
participant; publishers whose topic matches a stream entry are automatically
assigned a TSN socket and stream descriptor. The JSON format is documented in
the `tsn` package godoc with a full example config.

---

## Future Phases

The sections below capture where go-DDS is headed. Items are grouped by
concern. Nothing is scheduled — the order reflects implementation dependencies
more than priority.

---

### Phase 1 — DDS API Completeness

**Full DDS API surface.** The current API (`Participant`, `Publisher`,
`Subscriber`, `QoS`) is sufficient for publish-subscribe but omits the full
DDS 1.4 programming model. Future work adds `Topic` as a first-class object
(allowing QoS inheritance and type metadata), `Listener` interfaces for
status-driven callbacks (rather than polling or channels), `Condition` and
`WaitSet` extensions that include `ReadCondition` and `QueryCondition`, and
`Status` objects that carry per-entity counts and change flags. Adding these
completes the surface expected by applications ported from other DDS stacks.

**Complete Core QoS set.** The QoS model today covers Reliability, Durability,
History, Deadline, Lifespan, LatencyBudget, TransportPriority, and MaxSampleSize.
Still missing: `Ownership` and `OwnershipStrength` (exclusive write access),
`Liveliness` (assertion-based liveness separate from SPDP lease expiry),
`DestinationOrder` (by-source-timestamp vs by-reception-timestamp),
`Presentation` (coherent sets and ordered access), `Partition` (scoped matching),
`UserData` / `TopicData` / `GroupData` (metadata blobs in discovery), and
`TimeBasedFilter` (minimum separation between delivered samples). Each policy
requires wire encoding in SEDP parameter lists and enforcement in the delivery path.

**QoS profiles (YAML/JSON).** Hard-coding QoS in source is fragile in deployed
systems. A `qos.LoadProfile(path)` function would parse YAML or JSON files into
named `QoS` structs that applications can reference by name. Profiles support
inheritance (a `realtime` profile that extends `bestEffort`) and compatibility
checking (warn when writer and reader QoS are incompatible before the first
sample is lost).

---

### Phase 2 — Types and Serialisation

**IDL parser and Go code generation.** OMG IDL is the canonical schema language
for DDS. An `idl/` sub-package would parse `.idl` files and emit Go structs,
plus `Marshal` / `Unmarshal` methods targeting CDR. A `go generate` directive
would wire this into normal Go builds. Generated types integrate directly with
`TypedPublisher[T]` and `TypedSubscriber[T]` by satisfying `Codec[T]`.

**CDR / XCDR serialisation.** The current `cdr.go` only wraps and unwraps the
4-byte CDR_LE header. Full CDR support handles primitive types, strings, arrays,
sequences, unions, and nested structs in the CDR1 and XCDR2 encodings. XCDR2 is
required for DDS XTypes and for interoperability with recent versions of
CycloneDDS and RTI Connext.

**DDS XTypes — dynamic type system.** XTypes adds `TypeObject`, `TypeIdentifier`,
`DynamicData` (runtime-typed samples without generated code), type discovery
via SEDP builtin endpoints, schema evolution with compatibility validation, and
runtime introspection. XTypes is the foundation for tooling that can inspect
live DDS traffic without application-specific code.

---

### Phase 3 — Discovery

**Discovery Server.** Multicast SPDP does not work across routers or cloud
subnets. A Discovery Server is a well-known unicast endpoint that acts as a
rendezvous point: participants send their SPDP announcements to it and the
server relays to matched participants. The `discovery/` sub-package would
implement both the server process (a lightweight standalone binary) and the
client-side option `WithDiscoveryServer(addr string)`.

**WAN discovery.** Participants deployed across cloud VMs, behind NAT, or
in disconnected networks need a discovery path that works over routed IP.
Options include the Discovery Server pattern, a STUN/TURN-assisted approach
for NAT traversal, and pre-configured static peer lists. The goal is to allow
go-DDS participants to discover each other without any shared multicast domain.

---

### Phase 4 — Transport Framework

**Transport plugin interface.** The current transport is tightly coupled to UDP
sockets in `transport.go`. A `transport.Transport` interface (Send, Receive,
JoinMulticast, LeaveMulticast) decouples the RTPS layer from the underlying
network and allows alternate transports to be wired in without forking the
participant code. Built-in implementations: UDPv4 (current), UDPv6, TCP
(needed for WAN and some automotive backbones), QUIC (low-latency, loss-tolerant
for unreliable links), and the existing shared-memory transport.

**TCP transport.** TCP provides reliable ordered delivery and traverses NAT and
most firewalls. For DDS it is mainly used on WAN paths and for management
traffic. The TCP transport in `rtps/` would open a TCP listener, accept connections
from discovered peers, and multiplex RTPS submessages over the stream. Flow
control uses TCP's built-in backpressure; reliability uses HEARTBEAT/ACKNACK
as normal (the transport can be configured to skip RTPS-level reliability if
TCP's own reliability is sufficient).

**QUIC transport.** QUIC provides encrypted, multiplexed, low-latency delivery
suitable for edge-to-cloud and V2X use cases. A QUIC transport would replace
the UDP socket in `transport.go` with `quic-go` streams while keeping the RTPS
framing layer unchanged. The encrypted-by-default nature of QUIC provides
transport-level security separate from the DDS Security plugin.

---

### Phase 5 — Performance

**Zero-copy loaned samples.** The current delivery path copies the payload into
each subscriber's channel. For high-frequency topics (1 kHz+, large payloads)
this dominates CPU time. Zero-copy delivery uses a reference-counted pool: the
publisher writes to a pooled buffer, subscribers receive a handle, and the
buffer is returned to the pool when all subscribers release their handle.
This requires a `LoanedSample` type and `Release()` method on `Subscriber`.

**Flow control and rate limiting.** Unthrottled writers can overwhelm subscriber
channels and network buffers. A configurable async publication queue per writer
would buffer samples and allow the application to set per-topic maximum
rates and burst sizes. Back-pressure from a full queue can propagate to the
application (`Block`) or drop oldest / newest according to the `BackPressurePolicy`.

**Benchmarking suite.** Latency (percentile at 99th, 99.9th), throughput
(bytes/s and samples/s at various payload sizes), memory (allocations per
sample), CPU (cycles per sample), and scalability (N publishers × M subscribers)
benchmarks should live in `benchmarks/`. A regression gate in CI (±10% of
a recorded baseline) catches performance degradations before they ship.

---

### Phase 6 — Security (DDS Security Specification)

**DDS Security plugin compatibility.** The current security layer is a
go-DDS-specific payload-level plugin. Full DDS Security (`dds-security-1.1`)
defines five built-in plugins: Authentication (`DDS:Auth:PKI-DH`), Access Control
(`DDS:Access:Permissions`), Cryptographic (`DDS:Crypto:AES-GCM-GMAC`), Logging
(`DDS:Logging:DDS_LogTopic`), and Data Tagging. Implementing the spec-compliant
interfaces allows go-DDS participants to join secured domains run by RTI Connext
or Fast DDS with `SECURITY_ENABLED` governance files.

**X.509 certificate identity and governance.** The Authentication plugin
requires X.509 certificates for participant identity (P7 `IdentityCredential`),
plus a Permissions CA, a Governance XML file, and a Permissions XML file.
go-DDS would add a `security/x509/` package that loads these from disk,
validates the chain, and wires them into the Authentication and Access Control
plugins. Key rotation (hot-swap without restart) is a follow-on.

**Secure discovery.** Discovery (SPDP/SEDP) traffic must also be authenticated
and optionally encrypted in secured domains. The `ParticipantStateless` and
`ParticipantVolatile` builtin endpoints carry the authentication handshake;
SEDP messages carry signed endpoint announcements. This extends `spdp.go` and
`sedp.go` to attach security metadata when the Authentication plugin is active.

---

### Phase 7 — Observability

**Per-entity metrics.** The current `dds.Metrics` struct is per-participant.
Future work adds per-topic, per-writer, per-reader, per-transport, and
per-discovery metrics (e.g. SPDP announcement rate, SEDP match count, HEARTBEAT
sent/received, ACKNACK sent/received, security authentication failures). These
flow into the monitor dashboard and into OpenTelemetry metric instruments.

**Structured log correlation.** Log entries today carry only a free-form
message. Adding participant GUIDs, topic names, sequence numbers, and
correlation IDs to log attributes allows log aggregation tools (Loki, CloudWatch,
Datadog) to join DDS log events with network traces and application logs.

**CLI tools.** A `go-dds` CLI binary (in `cmd/go-dds/`) would provide `topic list`,
`participant list`, `subscribe <topic>` (print samples to stdout), `publish
<topic> <payload>`, and `diagnostics` (show drop counts, latency, QoS mismatches).
Implemented over the `rtps` package; no agent or special server required.

---

### Phase 8 — Recording and Replay

**Topic recording.** A `recorder/` sub-package wraps a participant and
intercepts all received samples, writing them to an append-only file with
nanosecond-precision timestamps and topic names. The file format is a simple
length-prefixed binary record (not a database) optimised for sequential writes
at high sample rates.

**Deterministic replay.** The recorder file can be replayed at the original
rate, at a configured speed multiplier, or at maximum speed. Replay creates a
participant that publishes recorded samples as if they were live, allowing
developers to reproduce field behaviour and test changes against captured data.

**RTPS PCAP capture.** An integration with `github.com/google/gopacket` (or
a zero-dependency pcap writer) would capture the raw RTPS UDP traffic to
standard `.pcap` / `.pcapng` files, readable by Wireshark (which has a built-in
RTPS dissector). This is valuable for debugging interoperability issues and
for offline analysis of timing.

---

### Phase 9 — Routing and Bridging

**DDS Router.** A router forwards samples between DDS domains (e.g. domain 0
on the vehicle network to domain 100 in the cloud gateway) and between network
segments. The `router/` sub-package would accept routing rules (topic patterns,
QoS translation, domain mapping) and create participant pairs in each domain.
It exposes a management HTTP API for runtime rule updates.

**Protocol Bridges.** The MQTT bridge (`bridge/mqtt/`) is already shipped.
Future bridges: Kafka (for high-throughput archiving and stream processing),
WebSocket (for browser-based dashboards), gRPC (for microservice integration),
and VISSR (for automotive software-defined vehicle data brokers). Each bridge
follows the same interface pattern as the MQTT bridge.

---

### Phase 10 — Safety and E2E Protection

**Safety Envelope (E2E header).** Automotive functional safety standards
(ISO 26262, IEC 61508) require E2E protection for safety-relevant data
channels. The `safety/` sub-package would prepend a compact header to each
sample payload containing: ProfileID, Counter, DataID, SourceID, SchemaID,
Timestamp, PayloadLength, and CRC (CRC-32/CRC-64 selectable by profile).
The safety envelope is transparent to the DDS wire format — it is just payload
bytes from RTPS's perspective — but is verified by a `SafetySubscriber` wrapper
that rejects samples with invalid CRCs, stale counters, or wrong source IDs.

**E2E Error Detection.** The safety runtime detects: corruption (CRC mismatch),
repetition (counter did not advance), loss (counter skipped), delay (timestamp
too old), stale data (same counter received twice), insertion (foreign sample),
masquerade (wrong SourceID), wrong schema (SchemaID mismatch), wrong topic
(DataID mismatch), and wrong sequence (out-of-order delivery). Each violation
fires a configurable callback and increments a labelled counter in `dds.Metrics`.

**Safety Runtime Constraints.** A `safety.BoundedParticipant` wrapper enforces:
maximum queue depth per subscriber (panics if exceeded rather than silently
dropping), fixed-size memory mode (pre-allocated sample pools, no heap
allocation in the write/deliver path), goroutine budget (panics if more than
N goroutines are started), and panic containment (wraps each delivery in a
`recover` so a panicking subscriber cannot kill the process).

---

### Phase 11 — TSN Enhancements

**Full IEEE 802.1AS clock integration.** The current `clockTAINow()` uses the
kernel TAI clock as a proxy for gPTP time, which is only valid after `ptp4l` and
`phc2sys` have converged. Future work adds a `tsn.Clock` interface (`Now() time.Time`,
`Offset() time.Duration`) with a `PHCClock` implementation that reads the PTP
Hardware Clock directly via `ioctl(SIOCGHWTSTAMP)`, bypassing the kernel
synchronisation step for lower jitter. A `LinuxKernelClock` wrapper and a mock
clock for testing complete the set.

**TSN stream diagnostics.** A `tsn.Monitor` type would track per-stream
statistics: scheduled transmit misses (when `SO_TXTIME` reports a late send),
clock drift (difference between TAI and the application's expected TX time),
frame size violations (payloads that hit `MaxSampleSize`), and interval
violations (samples written too fast or too slow relative to `PublishPeriod`).
Diagnostics surface as `dds.Metrics` extensions and as SSE events in the web monitor.

**TAPRIO / CBS configuration helpers.** go-DDS cannot configure the Linux
traffic control (`tc`) subsystem, but it can provide shell-script templates
or Netlink helpers that calculate the TAPRIO gate table and CBS bandwidth
allocation from the loaded `tsn.StreamConfig`. This makes it easier for system
integrators to set up the kernel side consistently with the go-DDS stream
parameters.

---

### Phase 12 — Unified Configuration

**Single YAML/JSON configuration file.** Today, each participant is configured
programmatically via `With*` options. A unified config loader would parse a
YAML/JSON file into a `config.ParticipantConfig` struct covering: domain,
transport (addresses, ports, multicast, IPv6), QoS profiles, security
(plugin, keys, certificates, governance), discovery (peers, server, intervals),
TSN streams, monitoring (address, interval), and logging (level, format).
This separates deployment configuration from application code.

**Hot reload of selected settings.** Some settings (log level, SPDP interval,
TSN jitter, security key rotation) can be changed at runtime without restarting
the participant. A `config.Watch(path string, onChange func(delta Config))` API
would monitor the config file for changes and apply safe updates atomically.

**Admin HTTP API.** A lightweight REST / JSON-RPC API (`admin/`) would expose
participant introspection (list topics, list peers, show QoS), runtime
diagnostics (metrics, health), and configuration validation (check if a proposed
QoS pair is compatible) over HTTP. The admin API is an optional dependency —
participants that do not start it incur zero overhead.

---

### Phase 13 — Platform Support

**Embedded Linux target.** go-DDS already runs on embedded Linux with no native
dependencies. Future work includes: a reduced-capability build tag
(`embedded`) that omits the web monitor, recorder, and admin API to minimise
binary size; a static analysis pass to confirm no heap allocation in the
publish/subscribe hot path (required by some real-time OS profiles); and
integration testing on Yocto-based images using QEMU.

**Windows / macOS parity.** TSN socket options (`SO_TXTIME`, `IP_TOS`, `SO_PRIORITY`)
are Linux-specific. On Windows and macOS, go-DDS falls back gracefully (the
no-op stubs are already in place). Future work documents the limits of each
platform and provides equivalent best-effort alternatives where they exist
(e.g., macOS `SO_TRAFFIC_CLASS` socket option for priority marking).

---

### Phase 14 — Automotive and V2X Integration

**VSS / VDM schema import.** COVESA Vehicle Signal Specification (VSS) defines
a tree of signal names (e.g., `Vehicle.Speed`, `Vehicle.Cabin.HVAC.AmbientAirTemperature`).
A `vss/` sub-package would parse a VSS JSON or YAML tree and generate DDS
topic names, QoS profiles (high-frequency sensor signals use BestEffort; actuator
commands use Reliable), and optionally `TypedPublisher[T]` / `TypedSubscriber[T]`
types from the signal metadata. This integrates directly with the IDL code
generator when signal value types are primitive or struct.

**VISSR bridge.** VISSR (COVESA Vehicle Information Service Specification
Reference Implementation) is a WebSocket/MQTT/gRPC server for vehicle signals.
The existing MQTT bridge (`bridge/mqtt/`) already connects go-DDS to VISSR's
MQTT interface. A dedicated `bridge/vissr/` package would implement the VISSR
`get`, `set`, and `subscribe` operations over the VISSR WebSocket protocol,
mapping DDS topics to VSS paths bidirectionally.

**S2DM and RCP integration.** COVESA Service Data Model (S2DM) and Remote
Control Plane (RCP) define service interfaces for SDV architectures. go-DDS
would provide a bridge between DDS DataWriters/DataReaders and S2DM service
endpoints, and implement the RCP thin-endpoint pattern (lightweight DDS participant
that registers with a centralised discovery server, allowing zonal ECUs with
limited memory to participate in a vehicle-wide DDS domain).

---

### Phase 15 — Testing Infrastructure

**Interoperability tests with multiple DDS stacks.** The existing CycloneDDS
interop test verifies basic compatibility. Future work adds interop tests
against Fast DDS (ROS 2 default), OpenDDS, and RTI Connext across a matrix of
QoS combinations (BestEffort/Reliable, Volatile/TransientLocal) and payload
sizes. These tests run in Docker Compose environments with each DDS stack as a
peer container and go-DDS as the system under test.

**Fuzzing.** RTPS packet parsing (`rtps/message.go`, `rtps/fragment.go`,
`rtps/spdp.go`, `rtps/sedp.go`), CDR serialisation, the security plugins, and
the safety E2E header all have external-facing parsers that must be hardened
against malformed input. `FuzzParseRTPS`, `FuzzParseCDR`, `FuzzOpenAESGCM`, and
`FuzzE2ECheck` targets would run in CI with a short duration and in a separate
extended fuzzing pipeline weekly.

**Fault injection framework.** A `testutil/chaos/` package would wrap a
participant to inject configurable faults: packet loss (drop N% of sends),
delay (add jitter to each send), corruption (flip bits in the payload), reorder
(swap adjacent packets), and partition (stop all traffic between two participants
for a duration). This enables resilience testing of reliable delivery, TSN
recovery, and security plugin behaviour under adverse conditions.

---

### Phase 16 — Enterprise Readiness

**Release artefacts.** Each tagged release produces: a multi-platform Docker
image, a Software Bill of Materials (SBOM) in SPDX and CycloneDX formats,
Sigstore keyless signatures for all build artefacts, and static binaries for
`go-dds` (CLI tool) and `go-dds-monitor` (web dashboard). These are published
to `ghcr.io/SoundMatt/go-DDS` and attached to the GitHub Release.

**Comprehensive documentation.** Beyond package godoc: an Architecture Guide
(component diagram, data-flow through the RTPS stack, threading model), a
Deployment Guide (container, embedded Linux, cloud), an API Compatibility
Commitment (semantic versioning rules, what constitutes a breaking change), and
a Troubleshooting Guide (diagnosing discovery failures, security mismatches, TSN
timing violations). Documentation lives in `docs/` and is published to a
GitHub Pages site.

**API compatibility.** The root `dds` package interface is stable from v0.5.
New methods or fields on `QoS`, `Sample`, or the participant interfaces will
follow a deprecation cycle. The `rtps`, `mock`, and `shmem` option APIs can
evolve more freely in minor versions as long as the `dds.Participant` / `Publisher`
/ `Subscriber` interface is unchanged.

---

### Phase 17 — Safety Evidence

**Test reports and coverage evidence.** The CI pipeline already generates test
results; future work captures them in a machine-readable format (JUnit XML)
and publishes them as release artefacts. Coverage reports are generated with
`-coverprofile` and uploaded to a hosted coverage service so trends are visible.
A minimum 90% statement coverage gate blocks releases.

**Fuzz campaign reports.** Each weekly fuzz run produces a report: which
targets were fuzzed, how many unique inputs were generated, whether any new
crashes were found, and the current corpus size. Crash reports are triaged and
linked to CVEs when applicable. Reports are archived as release assets.

**Traceability matrix.** A machine-readable YAML file links each item in this
roadmap to: the Go packages it touches, the test functions that verify it, the
fuzz targets that harden it, and the RTPS / DDS specification section it
implements. This traceability matrix supports IEC 61508 / ISO 26262 software
assessments by integrators who embed go-DDS in safety-relevant systems.

---

## Future Repository Split

Keep everything above in go-DDS initially. Only split into separate repositories
once the products mature and have independent release cadences.

**Proposed future organisation:**

```
SoundMatt/
├── go-DDS           (core middleware — everything in this file)
├── go-DDS-tools     (CLI utilities: go-dds, go-dds-record, go-dds-replay)
├── go-DDS-monitor   (web monitoring dashboard as a standalone service)
├── go-DDS-recorder  (record/replay as a deployable service)
├── go-DDS-router    (routing, domain bridges, protocol bridges)
├── go-DDS-vss       (VSS / VDM / S2DM integration and bridge)
├── go-DDS-rcp       (remote control plane, thin endpoints, zonal architecture)
├── go-DDS-safety    (safety documentation, evidence, and safety-kit helpers)
└── go-DDS-examples  (large, runnable example applications)
```

**Split rule:** If it is required to send or receive standards-compliant DDS
traffic, it belongs in go-DDS. If it operates, visualises, bridges, records,
configures, or manages DDS from outside the middleware core, it can become a
separate repository later.

---

## Architecture Boundary Note (TSN)

go-DDS can classify, mark, and schedule DDS traffic at the socket level, but
end-to-end TSN guarantees require co-operation from hardware and kernel outside
Go's control. The full stack is:

```
dds.QoS (TransportPriority, LatencyBudget, PublishPeriod, MaxSampleSize)
  ↓
tsn.StreamConfig (VLAN ID, PCP, DSCP, interval, max frame size, TxOffset)
  ↓
rtps.WithTSNConfig + per-PCP socket (SO_PRIORITY, IP_TOS)
  ↓
SO_TXTIME + CLOCK_TAI timestamp on outgoing packets (scheduledSend)
  ↓
Linux tc: mqprio + taprio (credit-based shaper) or etf (per-stream)
  ↓
TSN-capable NIC (Intel i210/i225, NXP ENET, Renesas AVB)
  ↓
TSN-capable Ethernet switch (802.1Qbv gate tables, 802.1Qav CBS)
  ↓
gPTP (IEEE 802.1AS) time synchronisation across all nodes
```

Before writing TSN transmit-time code, confirm that the test environment has
a TSN-capable NIC (`ethtool -T ethX` for `SOF_TIMESTAMPING_TX_HARDWARE`),
that `SO_TXTIME` is available (kernel ≥ 4.19), and that gPTP is running
(`ptp4l -i ethX` and `phc2sys`).
