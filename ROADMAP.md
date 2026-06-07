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
