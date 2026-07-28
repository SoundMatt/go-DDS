// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

//go:build js && wasm

package rtps

// The browser RTPS-over-WebSocket client-dial backend (Milestone 16,
// ROADMAP.md "WebAssembly Target") — the piece that lets a go-DDS build
// compiled with `GOOS=js GOARCH=wasm go build` and running in an actual
// browser tab (see examples/wasm-subscriber/) join a DDS domain as a
// genuine RTPS participant over WithWSPeers, not through bridge/ws's
// separate JSON gateway.
//
// transport_ws_dial.go's default dial backend needs a real net.Conn, which
// this platform cannot provide: the standard library's net package for
// GOOS=js is, in its own words, "fake networking for js/wasm... intended to
// allow tests of other packages to pass" ($GOROOT/src/net/fd_js.go) — an
// in-process loopback simulation with no path to an actual remote host, not
// a real network stack, because a browser sandbox simply does not expose
// raw TCP sockets to any script running inside it, Wasm or otherwise. The
// only network primitive a browser page actually has for this purpose is
// its own native WebSocket object, already reachable from Go via
// syscall/js, and already RFC 6455-compliant (opening handshake, masking,
// fragmentation, ping/pong — all handled by the browser itself, invisibly
// to this code): browserWSConn wraps exactly that object instead of a
// net.Conn, so wsSocket.dial here does no handshake or frame-codec work of
// its own at all — see readMessage/writeMessage below, which operate at
// the whole-message level the browser's `send`/`message` event pair
// already gives us, unlike wsConn's byte-level readWSFrame/writeWSFrame.
import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"syscall/js"
	"time"
)

// errBrowserWSClosed is returned by readMessage/writeMessage once the
// underlying browser WebSocket has closed (locally or by the peer) or
// never finished opening.
var errBrowserWSClosed = errors.New("rtps: browser WebSocket connection closed")

// wsBrowserFrame is one decoded inbound browser WebSocket message, queued
// by the onmessage callback for readMessage to hand to wsSocket.readLoop —
// mirrors wsPacket's role one level down.
type wsBrowserFrame struct {
	opcode  byte // wsOpText or wsOpBinary; the browser never surfaces Ping/Pong/Close as a "message" event
	payload []byte
}

// browserWSConn is the GOOS=js GOARCH=wasm wsConnIface backend: a thin
// wrapper around a browser `WebSocket` object obtained via syscall/js. It
// is always a client-role connection — a browser page can never accept an
// inbound connection, so this backend only ever exists via wsSocket.dial,
// never wsSocket.handleAccept (which, on this platform, is simply
// unreachable dead code: newWSSocket is only ever called with addr == ""
// here — see transport_ws.go's doc comment on dial-only sockets).
type browserWSConn struct {
	ws js.Value

	recv   chan wsBrowserFrame
	closed chan struct{}
	once   sync.Once
	sendMu sync.Mutex

	onMessage js.Func
	onError   js.Func
	onClose   js.Func
}

var _ wsConnIface = (*browserWSConn)(nil)

// dial opens a real browser WebSocket connection to addr (a "host:port"
// address, exactly as every other wsSocket.dial backend takes) via
// syscall/js, waiting up to wsDialTimeout for the browser to report the
// connection open (or fail). Unlike transport_ws_dial.go's dial, there is
// no separate handshake step to perform afterward — the browser's own
// WebSocket implementation has already completed the RFC 6455 opening
// handshake by the time its "open" event fires.
func (s *wsSocket) dial(addr string) (wsConnIface, error) {
	wsClass := js.Global().Get("WebSocket")
	if wsClass.IsUndefined() {
		return nil, fmt.Errorf("rtps: WS dial %s: no WebSocket constructor in this JS environment", addr)
	}
	url := wsBrowserURL(addr, s.tlsConfig != nil)

	bc := &browserWSConn{
		ws:     wsClass.New(url),
		recv:   make(chan wsBrowserFrame, 256),
		closed: make(chan struct{}),
	}
	bc.ws.Set("binaryType", "arraybuffer")
	bc.onMessage = js.FuncOf(bc.handleMessage)
	bc.onError = js.FuncOf(bc.handleErrorOrClose)
	bc.onClose = js.FuncOf(bc.handleErrorOrClose)
	bc.ws.Call("addEventListener", "message", bc.onMessage)
	bc.ws.Call("addEventListener", "error", bc.onError)
	bc.ws.Call("addEventListener", "close", bc.onClose)

	opened := make(chan struct{})
	var onOpen js.Func
	onOpen = js.FuncOf(func(this js.Value, args []js.Value) any {
		select {
		case <-opened:
		default:
			close(opened)
		}
		return nil
	})
	defer onOpen.Release()
	bc.ws.Call("addEventListener", "open", onOpen)

	select {
	case <-opened:
		return bc, nil
	case <-bc.closed:
		return nil, fmt.Errorf("rtps: WS dial %s: browser WebSocket closed before opening", addr)
	case <-time.After(wsDialTimeout):
		_ = bc.close()
		return nil, fmt.Errorf("rtps: WS dial %s: timed out waiting for browser WebSocket to open", addr)
	}
}

// handleMessage is the browser "message" event listener: it decodes the
// event's data — a JS string for a TEXT frame (browser's binaryType only
// governs binary frames) or an ArrayBuffer for a BINARY frame, per the
// WebSocket API — into a wsBrowserFrame and queues it for readMessage,
// dropping it if maxWSFrameSize is exceeded or the queue is backed up (the
// same hostile/slow-peer protection readWSFrame/readLoop give the default
// backend).
func (c *browserWSConn) handleMessage(_ js.Value, args []js.Value) any {
	if len(args) == 0 {
		return nil
	}
	data := args[0].Get("data")
	var frame wsBrowserFrame
	if data.Type() == js.TypeString {
		s := data.String()
		if len(s) > maxWSFrameSize {
			go func() { _ = c.close() }()
			return nil
		}
		frame = wsBrowserFrame{opcode: wsOpText, payload: []byte(s)}
	} else {
		n := data.Get("byteLength").Int()
		if n > maxWSFrameSize {
			go func() { _ = c.close() }()
			return nil
		}
		buf := make([]byte, n)
		js.CopyBytesToGo(buf, js.Global().Get("Uint8Array").New(data))
		frame = wsBrowserFrame{opcode: wsOpBinary, payload: buf}
	}
	select {
	case c.recv <- frame:
	case <-c.closed:
	default: // slow consumer; drop, mirroring wsSocket.readLoop's own policy
	}
	return nil
}

// handleErrorOrClose is the browser "error"/"close" event listener: either
// event means this connection is no longer usable, so it tears the
// connection down exactly as an explicit close() call would. Runs the
// actual teardown on a fresh goroutine rather than inline because close()
// releases this callback's own js.Func, which must not happen while the JS
// engine is still on the stack frame that invoked it.
func (c *browserWSConn) handleErrorOrClose(js.Value, []js.Value) any {
	go func() { _ = c.close() }()
	return nil
}

// readMessage implements wsConnIface: it returns the next queued inbound
// message, or errBrowserWSClosed once the connection has closed.
func (c *browserWSConn) readMessage() (opcode byte, payload []byte, err error) {
	select {
	case f := <-c.recv:
		return f.opcode, f.payload, nil
	case <-c.closed:
		return 0, nil, errBrowserWSClosed
	}
}

// writeMessage implements wsConnIface: unlike wsConn.writeMessage, there is
// no RFC 6455 frame to build by hand — `WebSocket.send` already frames
// (and, per spec, masks — a browser page is always the client role) a
// string argument as a TEXT frame and a binary argument (here, a
// Uint8Array) as a BINARY frame.
func (c *browserWSConn) writeMessage(framing wsFraming, data []byte) error {
	c.sendMu.Lock()
	defer c.sendMu.Unlock()
	select {
	case <-c.closed:
		return errBrowserWSClosed
	default:
	}
	if framing == wsFramingJSON {
		payload, err := json.Marshal(wsJSONMessage{Data: base64.StdEncoding.EncodeToString(data)})
		if err != nil {
			return err
		}
		c.ws.Call("send", string(payload))
		return nil
	}
	arr := js.Global().Get("Uint8Array").New(len(data))
	js.CopyBytesToJS(arr, data)
	c.ws.Call("send", arr)
	return nil
}

// close implements wsConnIface: closes the browser WebSocket and releases
// every js.Func callback registered on it. Idempotent — safe to call from
// readLoop's own teardown, from handleErrorOrClose, and from dial's own
// timeout path, in any order or combination.
func (c *browserWSConn) close() error {
	c.once.Do(func() {
		close(c.closed)
		c.ws.Call("close")
		c.onMessage.Release()
		c.onError.Release()
		c.onClose.Release()
	})
	return nil
}
