# wasm-subscriber

A browser tab running a genuine RTPS participant, compiled from Go to
WebAssembly (ROADMAP.md Milestone 16, "WebAssembly Target"). It joins a DDS
domain and subscribes to a topic entirely over the RTPS-over-WebSocket
transport (`rtps.WithWSPeers`) — performing its own SPDP/SEDP discovery
exactly like any other go-DDS participant — with **no protocol bridge** in
between.

## Scope: this is not `js/dds-client`

go-DDS's Milestone 16 ships two independent ways for a browser to reach a
DDS domain — see [`js/dds-client`](../../js/dds-client)'s README for the
full comparison:

- **`js/dds-client`** is a TypeScript client for `bridge/ws`'s separate JSON
  publish/subscribe gateway — a small Go process holding one real
  `dds.Participant` on the client's behalf, re-exposed as a much simpler
  protocol. No Go/Wasm build required on the browser side, but it is a
  bridge.
- **This example** is a real go-DDS build, compiled to WebAssembly, running
  *inside* the browser tab itself. `main.go` calls `rtps.New` with
  `rtps.WithWSPeers` directly — the tab performs RTPS discovery and message
  exchange on its own. This is what Milestone 16's success criterion means
  by "a browser tab ... can join a DDS domain ... without a protocol
  bridge."

## Why `WithWSPeers` and never `WithWSAddr`

A browser tab can never accept an inbound network connection — there is no
listener to bind. `rtps.WithWSPeers` alone (no `rtps.WithWSAddr`) puts the
RTPS-over-WebSocket transport in dial-only mode: the participant still
performs full two-way SPDP/SEDP discovery and reliable delivery, but only
ever as the connecting side. See `rtps.WithWSPeers`'s doc comment in
[`rtps/participant.go`](../../rtps/participant.go) for exactly how a peer
with no listener of its own stays reachable (replies and further messages
flow back over the same connection it dialled out on).

## Build

From the `examples/` module directory:

```sh
./wasm-subscriber/build.sh
```

This runs `GOOS=js GOARCH=wasm go build -o main.wasm .` and copies the
matching `wasm_exec.js` glue out of your local Go toolchain (`go env
GOROOT`) — see `build.sh`'s own comment for why that file is never vendored
into the repo.

You need a Go toolchain that supports `GOOS=js GOARCH=wasm` (the standard
library has since Go 1.11; this repo currently targets Go 1.25/1.26 — see
`go.mod`). No CGo, no external dependency, no browser required to build.

### TinyGo

TinyGo also targets `GOOS=js GOARCH=wasm`:

```sh
tinygo build -o main.wasm -target wasm .
```

for a materially smaller binary (`tinygo build`'s output is typically a
fraction of the standard toolchain's size, at the cost of a slower/more
restricted runtime — notably a different, non-cooperative-preemption
scheduler and a smaller standard-library surface). Nothing in `main.go` or
[`rtps/transport_ws_browser.go`](../../rtps/transport_ws_browser.go) uses
anything TinyGo doesn't support (`syscall/js`, channels, goroutines), but
only the standard `go build` path is exercised by this repo's own CI (see
`.github/workflows/ci.yml`'s `wasm-build` job) — treat the TinyGo path as
unverified by this repo until you've tried it against your TinyGo version.
TinyGo needs its own `wasm_exec.js` (bundled with the TinyGo distribution,
under `targets/wasm_exec.js`) instead of the Go toolchain's copy `build.sh`
fetches.

## Run the demo end to end

1. Start a native go-DDS peer with a WS listener bound, publishing samples
   for the browser tab to receive — this repo ships one for exactly this
   purpose:

   ```sh
   go run ./wasm-subscriber/server
   ```

   (flags: `-addr :7800 -domain 0 -topic vehicle/speed` are the defaults;
   pass your own to match a different setup.)

2. Build and serve `wasm-subscriber/` over HTTP — a Wasm module cannot be
   loaded from a `file://` URL:

   ```sh
   ./wasm-subscriber/build.sh
   cd wasm-subscriber && python3 -m http.server 8081
   ```

3. Open `http://localhost:8081/`. The page connects to `127.0.0.1:7800`,
   domain `0`, topic `vehicle/speed` by default; override any of these with
   a query string, e.g.:

   ```
   http://localhost:8081/?ws=127.0.0.1:7800&domain=0&topic=vehicle/speed
   ```

   `?tls=1` switches to `wss://` (see "Deploying over TLS" below). Open the
   browser's developer console to see connection and per-sample log lines
   too (`[wasm-subscriber] ...`).

You should see "Status: subscribed" and a growing list of received samples
within a couple of seconds — `server`'s periodic `sendAnnouncement` and the
browser participant's own reply-on-first-contact (see
`rtps/participant.go`'s `wsReplyToNewWSPeer`) complete SPDP/SEDP discovery
well inside that window.

## Deploying over TLS / from a real host

Browsers enforce mixed-content policy: a page served over `https://` may
only open `wss://` (not plain `ws://`) connections. `server`'s
`rtps.WithWSAddr` listener speaks plain `ws://` by itself; put a TLS-
terminating reverse proxy (or `rtps.WithWSTLSConfig`, if you'd rather
terminate TLS in Go itself) in front of it for a page served over HTTPS,
and pass `?ws=<host>:<port>&tls=1` so the browser dials `wss://` instead.

For running the *server* half as a serverless/edge function instead of a
long-lived process, see [`docs/WASM_DEPLOYMENT.md`](../../docs/WASM_DEPLOYMENT.md)
— the "cloud function" half of Milestone 16's success criterion, using the
same `GOOS=wasip1 GOARCH=wasm` build this repo's `wasm-build` CI job
verifies.
