# taprio-stream

TSN (Time-Sensitive Networking) stream scheduling for DDS topics using `tsn.StreamConfig` and `tsn.TAPRIOConfig`.

**Pattern:** bounded-latency publishing — a TSN stream descriptor binds a DDS topic to a TAPRIO gate control schedule. Published frames carry a computed SO_TXTIME transmit timestamp aligned to the TSN interval.

## Run

```sh
go run .
```

## What it shows

| Concept | Where |
|---|---|
| `tsn.StreamConfig` | per-topic TSN stream descriptors (VID, PCP, DSCP, interval, offset) |
| `tsn.TAPRIOFromStreams` | derives a TAPRIO gate control list from stream descriptors |
| `TAPRIOConfig.TCCommand` | generates a `tc(8)` command for manual qdisc programming |
| `TAPRIOConfig.Apply()` | programs the TAPRIO qdisc via `RTM_NEWQDISC` netlink (Linux only) |
| `nextTxTime` | computes per-frame SO_TXTIME: next interval boundary + configured offset |
| `TypedPublisher[T]` | publishes `VehicleSpeed` structs with embedded transmit timestamp |

## TAPRIO on Linux

To actually program the kernel qdisc (requires Linux + `CAP_NET_ADMIN`):

```go
taprioCfg.Interface = "eth0"
taprioCfg.BaseTime = 0 // kernel picks next cycle boundary
if err := taprioCfg.Apply(); err != nil {
    log.Fatal(err)
}
```

On macOS/Windows, `Apply()` returns `tsn.ErrNotSupported`. Use `TCCommand` to get the equivalent `tc(8)` shell command and run it manually.

## Expected output

```
Stream: vehicle/speed  interval=1ms  offset=0s  PCP=5  DSCP=46

TAPRIO schedule: cycle=3ms  entries=2
  entry[0]: gate_mask=0x20  interval=1ms
  entry[1]: gate_mask=0x10  interval=2ms

tc command (for use on Linux with CAP_NET_ADMIN):
  tc qdisc replace dev eth0 parent root handle 100 taprio ...

Publishing 8 frames at 1ms intervals:
  [send] kph=80.0  sched_tx=1749413400001000000ns
  [recv] kph=80.0  sched_tx=1749413400001000000ns
  ...
```
