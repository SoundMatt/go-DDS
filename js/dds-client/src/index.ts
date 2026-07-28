// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

/**
 * @go-dds/dds-client — a JavaScript/TypeScript client for go-DDS's
 * `bridge/ws` WebSocket gateway (Milestone 16, ROADMAP.md "WebSocket
 * Transport", "JavaScript/TypeScript client library (js/dds-client/) —
 * TypedPublisher and TypedSubscriber over WebSocket").
 *
 * This client speaks `bridge/ws`'s small JSON pub/sub protocol (subscribe /
 * unsubscribe / publish, samples pushed back as they arrive) over a single
 * long-lived WebSocket connection — see README.md's "Scope" section for how
 * this relates to `rtps.WithWSAddr`, go-DDS's native RTPS-over-WebSocket
 * transport, which this package does not implement.
 */

import { decodeBase64, encodeBase64 } from "./base64.js";

// ── wire protocol (mirrors bridge/ws's clientMessage/serverMessage) ─────────

interface ClientMessage {
  op: "subscribe" | "unsubscribe" | "publish";
  topic: string;
  data?: string;
}

interface ServerMessage {
  op: "subscribed" | "unsubscribed" | "sample" | "error";
  topic?: string;
  data?: string;
  message?: string;
}

// ── codecs ───────────────────────────────────────────────────────────────────

/** A Codec converts between an application-level value and raw bytes. */
export interface Codec<T> {
  encode(value: T): Uint8Array;
  decode(bytes: Uint8Array): T;
}

/**
 * jsonCodec returns a Codec that JSON-encodes/decodes values as UTF-8 bytes
 * — the default codec TypedPublisher/TypedSubscriber use when none is
 * given, matching bridge/ws's own "JSON ... framing" convention
 * (ROADMAP.md's "JSON and binary (base64-CDR) framing modes" bullet, on the
 * rtps/transport_ws.go side of this milestone).
 */
export function jsonCodec<T>(): Codec<T> {
  const encoder = new TextEncoder();
  const decoder = new TextDecoder();
  return {
    encode: (value: T) => encoder.encode(JSON.stringify(value)),
    decode: (bytes: Uint8Array) => JSON.parse(decoder.decode(bytes)) as T,
  };
}

/** bytesCodec is the identity Codec — for topics whose payload is already raw bytes. */
export const bytesCodec: Codec<Uint8Array> = {
  encode: (value) => value,
  decode: (bytes) => bytes,
};

/** textCodec encodes/decodes plain UTF-8 strings, with no JSON wrapping. */
export const textCodec: Codec<string> = {
  encode: (value) => new TextEncoder().encode(value),
  decode: (bytes) => new TextDecoder().decode(bytes),
};

// ── minimal WebSocket surface ────────────────────────────────────────────────

/**
 * WebSocketLike is the subset of the standard WebSocket API this client
 * depends on — a structural type rather than importing the DOM `WebSocket`
 * type directly, so a caller can supply any compatible implementation (a
 * ponyfill, a test double, a Node "ws" package instance, ...) via
 * DDSClientOptions.webSocketFactory without this package needing to depend
 * on `@types/node` or `@types/ws`.
 */
export interface WebSocketLike {
  readyState: number;
  send(data: string): void;
  close(code?: number, reason?: string): void;
  onopen: ((this: WebSocketLike, ev: unknown) => void) | null;
  onmessage: ((this: WebSocketLike, ev: { data: unknown }) => void) | null;
  onclose: ((this: WebSocketLike, ev: unknown) => void) | null;
  onerror: ((this: WebSocketLike, ev: unknown) => void) | null;
}

/** The four WebSocket readyState values (WHATWG WebSocket spec §4.3). */
const WS_OPEN = 1;

export type WebSocketFactory = (url: string) => WebSocketLike;

function defaultWebSocketFactory(url: string): WebSocketLike {
  const ctor = (globalThis as { WebSocket?: new (url: string) => WebSocketLike }).WebSocket;
  if (!ctor) {
    throw new Error(
      "@go-dds/dds-client: no global WebSocket constructor found in this runtime; " +
        "pass options.webSocketFactory explicitly (e.g. wrapping Node's \"ws\" package)",
    );
  }
  return new ctor(url);
}

// ── DDSClient ────────────────────────────────────────────────────────────────

export interface DDSClientOptions {
  /**
   * AuthToken, if set, authenticates against a bridge/ws Bridge configured
   * with a matching Options.AuthToken. Because the standard WebSocket API
   * gives page JavaScript no way to set an Authorization header on the
   * opening handshake, this is sent as a "?token=" query parameter — the
   * browser-compatible fallback bridge/ws's Bridge.authorize also accepts
   * (see its doc comment); a non-browser client (Node, a cloud function)
   * that wants the header form instead can dial the header-based bridge
   * directly without this client.
   */
  authToken?: string;
  /** Automatically reconnect (with reconnectDelayMs between attempts) after the connection drops. Default: true. */
  reconnect?: boolean;
  /** Delay between reconnect attempts. Default: 2000ms. */
  reconnectDelayMs?: number;
  /** Overrides how the underlying WebSocket is constructed — see WebSocketLike. */
  webSocketFactory?: WebSocketFactory;
  /** Called with every protocol-level error this client cannot otherwise report (malformed server message, {"op":"error",...} without a topic-scoped subscriber to hand it to, ...). */
  onError?: (err: unknown) => void;
}

type RawHandler = (bytes: Uint8Array) => void;

/**
 * DDSClient owns one WebSocket connection to a bridge/ws gateway. Most
 * callers should use TypedPublisher/TypedSubscriber instead of this
 * class's own subscribeRaw/publishRaw directly — those wrap a Codec around
 * exactly the same connection.
 */
export class DDSClient {
  private readonly url: string;
  private readonly factory: WebSocketFactory;
  private readonly reconnect: boolean;
  private readonly reconnectDelayMs: number;
  private readonly onError: (err: unknown) => void;

  private ws: WebSocketLike | null = null;
  private closedByCaller = false;
  private outbox: string[] = [];
  private readonly subscriptions = new Map<string, Set<RawHandler>>();

  constructor(url: string, options: DDSClientOptions = {}) {
    this.factory = options.webSocketFactory ?? defaultWebSocketFactory;
    this.reconnect = options.reconnect ?? true;
    this.reconnectDelayMs = options.reconnectDelayMs ?? 2000;
    this.onError = options.onError ?? ((err) => console.error("@go-dds/dds-client:", err));
    this.url = appendToken(url, options.authToken);
    this.connect();
  }

  private connect(): void {
    const ws = this.factory(this.url);
    this.ws = ws;

    ws.onopen = () => {
      // Flush anything queued while disconnected (publishes only — see
      // publishRaw/sendOrQueue), then (re-)establish every still-active
      // subscription by sending "subscribe" for each topic currently held
      // in `subscriptions`. This single loop is what both the very first
      // connect and every later reconnect rely on — subscribeRaw/
      // unsubscribeRaw never queue a subscribe/unsubscribe frame
      // themselves (see sendIfOpen), specifically so this is the *only*
      // place a "subscribe" frame is ever sent for a topic, and a topic
      // can never be double-subscribed by both a queued frame and this
      // loop.
      const queued = this.outbox;
      this.outbox = [];
      for (const frame of queued) ws.send(frame);
      for (const topic of this.subscriptions.keys()) {
        ws.send(JSON.stringify({ op: "subscribe", topic } satisfies ClientMessage));
      }
    };

    ws.onmessage = (ev) => {
      if (typeof ev.data !== "string") {
        // This client's own protocol is JSON-over-TEXT-frames only (see
        // bridge/ws's writeMessage, which always sends TEXT); a non-string
        // payload means either a misconfigured server or a different
        // protocol entirely, either way not something to try to parse.
        this.onError(new Error("@go-dds/dds-client: received a non-text WebSocket message"));
        return;
      }
      let msg: ServerMessage;
      try {
        msg = JSON.parse(ev.data) as ServerMessage;
      } catch (err) {
        this.onError(err);
        return;
      }
      this.handleServerMessage(msg);
    };

    ws.onclose = () => {
      this.ws = null;
      if (!this.closedByCaller && this.reconnect) {
        setTimeout(() => {
          if (!this.closedByCaller) this.connect();
        }, this.reconnectDelayMs);
      }
    };

    ws.onerror = (ev) => this.onError(ev);
  }

  private handleServerMessage(msg: ServerMessage): void {
    switch (msg.op) {
      case "sample": {
        if (!msg.topic || msg.data === undefined) return;
        const handlers = this.subscriptions.get(msg.topic);
        if (!handlers || handlers.size === 0) return;
        const bytes = decodeBase64(msg.data);
        for (const handler of handlers) {
          try {
            handler(bytes);
          } catch (err) {
            this.onError(err);
          }
        }
        return;
      }
      case "error":
        this.onError(new Error(`@go-dds/dds-client: server error: ${msg.message ?? "(no message)"}`));
        return;
      case "subscribed":
      case "unsubscribed":
        return; // purely informational acks; subscribeRaw/unsubscribe already updated local state optimistically
      default:
        this.onError(new Error(`@go-dds/dds-client: unknown server message op ${JSON.stringify(msg)}`));
    }
  }

  /**
   * sendIfOpen sends msg immediately if the connection is currently open,
   * or does nothing otherwise. It is only ever used for subscribe/
   * unsubscribe frames, whose state is authoritatively derived from
   * `subscriptions` and re-sent in full by connect's onopen handler on
   * every (re)connect — so a subscribe/unsubscribe call made while
   * disconnected needs no queue of its own; it is simply picked up the
   * next time onopen runs. This is what keeps a topic from ever being
   * subscribed twice (once from a queue, once from onopen's own resend
   * loop).
   */
  private sendIfOpen(msg: ClientMessage): void {
    if (this.ws && this.ws.readyState === WS_OPEN) {
      this.ws.send(JSON.stringify(msg));
    }
  }

  /**
   * subscribeRaw registers handler to be called with the raw payload bytes
   * of every sample delivered on topic, sending a "subscribe" request for
   * topic on first use if the connection is currently open (and, either
   * way, automatically on every future (re)connect — see connect's onopen
   * handler). Returns a function that unregisters handler; when it was the
   * last handler for topic, this also sends an "unsubscribe" request (if
   * currently open).
   */
  subscribeRaw(topic: string, handler: RawHandler): () => void {
    let handlers = this.subscriptions.get(topic);
    const isFirst = !handlers;
    if (!handlers) {
      handlers = new Set();
      this.subscriptions.set(topic, handlers);
    }
    handlers.add(handler);
    if (isFirst) {
      this.sendIfOpen({ op: "subscribe", topic });
    }

    let unsubscribed = false;
    return () => {
      if (unsubscribed) return;
      unsubscribed = true;
      const current = this.subscriptions.get(topic);
      if (!current) return;
      current.delete(handler);
      if (current.size === 0) {
        this.subscriptions.delete(topic);
        this.sendIfOpen({ op: "unsubscribe", topic });
      }
    };
  }

  /**
   * publishRaw sends payload as a sample on topic — immediately if the
   * connection is open, or queued to be flushed (in order) once it opens
   * otherwise. Unlike subscribe/unsubscribe, a publish is a one-shot
   * action with no persistent state to re-derive it from later, so it must
   * be queued explicitly to survive a not-yet-open (or momentarily
   * reconnecting) connection.
   */
  publishRaw(topic: string, payload: Uint8Array): void {
    const frame = JSON.stringify({ op: "publish", topic, data: encodeBase64(payload) } satisfies ClientMessage);
    if (this.ws && this.ws.readyState === WS_OPEN) {
      this.ws.send(frame);
    } else {
      this.outbox.push(frame);
    }
  }

  /** close permanently closes the connection; no further reconnect attempts are made. */
  close(): void {
    this.closedByCaller = true;
    this.ws?.close();
    this.ws = null;
  }
}

function appendToken(url: string, token: string | undefined): string {
  if (!token) return url;
  const separator = url.includes("?") ? "&" : "?";
  return `${url}${separator}token=${encodeURIComponent(token)}`;
}

// ── TypedPublisher / TypedSubscriber ────────────────────────────────────────

/**
 * TypedPublisher publishes typed values to one topic on client, encoding
 * each with codec (default: jsonCodec) before sending — ROADMAP.md's
 * "TypedPublisher ... over WebSocket".
 */
export class TypedPublisher<T> {
  constructor(
    private readonly client: DDSClient,
    private readonly topic: string,
    private readonly codec: Codec<T> = jsonCodec<T>(),
  ) {}

  /** publish encodes value with this publisher's codec and sends it on its topic. */
  publish(value: T): void {
    this.client.publishRaw(this.topic, this.codec.encode(value));
  }
}

/**
 * TypedSubscriber delivers typed values received on one topic, decoding
 * each with codec (default: jsonCodec) — ROADMAP.md's "... and
 * TypedSubscriber over WebSocket". Register a handler with onMessage;
 * multiple handlers on the same TypedSubscriber all receive every sample.
 */
export class TypedSubscriber<T> {
  private readonly handlers = new Set<(value: T) => void>();
  private readonly unsubscribeRaw: () => void;
  private closed = false;

  constructor(
    client: DDSClient,
    private readonly topic: string,
    private readonly codec: Codec<T> = jsonCodec<T>(),
  ) {
    this.unsubscribeRaw = client.subscribeRaw(topic, (bytes) => {
      let value: T;
      try {
        value = this.codec.decode(bytes);
      } catch {
        return; // a malformed/undecodable sample is dropped, not delivered
      }
      for (const handler of this.handlers) handler(value);
    });
  }

  /** onMessage registers handler for every future sample; returns a function that removes it. */
  onMessage(handler: (value: T) => void): () => void {
    this.handlers.add(handler);
    return () => this.handlers.delete(handler);
  }

  /** close stops delivery and unsubscribes from the topic (see DDSClient.subscribeRaw). */
  close(): void {
    if (this.closed) return;
    this.closed = true;
    this.handlers.clear();
    this.unsubscribeRaw();
  }

  /** the topic this subscriber was created for. */
  get topicName(): string {
    return this.topic;
  }
}

export { decodeBase64, encodeBase64 } from "./base64.js";
