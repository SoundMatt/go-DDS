// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package rest_test

//fusa:test REQ-BRIDGE-003
//fusa:test REQ-BRIDGE-004
//fusa:test REQ-BRIDGE-005

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	dds "github.com/SoundMatt/go-DDS"
	"github.com/SoundMatt/go-DDS/bridge/rest"
	"github.com/SoundMatt/go-DDS/mock"
)

func newBridgeServer(t *testing.T, opts rest.Options) (*rest.Bridge, *httptest.Server) {
	t.Helper()
	p, err := mock.New(dds.Domain(0))
	if err != nil {
		t.Fatalf("mock.New: %v", err)
	}
	b := rest.New(p, opts)
	srv := httptest.NewServer(b)
	t.Cleanup(func() {
		srv.Close()
		_ = b.Close()
		_ = p.Close()
	})
	return b, srv
}

func doGet(t *testing.T, url string) *http.Response {
	t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("NewRequestWithContext: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	return resp
}

func doPost(t *testing.T, url, body string) *http.Response {
	t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, url,
		strings.NewReader(body))
	if err != nil {
		t.Fatalf("NewRequestWithContext: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST %s: %v", url, err)
	}
	return resp
}

// ── GET /topics ───────────────────────────────────────────────────────────────

func TestBridge_ListTopics_Empty(t *testing.T) {
	ignoredRet, srv := newBridgeServer(t, rest.Options{})
	_ = ignoredRet
	resp := doGet(t, srv.URL+"/topics")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d", resp.StatusCode)
	}
	var topics []string
	if err := json.NewDecoder(resp.Body).Decode(&topics); err != nil {
		t.Fatal(err)
	}
	if len(topics) != 0 {
		t.Errorf("expected empty list, got %v", topics)
	}
}

func TestBridge_ListTopics_AfterSubscribe(t *testing.T) {
	ignoredRet, srv := newBridgeServer(t, rest.Options{})
	_ = ignoredRet

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/topics/sensors/temp", nil)
	if err != nil {
		t.Fatalf("NewRequestWithContext: %v", err)
	}
	go func() {
		resp, err := http.DefaultClient.Do(req)
		if err == nil {
			resp.Body.Close()
		}
	}()
	time.Sleep(50 * time.Millisecond)

	resp := doGet(t, srv.URL+"/topics")
	defer resp.Body.Close()
	var topics []string
	_ = json.NewDecoder(resp.Body).Decode(&topics)
	found := false
	for _, tpc := range topics {
		if tpc == "sensors/temp" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected sensors/temp in list, got %v", topics)
	}
}

// ── POST /topics/{t} ─────────────────────────────────────────────────────────

func TestBridge_Publish_ReturnsNoContent(t *testing.T) {
	ignoredRet, srv := newBridgeServer(t, rest.Options{})
	_ = ignoredRet
	resp := doPost(t, srv.URL+"/topics/test/data", "hello-dds")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", resp.StatusCode)
	}
}

func TestBridge_Publish_DeliveredToSubscriber(t *testing.T) {
	ignoredRet, srv := newBridgeServer(t, rest.Options{})
	_ = ignoredRet

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/topics/notify", nil)
	if err != nil {
		t.Fatalf("NewRequestWithContext: %v", err)
	}
	sseResp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer sseResp.Body.Close()

	time.Sleep(30 * time.Millisecond)

	payload := "round-trip"
	pubReq, err := http.NewRequestWithContext(context.Background(), http.MethodPost,
		srv.URL+"/topics/notify", strings.NewReader(payload))
	if err != nil {
		t.Fatalf("NewRequestWithContext: %v", err)
	}
	pubResp, err := http.DefaultClient.Do(pubReq)
	if err != nil {
		t.Fatal(err)
	}
	pubResp.Body.Close()

	scanner := bufio.NewScanner(sseResp.Body)
	var dataLine string
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "data: ") {
			dataLine = strings.TrimPrefix(line, "data: ")
			break
		}
	}
	if dataLine == "" {
		t.Fatal("no data line received in SSE stream")
	}
	decoded, err := base64.StdEncoding.DecodeString(dataLine)
	if err != nil {
		t.Fatalf("base64 decode: %v", err)
	}
	if string(decoded) != payload {
		t.Errorf("payload: got %q, want %q", decoded, payload)
	}
}

// ── SSE stream structure ──────────────────────────────────────────────────────

func TestBridge_SSE_EventFormat(t *testing.T) {
	ignoredRet, srv := newBridgeServer(t, rest.Options{})
	_ = ignoredRet

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/topics/fmt", nil)
	if err != nil {
		t.Fatalf("NewRequestWithContext: %v", err)
	}
	sseResp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer sseResp.Body.Close()

	if ct := sseResp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Errorf("Content-Type: %q", ct)
	}

	time.Sleep(20 * time.Millisecond)

	postReq, err := http.NewRequestWithContext(context.Background(), http.MethodPost,
		srv.URL+"/topics/fmt", strings.NewReader("x"))
	if err != nil {
		t.Fatalf("NewRequestWithContext: %v", err)
	}
	postResp, err := http.DefaultClient.Do(postReq)
	if err != nil {
		t.Fatal(err)
	}
	postResp.Body.Close()

	scanner := bufio.NewScanner(sseResp.Body)
	lines := map[string]bool{}
	for scanner.Scan() {
		l := scanner.Text()
		if l == "" {
			break
		}
		if strings.HasPrefix(l, "id: ") {
			lines["id"] = true
		}
		if strings.HasPrefix(l, "event: message") {
			lines["event"] = true
		}
		if strings.HasPrefix(l, "data: ") {
			lines["data"] = true
		}
	}
	for _, field := range []string{"id", "event", "data"} {
		if !lines[field] {
			t.Errorf("SSE event missing %q field", field)
		}
	}
}

// ── Auth ──────────────────────────────────────────────────────────────────────

func TestBridge_Auth_MissingToken(t *testing.T) {
	ignoredRet, srv := newBridgeServer(t, rest.Options{AuthToken: "secret"})
	_ = ignoredRet
	resp := doGet(t, srv.URL+"/topics")
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", resp.StatusCode)
	}
}

func TestBridge_Auth_WrongToken(t *testing.T) {
	ignoredRet, srv := newBridgeServer(t, rest.Options{AuthToken: "secret"})
	_ = ignoredRet
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL+"/topics", nil)
	if err != nil {
		t.Fatalf("NewRequestWithContext: %v", err)
	}
	req.Header.Set("Authorization", "Bearer wrong")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", resp.StatusCode)
	}
}

func TestBridge_Auth_CorrectToken(t *testing.T) {
	ignoredRet, srv := newBridgeServer(t, rest.Options{AuthToken: "secret"})
	_ = ignoredRet
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL+"/topics", nil)
	if err != nil {
		t.Fatalf("NewRequestWithContext: %v", err)
	}
	req.Header.Set("Authorization", "Bearer secret")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
}

// ── Method not allowed ────────────────────────────────────────────────────────

func TestBridge_MethodNotAllowed_OnList(t *testing.T) {
	ignoredRet, srv := newBridgeServer(t, rest.Options{})
	_ = ignoredRet
	req, err := http.NewRequestWithContext(context.Background(), http.MethodDelete, srv.URL+"/topics", nil)
	if err != nil {
		t.Fatalf("NewRequestWithContext: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", resp.StatusCode)
	}
}

func TestBridge_MethodNotAllowed_OnTopic(t *testing.T) {
	ignoredRet, srv := newBridgeServer(t, rest.Options{})
	_ = ignoredRet
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPut, srv.URL+"/topics/foo", nil)
	if err != nil {
		t.Fatalf("NewRequestWithContext: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", resp.StatusCode)
	}
}

// ── Close ─────────────────────────────────────────────────────────────────────

func TestBridge_Close_Idempotent(t *testing.T) {
	p, err := mock.New(dds.Domain(0))
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	b := rest.New(p, rest.Options{})
	if err := b.Close(); err != nil {
		t.Errorf("first Close: %v", err)
	}
	if err := b.Close(); err != nil {
		t.Errorf("second Close: %v", err)
	}
}

// ── Error paths ───────────────────────────────────────────────────────────────

// TestBridge_Publish_PublisherCreateError covers the handlePublish path where
// getOrCreatePub fails because the participant has been closed.
func TestBridge_Publish_PublisherCreateError(t *testing.T) {
	p, err := mock.New(dds.Domain(0))
	if err != nil {
		t.Fatal(err)
	}
	b := rest.New(p, rest.Options{})
	srv := httptest.NewServer(b)
	t.Cleanup(func() { srv.Close(); b.Close() })

	p.Close() // close before any publish so NewPublisher fails

	resp := doPost(t, srv.URL+"/topics/err/pub", "hello")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", resp.StatusCode)
	}
}

// TestBridge_Publish_WriteCtxError covers the handlePublish path where
// WriteCtx fails due to MaxSampleSize QoS rejection. This also exercises
// the Options.qos() non-default branch.
func TestBridge_Publish_WriteCtxError(t *testing.T) {
	opts := rest.Options{QoS: dds.QoS{MaxSampleSize: 1}}
	ignoredRet, srv := newBridgeServer(t, opts)
	_ = ignoredRet

	resp := doPost(t, srv.URL+"/topics/err/write", "hi") // 2 bytes > MaxSampleSize 1
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", resp.StatusCode)
	}
}

// TestBridge_Subscribe_SubscriberCreateError covers the handleSubscribe path
// where getOrCreateSub fails because the participant has been closed.
func TestBridge_Subscribe_SubscriberCreateError(t *testing.T) {
	p, err := mock.New(dds.Domain(0))
	if err != nil {
		t.Fatal(err)
	}
	b := rest.New(p, rest.Options{})
	srv := httptest.NewServer(b)
	t.Cleanup(func() { srv.Close(); b.Close() })

	p.Close() // close before subscribe so NewSubscriber fails

	resp := doGet(t, srv.URL+"/topics/err/sub")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", resp.StatusCode)
	}
}

// TestBridge_Subscribe_Keepalive_FiresComment covers the keepalive case in
// handleSubscribe and the Options.keepalive() non-default branch.
func TestBridge_Subscribe_Keepalive_FiresComment(t *testing.T) {
	opts := rest.Options{SSEKeepalive: 5 * time.Millisecond}
	ignoredRet, srv := newBridgeServer(t, opts)
	_ = ignoredRet

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/topics/ka/test", nil)
	if err != nil {
		t.Fatalf("NewRequestWithContext: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		if strings.HasPrefix(scanner.Text(), ": keepalive") {
			return // keepalive comment confirmed
		}
	}
	t.Error("no keepalive comment received in SSE stream within timeout")
}

// ── handleSubscribe: non-Flusher ResponseWriter ───────────────────────────────

// noFlusher wraps a ResponseWriter to hide http.Flusher, triggering the
// "streaming not supported" branch in handleSubscribe.
type noFlusher struct{ http.ResponseWriter }

// TestBridge_Subscribe_NotFlusher covers the "streaming not supported" branch
// in handleSubscribe when the ResponseWriter does not implement http.Flusher.
func TestBridge_Subscribe_NotFlusher(t *testing.T) {
	p, err := mock.New(dds.Domain(0))
	if err != nil {
		t.Fatalf("mock.New: %v", err)
	}
	defer p.Close()
	b := rest.New(p, rest.Options{})
	defer b.Close()

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "/topics/no-flush", nil)
	rw := httptest.NewRecorder()
	b.ServeHTTP(&noFlusher{rw}, req)
	if rw.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rw.Code)
	}
}

// ── handlePublish: body read error ───────────────────────────────────────────

// errReader always returns an error on Read, simulating a broken request body.
type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }
func (errReader) Close() error             { return nil }

// TestBridge_Publish_BodyReadError covers the io.ReadAll error branch in
// handlePublish when the request body returns an error.
func TestBridge_Publish_BodyReadError(t *testing.T) {
	p, err := mock.New(dds.Domain(0))
	if err != nil {
		t.Fatalf("mock.New: %v", err)
	}
	defer p.Close()
	b := rest.New(p, rest.Options{})
	defer b.Close()

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, "/topics/body-err", errReader{})
	rw := httptest.NewRecorder()
	b.ServeHTTP(rw, req)
	if rw.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rw.Code)
	}
}

// ── getOrCreateSub cache hit ──────────────────────────────────────────────────

// TestBridge_Subscribe_SameTopicCache verifies that subscribing to the same
// topic twice reuses the cached subscriber, covering the cache-hit branch in
// Bridge.getOrCreateSub.
func TestBridge_Subscribe_SameTopicCache(t *testing.T) {
	ignoredRet, srv := newBridgeServer(t, rest.Options{})
	_ = ignoredRet

	// Open two concurrent SSE streams on the same topic.
	ctx1, cancel1 := context.WithCancel(context.Background())
	defer cancel1()
	ctx2, cancel2 := context.WithCancel(context.Background())
	defer cancel2()

	req1, _ := http.NewRequestWithContext(ctx1, http.MethodGet, srv.URL+"/topics/cache-hit", nil)
	req2, _ := http.NewRequestWithContext(ctx2, http.MethodGet, srv.URL+"/topics/cache-hit", nil)

	go func() {
		resp, err := http.DefaultClient.Do(req1)
		if err == nil {
			resp.Body.Close()
		}
	}()
	time.Sleep(30 * time.Millisecond)
	go func() {
		resp, err := http.DefaultClient.Do(req2)
		if err == nil {
			resp.Body.Close()
		}
	}()
	time.Sleep(30 * time.Millisecond)
	// Both streams ran; second call hits getOrCreateSub cache.
}

// ── handleSubscribe: channel closed ──────────────────────────────────────────

// TestBridge_Subscribe_ChannelClosed covers the !ok branch in the SSE select
// loop when b.Close() is called, which closes the subscriber channel.
// Uses b.ServeHTTP directly so r.Context() never cancels — only the channel
// close can terminate the handler.
func TestBridge_Subscribe_ChannelClosed(t *testing.T) {
	p, err := mock.New(dds.Domain(0))
	if err != nil {
		t.Fatalf("mock.New: %v", err)
	}
	defer p.Close()
	b := rest.New(p, rest.Options{})

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "/topics/chan-close", nil)
	rw := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		defer close(done)
		b.ServeHTTP(rw, req) // blocks in SSE select loop
	}()

	time.Sleep(30 * time.Millisecond) // let subscribe establish
	_ = b.Close()                     // closes subscriber channel → !ok branch fires

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("SSE handler did not exit after subscriber channel closed")
	}
}

// ── handleSubscribe: write error ─────────────────────────────────────────────

// failWriter wraps a ResponseWriter and implements http.Flusher. It allows the
// initial headers/flush through, then returns an error on the first Write call
// that carries SSE data (triggered after the first Flush).
type failWriter struct {
	http.ResponseWriter
	flushed bool
	failed  bool
}

func (w *failWriter) Write(b []byte) (int, error) {
	if w.flushed {
		w.failed = true
		return 0, errors.New("injected write error")
	}
	return w.ResponseWriter.Write(b)
}
func (w *failWriter) Flush() {
	if flusher, ok := w.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
	w.flushed = true
}

// TestBridge_Subscribe_KeepaliveWriteError covers the write-error return in
// the keepalive case of the SSE select loop.
func TestBridge_Subscribe_KeepaliveWriteError(t *testing.T) {
	p, err := mock.New(dds.Domain(0))
	if err != nil {
		t.Fatalf("mock.New: %v", err)
	}
	defer p.Close()
	opts := rest.Options{SSEKeepalive: 5 * time.Millisecond}
	b := rest.New(p, opts)
	defer b.Close()

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "/topics/ka/we", nil)
	rw := httptest.NewRecorder()
	fw := &failWriter{ResponseWriter: rw}

	done := make(chan struct{})
	go func() {
		defer close(done)
		b.ServeHTTP(fw, req)
	}()

	select {
	case <-done: // keepalive write error caused handler to return
	case <-time.After(2 * time.Second):
		t.Fatal("handleSubscribe did not exit after keepalive write error")
	}
}

// TestBridge_Subscribe_WriteError covers the write-error return in the SSE
// message loop when fmt.Fprintf fails after a sample is delivered.
func TestBridge_Subscribe_WriteError(t *testing.T) {
	p, err := mock.New(dds.Domain(0))
	if err != nil {
		t.Fatalf("mock.New: %v", err)
	}
	defer p.Close()
	b := rest.New(p, rest.Options{})
	defer b.Close()

	pub, err := p.NewPublisher("we/test", dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
	defer pub.Close()

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "/topics/we/test", nil)
	rw := httptest.NewRecorder()
	fw := &failWriter{ResponseWriter: rw}

	// handleSubscribe blocks until context done or write error.
	// Drive it: start in a goroutine, publish a sample to trigger the write,
	// then wait for the handler to return.
	done := make(chan struct{})
	go func() {
		defer close(done)
		b.ServeHTTP(fw, req)
	}()

	time.Sleep(30 * time.Millisecond) // let subscribe establish + initial flush set flushed=true
	if writeErr := pub.Write([]byte("trigger")); writeErr != nil {
		t.Fatalf("Write: %v", writeErr)
	}

	select {
	case <-done:
		if !fw.failed {
			t.Error("write error was not injected")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("handleSubscribe did not exit after write error")
	}
}

// ── Fuzz ──────────────────────────────────────────────────────────────────────

func FuzzBridge_Publish(f *testing.F) {
	p, err := mock.New(dds.Domain(0))
	if err != nil {
		f.Fatal(err)
	}
	f.Cleanup(func() { p.Close() })
	b := rest.New(p, rest.Options{})
	srv := httptest.NewServer(b)
	f.Cleanup(func() { srv.Close(); b.Close() })

	f.Add([]byte("hello"))
	f.Add([]byte(""))
	f.Add([]byte{0x00, 0xFF, 0xAB})

	f.Fuzz(func(t *testing.T, payload []byte) {
		req, err := http.NewRequestWithContext(context.Background(), http.MethodPost,
			srv.URL+"/topics/fuzz", strings.NewReader(string(payload)))
		if err != nil {
			t.Fatalf("NewRequestWithContext: %v", err)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return
		}
		io.Copy(io.Discard, resp.Body) //nolint:errcheck
		resp.Body.Close()
		if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusInternalServerError {
			t.Errorf("unexpected status %d", resp.StatusCode)
		}
	})
}

func FuzzBridge_TopicPath(f *testing.F) {
	p, err := mock.New(dds.Domain(0))
	if err != nil {
		f.Fatal(err)
	}
	f.Cleanup(func() { p.Close() })
	b := rest.New(p, rest.Options{})
	srv := httptest.NewServer(b)
	f.Cleanup(func() { srv.Close(); b.Close() })

	f.Add("/topics/sensors/temp")
	f.Add("/topics/")
	f.Add("/topics")
	f.Add("/topics/a/b/c/d")

	f.Fuzz(func(t *testing.T, path string) {
		// Use a cancel context so SSE streams (which block forever) are cut short.
		ctx, cancel := context.WithCancel(context.Background())
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+path, nil)
		if err != nil {
			cancel()
			return
		}
		resp, err := http.DefaultClient.Do(req)
		cancel() // cut any open SSE stream immediately
		if err != nil {
			return
		}
		io.Copy(io.Discard, resp.Body) //nolint:errcheck
		resp.Body.Close()
		// Must not panic — any HTTP status is acceptable.
	})
}
