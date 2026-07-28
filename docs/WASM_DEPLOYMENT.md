# Deploying go-DDS as a Wasm Edge/Cloud Function

ROADMAP.md Milestone 16, "WebAssembly Target": the "cloud function" half of
that sub-phase's success criterion ("A browser tab **and a cloud function**
can join a DDS domain alongside embedded devices without a protocol
bridge"). For the browser half, see
[`examples/wasm-subscriber`](../examples/wasm-subscriber).

## What this is, precisely

`GOOS=wasip1 GOARCH=wasm go build` produces a WebAssembly module targeting
[WASI](https://wasi.dev) preview 1 — the ABI Fastly Compute, Cloudflare
Workers (via their respective Wasm/WASI runtimes), and general-purpose WASI
hosts (Wasmtime, Wasmer) all execute. Unlike `GOOS=js GOARCH=wasm` (the
browser target — see `examples/wasm-subscriber`), Go's `net` package under
`wasip1` is backed by real WASI socket syscalls
(`$GOROOT/src/net/fd_wasip1.go`), not the browser port's in-process fake
network — so `mock` and `rtps/transport_ws.go` build, and are intended to
run, unmodified: no build tags, no stubbed-out networking, no CGo.

**Verified in this repo**: `go build`/`go vet` for `GOOS=wasip1
GOARCH=wasm` across every module (`.`, `bridge`, `tools`, `observability`,
`safety`, `examples`) — see `.github/workflows/ci.yml`'s `wasm-build` job —
and, locally during this sub-phase's development, an actual `rtps.New`
participant (using `rtps.WithWSPeers`, the dial-only mode `WithWSPeers`
doc comment in `rtps/participant.go` describes) starting up correctly under
[Wasmtime](https://wasmtime.dev) with `wasi:sockets` enabled
(`wasmtime run -S tcp=y -S udp=y -S inherit-network=y -S
allow-ip-name-lookup=y ...`). That exercise is exactly what surfaced and
fixed a real pre-existing bug this sub-phase's `REQ-TRANS-020` now covers:
`newMulticastReceiveSocket` was previously fatal, rather than falling back
to a plain unicast bind as its own doc comment already promised, whenever
`net.Interfaces()` itself returned nothing — the case under a WASI runtime
that does not implement interface enumeration at all (as opposed to
enumerating interfaces successfully but then failing the multicast join
itself, which it already handled). Without that fix, `rtps.New` could never
succeed under a WASI sandbox in the first place.

**Not independently verified by this repo**: whether a *specific* edge
platform's current WASI/networking integration bridges an inbound listener
(`rtps.WithWSAddr`) or an outbound dial (`rtps.WithWSPeers`) to genuine
external connectivity the way a local Wasmtime CLI invocation with
`-S inherit-network=y` might lead you to expect. In this repo's own local
testing, a plain Wasmtime CLI invocation with sockets enabled did **not**
bridge the guest's TCP connections through to the host's real loopback
network despite that flag — the guest's `net.Dial` to a real, independently
verified-reachable native go-DDS server returned "connection refused" —
which is a property of *that specific CLI sandbox's networking model*, not
of the go-DDS code, but it is exactly the kind of platform-specific
integration detail that will differ by host and by SDK version. Treat
everything below as a starting point to adapt against your chosen
platform's *current* documentation, not a turnkey guarantee — the same
honesty this repo's QUIC transport scoping note (ROADMAP.md, "QUIC
Transport", FastDDS interoperability) already applies to an evolving
external spec.

## Which option to reach for

A serverless/edge function, like a browser tab, essentially never has a
stable, publicly dialable address of its own — it is invoked per-request or
spun up on demand. That means **`rtps.WithWSPeers` without `rtps.WithWSAddr`
— the dial-only mode** (see that option's doc comment in
`rtps/participant.go`, and `rtps/transport_ws.go`'s doc comment on
listener-less sockets) — is very likely the right shape for this side too:
the function dials out to a real, long-lived go-DDS domain member (the same
role `examples/wasm-subscriber/server` plays for the browser demo), and
every reply/HEARTBEAT/further SEDP or user-data message flows back over
that same connection — no inbound listener, and therefore no
platform-specific "how do I expose a public port from my edge function"
problem to solve at all.

```go
p, err := rtps.New(dds.Domain(0),
    rtps.WithWSPeers("your-go-dds-broker.example.com:7800"),
    rtps.WithWSTLSConfig(&tls.Config{}), // wss:// — almost always required at the edge
)
```

## Build

From the module containing your handler code:

```sh
GOOS=wasip1 GOARCH=wasm go build -o handler.wasm .
```

TinyGo also targets `wasip1` (`tinygo build -target wasip1 -o handler.wasm
.`); as with `examples/wasm-subscriber`'s TinyGo note, this repo's own CI
only exercises the standard `go build` path.

## Fastly Compute

Fastly's Go/TinyGo toolchain (the `fastly compute` CLI,
`github.com/fastly/compute-sdk-go`) targets `wasm32-wasi` — the same ABI
family as `GOOS=wasip1 GOARCH=wasm`. Fastly's own SDK is normally the
entrypoint for handling the inbound request and making outbound calls
through Fastly's **backends** mechanism (including, in current SDK
versions, raw TCP via dynamic backends) rather than calling `net.Dial`
directly against an arbitrary host:port from an unmodified `net` import.
Whether go-DDS's own `net.Dial`-based `rtps.WithWSPeers` path works
as-is under `fastly compute serve`/a deployed Fastly Compute service, or
needs its outbound connection routed through a Fastly backend explicitly
configured for your broker's host:port, depends on the Fastly SDK/platform
version you target — consult Fastly's current Compute documentation for
its supported outbound-networking primitives before assuming a direct
`net.Dial` reaches the outside world unmodified.

## Cloudflare Workers

Cloudflare's Workers platform executes JavaScript/Wasm inside `workerd`.
Outbound networking from a Worker is normally `fetch()` (HTTP) or, for raw
TCP, the [Workers TCP Sockets
API](https://developers.cloudflare.com/workers/runtime-apis/tcp-sockets/)
(`connect()`), called from the Worker's own JavaScript — not a generic
WASI `wasi:sockets` implementation wired up under a wasm module's own
`net.Dial` calls automatically. Running a `GOOS=wasip1` go-DDS build inside
a Worker most likely means invoking it as a Wasm module from a small
JavaScript shim that opens the actual TCP connection via `connect()` and
bridges its bytes to whatever import the wasm module expects, rather than
expecting the module's own `net` package calls to reach the network
directly — this is genuinely a per-Worker integration exercise today, not
something this repo can pre-solve generically. Check Cloudflare's current
Workers documentation for the state of WASI/networking support before
planning around a specific approach.

## If your platform gives you a real WASI `wasi:sockets` host

Some deployment targets (a self-hosted Wasmtime/Wasmer-based edge runtime,
for example) genuinely do implement `wasi:sockets` end to end. In that
case, this repo's own local verification (above) is directly representative:
`rtps.New` with `WithWSPeers`/`WithWSTLSConfig` should behave exactly like
any other dial-only go-DDS participant, because from `rtps/transport_ws.go`'s
point of view it is one — nothing in the RTPS-over-WebSocket transport
distinguishes "real OS" from "WASI" once the host's `net.Dial` genuinely
reaches the target.
