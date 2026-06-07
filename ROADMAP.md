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

---

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
`splitIntoFragments` (splits payload into ≤1200-byte chunks), and
`fragmentAssembler` (reassembles concurrent streams keyed by writer+seqnum).
The submessage ID `0x16` is defined as `submsgDATAFRAG`. Integration into the
write/receive paths is the next step; this PR ships the marshal/parse/reassembly
layer with full test coverage.

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
the deliver goroutine until the subscriber drains). `SubscriberConfig` exports
`ChannelDepth` and `BackPressure` fields; `ChanDepth(default int) int` resolves
the effective depth. Both `mock/mock.go` (`broker.deliver`) and
`rtps/participant.go` (`deliverToReader`) implement all three policies with a
`switch` on `r.backPressure`.

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
on all reliable-writer drain channels before closing sockets. The mock's
`CloseWithDrain` is a no-op (all writes are synchronous).

---

**Multicast data delivery** ✅

The participant now creates a `dataMcastSock` that joins `239.255.0.2` on
`userMulticastPort(domain)` (= 7400 + 250*domain + 1) unless `WithNoMulticast`
is set. The `dataReceiveLoop` fan-in includes this socket. In `rtpsWriter.Write`,
when matched readers exist and `dataMcastSock` is available, one multicast send
replaces the previous N unicast sends (one per peer). Files: `rtps/locator.go`
(new `userDataMulticastAddr`, `userMulticastPort`), `rtps/participant.go`.

---

**Shared memory transport (`shmem/` sub-package)** ✅

`shmem.New(domain)` returns a `dds.Participant` backed by an in-process
`shmBroker` (zero-copy for same-process) plus a file-backed cross-process
channel (`/tmp/godds-shmem/<topic>/data.bin`) signalled via a Unix domain
socket (`notify.sock`). The `shmSubscriber` fans in samples from both sources.
Implements `dds.Drainer` and `dds.MetricsProvider`. Files: `shmem/shmem.go`,
`shmem/shmem_test.go`.

---

**INFO_TS submessage — source timestamps in `Sample.Timestamp`** ✅

`dds.Sample` gains a `Timestamp time.Time` field (zero when no timestamp was
present). `marshalInfoTS(t time.Time) []byte` and `parseInfoTS(body []byte)
(time.Time, bool)` were added to `rtps/message.go` using the NTP64 encoding
(seconds since 1 Jan 1900 + 32-bit fractional second). `rtpsWriter.Write`
captures `time.Now()` and prepends an INFO_TS submessage before each DATA
submessage. `handleDataPacket` parses INFO_TS and threads the timestamp through
`dispatchToReaders`. The mock transport also sets `Timestamp: time.Now()` on
each published sample.

---

**MQTT bridge (`bridge/mqtt/`)** ✅

`bridge/mqtt.NewBridge(p dds.Participant, client MQTTClient, opts Options)`
creates a bidirectional bridge. `MQTTClient` is an interface (no paho dep in
go-DDS go.mod). `QoSMapper` and `TopicMapper` interfaces allow caller-supplied
policy; built-ins are `DefaultQoSMap` and `IdentityMap` / `PrefixMap(dds, mqtt)`.
The bridge subscribes to DDS topics and calls `client.Publish`, and calls
`client.Subscribe` and forwards to `participant.NewPublisher`. Files:
`bridge/mqtt/mqtt.go`, `bridge/mqtt/mqtt_test.go`.

---

**Typed generics (`TypedPublisher[T]` / `TypedSubscriber[T]`)** ✅

`dds.Codec[T any]` interface with `Marshal(T) ([]byte, error)` and
`Unmarshal([]byte) (T, error)`. `dds.TypedPublisher[T]` wraps a `Publisher`,
encodes with `Codec.Marshal` before `Write`. `dds.TypedSubscriber[T]` wraps a
`Subscriber`, runs a pump goroutine that decodes with `Codec.Unmarshal` and
delivers `TypedSample[T]` values (decode errors are dropped). `dds.JSONCodec[T]`
is a zero-size built-in backed by `encoding/json`. All in `dds.go`.

---

**OpenTelemetry-compatible tracing** ✅

`dds.Tracer` interface (mirroring the OTel trace.Tracer shape) with `Start(ctx,
spanName, ...SpanAttribute) (context.Context, Span)`. `dds.Span` with
`SetAttribute` and `End`. `dds.NoopTracer` is the zero-cost default. `dds.SpanAttribute`
is a `Key/Value string` struct. `rtps.WithTracer(t dds.Tracer)` stores the tracer
on the participant; `dispatchToReaders` opens a `dds.dispatch` span per delivery
batch. No `go.opentelemetry.io/otel` import — callers bridge their real tracer
via a thin adapter. All in `dds.go` + `rtps/participant.go`.

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
