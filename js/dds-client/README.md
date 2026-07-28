# @go-dds/dds-client

A dependency-free TypeScript/JavaScript client for go-DDS's [`bridge/ws`](../../bridge/ws)
WebSocket gateway (ROADMAP.md Milestone 16, "WebSocket Transport"):
`TypedPublisher` and `TypedSubscriber` over a single WebSocket connection,
for browsers, Node.js, and edge/cloud-function runtimes.

## Scope

go-DDS's Milestone 16 ships **two** independent ways for a WebSocket peer to
reach a DDS domain, and this package is the client for only one of them:

- **`rtps.WithWSAddr`** (`rtps/transport_ws.go`) is the native
  RTPS-over-WebSocket *transport*: a peer that speaks it joins the domain as
  a genuine RTPS participant, performing its own SPDP/SEDP discovery — no
  bridge process involved at all. A Go program compiled for a browser/Wasm
  target (the sibling "WebAssembly Target" sub-phase) would use this
  directly. This package does **not** implement that protocol.
- **`bridge/ws`** is a separate, optional gateway: a small Go HTTP server
  that holds one real `dds.Participant` and re-exposes it to WebSocket
  clients as a much simpler JSON publish/subscribe protocol, so a client
  doesn't have to implement RTPS discovery at all. **This package is the
  client for that protocol.**

If you need a browser tab to be a first-class RTPS participant with no Go
process in between, you want `WithWSAddr` (paired with a Wasm-compiled
go-DDS build). If you just want a web page or a cloud function to publish
and subscribe to DDS topics with minimal client-side complexity, you want
this package talking to a `bridge/ws` gateway.

## Install

```sh
npm install @go-dds/dds-client
```

## Usage

```ts
import { DDSClient, TypedPublisher, TypedSubscriber } from "@go-dds/dds-client";

const client = new DDSClient("wss://example.com/dds-gateway");

interface Reading {
  sensor: string;
  celsius: number;
}

const sub = new TypedSubscriber<Reading>(client, "sensors/temp");
sub.onMessage((reading) => console.log(reading.sensor, reading.celsius));

const pub = new TypedPublisher<Reading>(client, "sensors/temp");
pub.publish({ sensor: "cabin", celsius: 21.5 });
```

`TypedPublisher`/`TypedSubscriber` JSON-encode/decode values by default
(matching ROADMAP.md's "JSON ... framing" convention). Pass a custom
[`Codec<T>`](./src/index.ts) as the third constructor argument for a
different wire representation — `bytesCodec` (identity) and `textCodec`
(plain UTF-8 strings, no JSON envelope) are provided.

For lower-level use, `DDSClient.subscribeRaw`/`publishRaw` work directly
with `Uint8Array` payloads without a codec.

### Server (Go) side

```go
p, _ := rtps.New(dds.Domain(0))
bridge := ws.New(p, ws.Options{})
http.ListenAndServe(":8091", bridge)
```

## Authentication

A `bridge/ws` gateway configured with `Options.AuthToken` normally expects
`Authorization: Bearer <token>`. The standard browser `WebSocket` API gives
page JavaScript no way to set that header on the opening handshake, so this
client instead sends the token as a `?token=` query parameter — the
browser-compatible fallback `Bridge.authorize` on the Go side also accepts:

```ts
const client = new DDSClient("wss://example.com/dds-gateway", {
  authToken: "your-token",
});
```

A non-browser client (Node.js, a cloud function) that would rather use the
header form can talk to the gateway directly without this package.

## Reconnection

`DDSClient` reconnects automatically by default (`reconnect: true`), with
`reconnectDelayMs` (default 2000ms) between attempts. Every currently-active
subscription (from any live `TypedSubscriber`/`subscribeRaw` call) is
re-subscribed automatically once the new connection opens — no application
code needs to react to a reconnect. A queued `publish` made while
disconnected is sent as soon as the connection (re)opens; nothing is
retried or acknowledged beyond that single send.

```ts
const client = new DDSClient(url, {
  reconnect: true,
  reconnectDelayMs: 2000,
});
```

## Runtime support

This package uses only `Uint8Array`, `TextEncoder`/`TextDecoder`, `JSON`,
and a structurally-typed subset of the standard `WebSocket` API — see
[`WebSocketLike`](./src/index.ts) — so it runs unmodified in any environment
that provides a global `WebSocket` implementing that subset: browsers,
Node.js ≥ 18 (via its built-in `WebSocket`), and most edge/Workers
runtimes. In an environment without a global `WebSocket` (or to use a
different implementation, e.g. the `ws` package on an older Node version),
pass `options.webSocketFactory`:

```ts
import WebSocket from "ws";

const client = new DDSClient(url, {
  webSocketFactory: (u) => new WebSocket(u) as unknown as WebSocketLike,
});
```

## Development

```sh
npm install
npm run build   # tsc -> dist/
npm test        # build, then run dist/*.test.js under node --test
```

Tests run entirely against an in-memory fake `WebSocket` (see
`src/index.test.ts`) — no network, no Go process required — plus a
standalone base64 codec test (`src/base64.test.ts`) cross-checked against
Go's `encoding/base64.StdEncoding` output for the same bytes.

## License

MPL-2.0, same as the rest of the go-DDS repository — see the repository
root [`LICENSE`](../../LICENSE).
