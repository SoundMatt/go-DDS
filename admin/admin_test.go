// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package admin_test

//fusa:test REQ-ADMIN-001
//fusa:test REQ-ADMIN-002
//fusa:test REQ-ADMIN-003
//fusa:test REQ-ADMIN-004
//fusa:test REQ-ADMIN-005

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	dds "github.com/SoundMatt/go-DDS"
	"github.com/SoundMatt/go-DDS/admin"
	"github.com/SoundMatt/go-DDS/mock"
)

func newPart(t *testing.T) dds.Participant {
	t.Helper()
	p, err := mock.New(0)
	if err != nil {
		t.Fatalf("mock.New: %v", err)
	}
	t.Cleanup(func() { _ = p.Close() })
	return p
}

func newServer(t *testing.T, p dds.Participant, opts admin.Options) *admin.Server {
	t.Helper()
	if opts.Addr == "" {
		opts.Addr = "127.0.0.1:0"
	}
	s, err := admin.New(p, opts)
	if err != nil {
		t.Fatalf("admin.New: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func get(t *testing.T, url string, headers ...string) *http.Response {
	t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	return resp
}

// tryGet issues a GET and returns (resp, err) without fatally failing.
func tryGet(url string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	return http.DefaultClient.Do(req)
}

func post(t *testing.T, url string, body []byte, headers ...string) *http.Response {
	t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, url,
		bytes.NewReader(body))
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST %s: %v", url, err)
	}
	return resp
}

// ── Tests ─────────────────────────────────────────────────────────────────────

func TestAdmin_Addr_NonEmpty(t *testing.T) {
	p := newPart(t)
	s := newServer(t, p, admin.Options{})
	if s.Addr() == "" {
		t.Error("Addr() must not be empty")
	}
}

func TestAdmin_Health_OK(t *testing.T) {
	p := newPart(t)
	s := newServer(t, p, admin.Options{})

	resp := get(t, "http://"+s.Addr()+"/admin/health")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var m map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&m); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if m["status"] != "ok" {
		t.Errorf("status: got %v, want ok", m["status"])
	}
}

func TestAdmin_Health_Down(t *testing.T) {
	p := newPart(t)
	s := newServer(t, p, admin.Options{})
	p.Close()

	resp := get(t, "http://"+s.Addr()+"/admin/health")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 for HealthDown, got %d", resp.StatusCode)
	}
}

func TestAdmin_Health_NoProvider(t *testing.T) {
	type minPart struct{ dds.Participant }
	realPart := newPart(t)
	stub := &minPart{Participant: realPart}

	s := newServer(t, stub, admin.Options{})
	resp := get(t, "http://"+s.Addr()+"/admin/health")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotImplemented {
		t.Fatalf("expected 501 when no HealthProvider, got %d", resp.StatusCode)
	}
}

func TestAdmin_Topics_ReturnsArray(t *testing.T) {
	p := newPart(t)
	s := newServer(t, p, admin.Options{})

	resp := get(t, "http://"+s.Addr()+"/admin/topics")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var arr []any
	if err := json.NewDecoder(resp.Body).Decode(&arr); err != nil {
		t.Fatalf("decode JSON array: %v", err)
	}
}

func TestAdmin_Topics_NoProvider_ReturnsEmptyArray(t *testing.T) {
	type minPart struct{ dds.Participant }
	realPart := newPart(t)
	stub := &minPart{Participant: realPart}

	s := newServer(t, stub, admin.Options{})
	resp := get(t, "http://"+s.Addr()+"/admin/topics")
	defer resp.Body.Close()

	var arr []any
	if err := json.NewDecoder(resp.Body).Decode(&arr); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(arr) != 0 {
		t.Errorf("expected empty array, got %v", arr)
	}
}

func TestAdmin_Discovery_ReturnsJSON(t *testing.T) {
	p := newPart(t)
	s := newServer(t, p, admin.Options{})

	resp := get(t, "http://"+s.Addr()+"/admin/discovery")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var m map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&m); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if _, ok := m["metrics"]; !ok {
		t.Error("expected 'metrics' key in discovery response")
	}
}

func TestAdmin_Publish_InjectsSample(t *testing.T) {
	p := newPart(t)
	s := newServer(t, p, admin.Options{})

	topic := "admin/inject/test"
	sub, err := p.NewSubscriber(topic, dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewSubscriber: %v", err)
	}
	defer sub.Close()

	payload := base64.StdEncoding.EncodeToString([]byte("hello admin"))
	body, _ := json.Marshal(map[string]string{"topic": topic, "payload": payload})

	resp := post(t, "http://"+s.Addr()+"/admin/publish", body)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	select {
	case s2 := <-sub.C():
		if string(s2.Payload) != "hello admin" {
			t.Errorf("injected payload: got %q, want hello admin", s2.Payload)
		}
	case <-make(chan struct{}): // no-op — select will always pick sub.C() or fall through
	}
}

func TestAdmin_Publish_EmptyTopic_BadRequest(t *testing.T) {
	p := newPart(t)
	s := newServer(t, p, admin.Options{})

	body, _ := json.Marshal(map[string]string{"topic": "", "payload": ""})
	resp := post(t, "http://"+s.Addr()+"/admin/publish", body)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for empty topic, got %d", resp.StatusCode)
	}
}

func TestAdmin_Publish_BadPayload_BadRequest(t *testing.T) {
	p := newPart(t)
	s := newServer(t, p, admin.Options{})

	body, _ := json.Marshal(map[string]string{"topic": "some/topic", "payload": "not-base64!!!"})
	resp := post(t, "http://"+s.Addr()+"/admin/publish", body)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for bad base64 payload, got %d", resp.StatusCode)
	}
}

func TestAdmin_Publish_BadJSON_BadRequest(t *testing.T) {
	p := newPart(t)
	s := newServer(t, p, admin.Options{})

	resp := post(t, "http://"+s.Addr()+"/admin/publish", []byte("{bad json"))
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for malformed JSON, got %d", resp.StatusCode)
	}
}

func TestAdmin_Publish_MethodNotAllowed(t *testing.T) {
	p := newPart(t)
	s := newServer(t, p, admin.Options{})

	resp := get(t, "http://"+s.Addr()+"/admin/publish")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405 for GET on /admin/publish, got %d", resp.StatusCode)
	}
}

func TestAdmin_Publish_ClosedParticipant(t *testing.T) {
	p := newPart(t)
	s := newServer(t, p, admin.Options{})
	p.Close()

	payload := base64.StdEncoding.EncodeToString([]byte("data"))
	body, _ := json.Marshal(map[string]string{"topic": "any/topic", "payload": payload})
	resp := post(t, "http://"+s.Addr()+"/admin/publish", body)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 for closed participant, got %d", resp.StatusCode)
	}
}

func TestAdmin_APIKey_Required(t *testing.T) {
	p := newPart(t)
	s := newServer(t, p, admin.Options{APIKey: "secret"})

	resp := get(t, "http://"+s.Addr()+"/admin/health")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 without API key, got %d", resp.StatusCode)
	}
}

func TestAdmin_APIKey_ValidBearer(t *testing.T) {
	p := newPart(t)
	s := newServer(t, p, admin.Options{APIKey: "mysecret"})

	resp := get(t, "http://"+s.Addr()+"/admin/health",
		"Authorization", "Bearer mysecret")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 with valid API key, got %d", resp.StatusCode)
	}
}

func TestAdmin_APIKey_WrongKey(t *testing.T) {
	p := newPart(t)
	s := newServer(t, p, admin.Options{APIKey: "correct"})

	resp := get(t, "http://"+s.Addr()+"/admin/health",
		"Authorization", "Bearer wrong")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 with wrong API key, got %d", resp.StatusCode)
	}
}

func TestAdmin_Close_StopsServer(t *testing.T) {
	p := newPart(t)
	s, err := admin.New(p, admin.Options{Addr: "127.0.0.1:0"})
	if err != nil {
		t.Fatalf("admin.New: %v", err)
	}
	addr := s.Addr()
	_ = s.Close()

	resp, err := tryGet("http://" + addr + "/admin/health")
	if err == nil {
		resp.Body.Close()
		t.Fatal("expected error after Close")
	}
}

func TestAdmin_New_ListenError(t *testing.T) {
	p := newPart(t)
	_, err := admin.New(p, admin.Options{Addr: "127.0.0.1:99999"})
	if err == nil {
		t.Fatal("expected error for invalid port")
	}
}

// TestAdmin_New_DefaultAddr exercises the default ":7070" address path when
// Options.Addr is empty. The test skips if port 7070 is already in use.
func TestAdmin_New_DefaultAddr(t *testing.T) {
	p := newPart(t)
	s, err := admin.New(p, admin.Options{}) // Addr == "" → defaults to ":7070"
	if err != nil {
		t.Skipf("default port 7070 not available (likely in use): %v", err)
	}
	defer func() { _ = s.Close() }()
	if s.Addr() == "" {
		t.Error("Addr() must not be empty after binding to default address")
	}
}

func TestAdmin_ContentType_JSON(t *testing.T) {
	p := newPart(t)
	s := newServer(t, p, admin.Options{})

	for _, path := range []string{"/admin/health", "/admin/topics", "/admin/discovery"} {
		resp := get(t, "http://"+s.Addr()+path)
		resp.Body.Close()
		ct := resp.Header.Get("Content-Type")
		if !strings.HasPrefix(ct, "application/json") {
			t.Errorf("%s Content-Type: got %q, want application/json", path, ct)
		}
	}
}
