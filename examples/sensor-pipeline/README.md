# sensor-pipeline

Periodic publisher and aggregating subscriber using `TypedPublisher`/`TypedSubscriber` with the JSON codec.

**Pattern:** telemetry fan-in — multiple sensor IDs publish on a single topic; one subscriber aggregates readings in real time.

## Run

```sh
go run .
```

## What it shows

| Concept | Where |
|---|---|
| `TypedPublisher[T]` + `TypedSubscriber[T]` | wraps raw `Publisher`/`Subscriber` with a codec |
| `JSONCodec[T]` | zero-config marshalling for any struct |
| `WriteCtx` | context-aware write with cancellation support |
| `dds.DefaultQoS` | best-effort, volatile — appropriate for sensor streams |

## Expected output

```
[sensor-A] 21.0°C   (running avg: 21.00°C, n=1)
[sensor-B] 19.8°C   (running avg: 20.40°C, n=2)
...
Final average over 10 readings: 22.56°C
```
