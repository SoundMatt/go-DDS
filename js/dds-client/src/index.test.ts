// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Tests DDSClient/TypedPublisher/TypedSubscriber against a fully in-memory
// fake WebSocket (no real network, no real bridge/ws server) — this package
// intentionally has no test dependency on the Go side of the repo, so the
// fake directly speaks the bridge/ws JSON wire protocol its doc comment
// documents, letting these tests exercise the client's actual framing
// logic rather than just its in-process object graph.

import assert from "node:assert/strict";
import { test } from "node:test";
import {
  DDSClient,
  TypedPublisher,
  TypedSubscriber,
  bytesCodec,
  type WebSocketLike,
} from "./index.js";
import { decodeBase64, encodeBase64 } from "./base64.js";

interface ClientMessage {
  op: "subscribe" | "unsubscribe" | "publish";
  topic: string;
  data?: string;
}

/**
 * FakeSocket is a WebSocketLike whose "server side" is just this test:
 * sent frames land in `sent`, and the test drives `emitOpen`/`emitMessage`/
 * `emitClose` directly instead of a real network round trip.
 */
class FakeSocket implements WebSocketLike {
  readyState = 0; // CONNECTING
  sent: string[] = [];
  closeCalls = 0;
  onopen: ((this: WebSocketLike, ev: unknown) => void) | null = null;
  onmessage: ((this: WebSocketLike, ev: { data: unknown }) => void) | null = null;
  onclose: ((this: WebSocketLike, ev: unknown) => void) | null = null;
  onerror: ((this: WebSocketLike, ev: unknown) => void) | null = null;

  send(data: string): void {
    if (this.readyState !== 1) throw new Error("FakeSocket.send called while not open");
    this.sent.push(data);
  }

  close(): void {
    this.closeCalls++;
    this.readyState = 3;
    this.onclose?.call(this, {});
  }

  emitOpen(): void {
    this.readyState = 1;
    this.onopen?.call(this, {});
  }

  emitMessage(data: string): void {
    this.onmessage?.call(this, { data });
  }

  emitServerJSON(msg: unknown): void {
    this.emitMessage(JSON.stringify(msg));
  }

  lastSent(): ClientMessage {
    const raw = this.sent[this.sent.length - 1];
    if (raw === undefined) throw new Error("expected at least one sent frame");
    return JSON.parse(raw) as ClientMessage;
  }
}

function newHarness(opts: { reconnect?: boolean; authToken?: string } = {}) {
  const sockets: FakeSocket[] = [];
  const client = new DDSClient("ws://example.invalid/", {
    reconnect: opts.reconnect ?? false,
    authToken: opts.authToken,
    webSocketFactory: (url) => {
      const s = new FakeSocket();
      sockets.push(s);
      (s as unknown as { url: string }).url = url;
      return s;
    },
    onError: () => {}, // tests assert on behaviour, not console noise
  });
  return { client, sockets, socket: () => sockets[sockets.length - 1]! };
}

// ── DDSClient wire protocol ───────────────────────────────────────────────────

test("subscribeRaw sends a subscribe frame once the socket is open", () => {
  const { client, socket } = newHarness();
  // Before open, a subscribe call queues rather than sending immediately.
  const unsub = client.subscribeRaw("t", () => {});
  assert.equal(socket().sent.length, 0);
  socket().emitOpen();
  assert.equal(socket().sent.length, 1);
  const msg = socket().lastSent();
  assert.equal(msg.op, "subscribe");
  assert.equal(msg.topic, "t");
  unsub();
});

test("subscribeRaw queues before open and flushes exactly once on open", () => {
  const { client, socket } = newHarness();
  const received: Uint8Array[] = [];
  client.subscribeRaw("sensors/temp", (b) => received.push(b));
  assert.equal(socket().sent.length, 0);
  socket().emitOpen();
  assert.equal(socket().sent.length, 1);
  assert.deepEqual(socket().lastSent(), { op: "subscribe", topic: "sensors/temp" });
});

test("unsubscribing the last handler for a topic sends unsubscribe", () => {
  const { client, socket } = newHarness();
  const unsub = client.subscribeRaw("t", () => {});
  socket().emitOpen();
  assert.equal(socket().sent.length, 1);
  unsub();
  assert.equal(socket().sent.length, 2);
  assert.deepEqual(socket().lastSent(), { op: "unsubscribe", topic: "t" });
});

test("a second handler on the same topic does not re-send subscribe, and unsubscribe only fires after the last handler is removed", () => {
  const { client, socket } = newHarness();
  const unsub1 = client.subscribeRaw("t", () => {});
  socket().emitOpen();
  assert.equal(socket().sent.length, 1);
  const unsub2 = client.subscribeRaw("t", () => {});
  assert.equal(socket().sent.length, 1, "second handler on an already-subscribed topic must not resend subscribe");
  unsub1();
  assert.equal(socket().sent.length, 1, "unsubscribe must not fire while another handler remains");
  unsub2();
  assert.equal(socket().sent.length, 2);
  assert.deepEqual(socket().lastSent(), { op: "unsubscribe", topic: "t" });
});

test("publishRaw base64-encodes the payload", () => {
  const { client, socket } = newHarness();
  socket().emitOpen();
  const payload = new TextEncoder().encode("hello-ws-bridge");
  client.publishRaw("t", payload);
  const msg = socket().lastSent();
  assert.equal(msg.op, "publish");
  assert.equal(msg.topic, "t");
  assert.deepEqual(Array.from(decodeBase64(msg.data!)), Array.from(payload));
});

test("an incoming sample message routes to the matching topic's handler, decoded", () => {
  const { client, socket } = newHarness();
  const received: Uint8Array[] = [];
  client.subscribeRaw("t", (b) => received.push(b));
  socket().emitOpen();

  const payload = new TextEncoder().encode("payload-bytes");
  socket().emitServerJSON({ op: "sample", topic: "t", data: encodeBase64(payload) });

  assert.equal(received.length, 1);
  assert.deepEqual(Array.from(received[0]!), Array.from(payload));
});

test("a sample for an unsubscribed topic is ignored, not delivered to the wrong handler", () => {
  const { client, socket } = newHarness();
  const receivedA: Uint8Array[] = [];
  client.subscribeRaw("a", (b) => receivedA.push(b));
  socket().emitOpen();

  socket().emitServerJSON({ op: "sample", topic: "b", data: encodeBase64(new Uint8Array([1])) });
  assert.equal(receivedA.length, 0);
});

test("authToken is appended as a ?token= query parameter", () => {
  let capturedURL = "";
  new DDSClient("ws://example.invalid/gateway", {
    reconnect: false,
    authToken: "s3cr3t",
    webSocketFactory: (url) => {
      capturedURL = url;
      return new FakeSocket();
    },
  });
  assert.equal(capturedURL, "ws://example.invalid/gateway?token=s3cr3t");
});

test("authToken is appended with & when the URL already has a query string", () => {
  let capturedURL = "";
  new DDSClient("ws://example.invalid/gateway?x=1", {
    reconnect: false,
    authToken: "s3cr3t",
    webSocketFactory: (url) => {
      capturedURL = url;
      return new FakeSocket();
    },
  });
  assert.equal(capturedURL, "ws://example.invalid/gateway?x=1&token=s3cr3t");
});

test("reconnect re-subscribes every still-active topic on the new socket", () => {
  const sockets: FakeSocket[] = [];
  const client = new DDSClient("ws://example.invalid/", {
    reconnect: true,
    reconnectDelayMs: 0,
    webSocketFactory: () => {
      const s = new FakeSocket();
      sockets.push(s);
      return s;
    },
    onError: () => {},
  });
  client.subscribeRaw("t1", () => {});
  client.subscribeRaw("t2", () => {});
  sockets[0]!.emitOpen();
  assert.equal(sockets[0]!.sent.length, 2);

  return new Promise<void>((resolve) => {
    sockets[0]!.close(); // simulate a drop; reconnectDelayMs=0 schedules a reconnect via setTimeout
    setTimeout(() => {
      assert.equal(sockets.length, 2, "expected exactly one reconnect dial");
      sockets[1]!.emitOpen();
      const topics = sockets[1]!.sent.map((raw) => (JSON.parse(raw) as ClientMessage).topic).sort();
      assert.deepEqual(topics, ["t1", "t2"]);
      resolve();
    }, 10);
  });
});

test("close() prevents any reconnect attempt", () => {
  const sockets: FakeSocket[] = [];
  const client = new DDSClient("ws://example.invalid/", {
    reconnect: true,
    reconnectDelayMs: 0,
    webSocketFactory: () => {
      const s = new FakeSocket();
      sockets.push(s);
      return s;
    },
    onError: () => {},
  });
  sockets[0]!.emitOpen();
  client.close();
  assert.equal(sockets[0]!.closeCalls, 1);

  return new Promise<void>((resolve) => {
    setTimeout(() => {
      assert.equal(sockets.length, 1, "close() must prevent a reconnect dial");
      resolve();
    }, 20);
  });
});

// ── TypedPublisher / TypedSubscriber ──────────────────────────────────────────

interface Reading {
  sensor: string;
  celsius: number;
}

test("TypedPublisher/TypedSubscriber round-trip a value through jsonCodec (default)", () => {
  const { client, socket } = newHarness();
  socket().emitOpen();

  const sub = new TypedSubscriber<Reading>(client, "sensors/temp");
  const received: Reading[] = [];
  sub.onMessage((v) => received.push(v));

  const pub = new TypedPublisher<Reading>(client, "sensors/temp");
  pub.publish({ sensor: "cabin", celsius: 21.5 });

  // The publish went out over the wire (base64/JSON); feed it back in as
  // if the bridge had delivered it back to this same client (subscribed to
  // its own publish topic), proving the codec is symmetric end to end.
  const publishMsg = socket().lastSent();
  assert.equal(publishMsg.op, "publish");
  socket().emitServerJSON({ op: "sample", topic: "sensors/temp", data: publishMsg.data });

  assert.equal(received.length, 1);
  assert.deepEqual(received[0], { sensor: "cabin", celsius: 21.5 });
  sub.close();
});

test("TypedSubscriber with an explicit codec (bytesCodec) delivers raw bytes", () => {
  const { client, socket } = newHarness();
  socket().emitOpen();

  const sub = new TypedSubscriber<Uint8Array>(client, "raw/topic", bytesCodec);
  const received: Uint8Array[] = [];
  sub.onMessage((v) => received.push(v));

  const payload = new Uint8Array([9, 8, 7]);
  socket().emitServerJSON({ op: "sample", topic: "raw/topic", data: encodeBase64(payload) });

  assert.equal(received.length, 1);
  assert.deepEqual(Array.from(received[0]!), [9, 8, 7]);
  sub.close();
});

test("TypedSubscriber.close() stops delivery and unsubscribes", () => {
  const { client, socket } = newHarness();
  socket().emitOpen();

  const sub = new TypedSubscriber<Reading>(client, "t");
  const received: Reading[] = [];
  sub.onMessage((v) => received.push(v));
  const sentBeforeClose = socket().sent.length;

  sub.close();
  assert.equal(socket().sent.length, sentBeforeClose + 1);
  assert.deepEqual(socket().lastSent(), { op: "unsubscribe", topic: "t" });

  // A late-arriving sample after close() must not be delivered.
  socket().emitServerJSON({
    op: "sample",
    topic: "t",
    data: encodeBase64(new TextEncoder().encode(JSON.stringify({ sensor: "x", celsius: 1 }))),
  });
  assert.equal(received.length, 0);
});

test("server error messages reach onError", () => {
  const errors: unknown[] = [];
  const socket = new FakeSocket();
  new DDSClient("ws://example.invalid/", {
    reconnect: false,
    webSocketFactory: () => socket,
    onError: (e) => errors.push(e),
  });
  socket.emitOpen();
  socket.emitServerJSON({ op: "error", message: "topic name required" });
  assert.equal(errors.length, 1);
  assert.match(String(errors[0]), /topic name required/);
});
