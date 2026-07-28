// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

//go:build js && wasm

// Command wasm-subscriber is a browser-native DDS subscriber (ROADMAP.md
// Milestone 16, "WebAssembly Target"): compiled with `GOOS=js GOARCH=wasm
// go build` and run inside index.html via wasm_exec.js, it is a genuine
// RTPS participant dialling out over the RTPS-over-WebSocket transport
// (rtps.WithWSPeers, backed in a real browser by
// rtps/transport_ws_browser.go's syscall/js WebSocket wrapper) directly
// against a real go-DDS domain — performing its own SPDP/SEDP discovery
// exactly as any other go-DDS participant does — not bridge/ws's separate
// JSON gateway (see js/dds-client for that path). This is what Milestone
// 16's success criterion means by "A browser tab ... can join a DDS domain
// alongside embedded devices without a protocol bridge."
//
// It never binds rtps.WithWSAddr: a browser tab can never accept an
// inbound connection, so it relies entirely on WithWSPeers' dial-only mode
// (see that option's doc comment in rtps/participant.go) — the connection
// it dials out to the go-DDS peer named by its "ws" config value is also
// how every reply, HEARTBEAT, and further SEDP/user-data message from that
// peer reaches it back.
//
// Build (from the examples/ module directory):
//
//	GOOS=js GOARCH=wasm go build -o wasm-subscriber/main.wasm ./wasm-subscriber
//
// wasm-subscriber/build.sh does this plus copies the matching wasm_exec.js
// glue from the local Go toolchain (its content is tied to the exact Go
// version used to build main.wasm, so this repo does not vendor a copy).
// See this directory's README.md for the full build/serve/run walkthrough
// and index.html for the served page.
//
// TinyGo also targets GOOS=js GOARCH=wasm (`tinygo build -o main.wasm
// -target wasm ./wasm-subscriber`) for a materially smaller binary —
// nothing in this file or rtps/transport_ws_browser.go is
// standard-Go-toolchain-specific — but only the `go build` path above is
// exercised by this repo's own CI (.github/workflows/ci.yml's wasm-build
// job); see the README for that tradeoff.
package main

import (
	"crypto/tls"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"syscall/js"

	dds "github.com/SoundMatt/go-DDS"
	"github.com/SoundMatt/go-DDS/rtps"
)

// config holds this run's settings, read from the page URL's query string
// by loadConfig — see this file's doc comment for the query parameters.
type config struct {
	wsAddr string // "host:port" of the go-DDS peer to dial (rtps.WithWSPeers)
	tls    bool   // wss:// instead of ws://
	domain int    // DDS domain ID
	topic  string // topic name to subscribe to
}

func defaultConfig() config {
	return config{
		wsAddr: "127.0.0.1:7800",
		tls:    false,
		domain: 0,
		topic:  "vehicle/speed",
	}
}

// loadConfig reads window.location.search (e.g.
// "?ws=example.com:7800&tls=1&domain=3&topic=sensors/temp") and overrides
// defaultConfig's values with whatever it finds, so this page can be
// pointed at any go-DDS domain/topic without a rebuild.
func loadConfig() config {
	cfg := defaultConfig()
	search := js.Global().Get("location").Get("search").String()
	values, err := url.ParseQuery(strings.TrimPrefix(search, "?"))
	if err != nil {
		return cfg
	}
	if v := values.Get("ws"); v != "" {
		cfg.wsAddr = v
	}
	if v := values.Get("tls"); v == "1" || v == "true" {
		cfg.tls = true
	}
	if v := values.Get("domain"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.domain = n
		}
	}
	if v := values.Get("topic"); v != "" {
		cfg.topic = v
	}
	return cfg
}

func main() {
	cfg := loadConfig()
	logLine(fmt.Sprintf("go-DDS wasm-subscriber: ws=%s tls=%v domain=%d topic=%q",
		cfg.wsAddr, cfg.tls, cfg.domain, cfg.topic))
	setStatus("connecting")

	opts := []rtps.Option{
		rtps.WithWSPeers(cfg.wsAddr),
	}
	if cfg.tls {
		// See rtps/transport_ws_browser.go: on this platform the *tls.Config
		// itself is never read for TLS parameters (the browser's own
		// WebSocket implementation handles TLS entirely on its own) — only
		// its non-nil-ness is consulted, to choose the wss:// URL scheme.
		opts = append(opts, rtps.WithWSTLSConfig(&tls.Config{}))
	}

	p, err := rtps.New(dds.Domain(cfg.domain), opts...)
	if err != nil {
		logLine("participant start failed: " + err.Error())
		setStatus("error: " + err.Error())
		return
	}

	sub, err := p.NewSubscriber(cfg.topic, dds.DefaultQoS)
	if err != nil {
		logLine("NewSubscriber failed: " + err.Error())
		setStatus("error: " + err.Error())
		return
	}
	setStatus("subscribed")
	logLine("subscribed to " + cfg.topic + " — waiting for samples")

	for sample := range sub.C() {
		appendSample(string(sample.Payload))
	}
}

// ── DOM glue (syscall/js) ────────────────────────────────────────────────

func doc() js.Value { return js.Global().Get("document") }

// logLine writes s to the browser console, exactly like a normal Go
// program's log output would if it had a terminal.
func logLine(s string) {
	js.Global().Get("console").Call("log", "[wasm-subscriber] "+s)
}

// setStatus updates index.html's #status element, if present, so the page
// itself reflects connection state without needing the browser console
// open.
func setStatus(s string) {
	el := doc().Call("getElementById", "status")
	if el.IsNull() || el.IsUndefined() {
		return
	}
	el.Set("textContent", s)
}

// appendSample logs a received sample and, if index.html defines a
// #samples list element, prepends a new entry for it — capped at 50
// visible entries so a fast publisher cannot grow the DOM unboundedly.
func appendSample(payload string) {
	logLine("sample: " + payload)
	list := doc().Call("getElementById", "samples")
	if list.IsNull() || list.IsUndefined() {
		return
	}
	item := doc().Call("createElement", "li")
	item.Set("textContent", payload)
	list.Call("insertBefore", item, list.Get("firstChild"))
	for list.Get("children").Get("length").Int() > 50 {
		list.Call("removeChild", list.Get("lastChild"))
	}
}
