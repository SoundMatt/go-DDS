// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package monitor_test

import (
	"bufio"
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	dds "github.com/SoundMatt/go-DDS"
	"github.com/SoundMatt/go-DDS/mock"
	"github.com/SoundMatt/go-DDS/monitor"
)

func newMockParticipant(t *testing.T) dds.Participant {
	t.Helper()
	p, err := mock.New(dds.Domain(0))
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func get(ctx context.Context, url string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	return http.DefaultClient.Do(req)
}

func TestMonitor_ServesIndexHTML(t *testing.T) {
	p := newMockParticipant(t)
	defer p.Close()
	mon, err := monitor.New(p, monitor.Options{Addr: "127.0.0.1:0"})
	if err != nil {
		t.Fatal(err)
	}
	defer mon.Close()

	ctx := context.Background()
	resp, err := get(ctx, "http://"+mon.Addr()+"/")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Fatalf("want text/html, got %q", ct)
	}
}

func TestMonitor_AddrReturnsListenAddr(t *testing.T) {
	p := newMockParticipant(t)
	defer p.Close()
	mon, err := monitor.New(p, monitor.Options{Addr: "127.0.0.1:0"})
	if err != nil {
		t.Fatal(err)
	}
	defer mon.Close()
	if mon.Addr() == "" {
		t.Fatal("Addr should not be empty")
	}
}

func TestMonitor_SSEDeliversSampleEvent(t *testing.T) {
	p := newMockParticipant(t)
	defer p.Close()
	mon, err := monitor.New(p, monitor.Options{Addr: "127.0.0.1:0"})
	if err != nil {
		t.Fatal(err)
	}
	defer mon.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	resp, err := get(ctx, "http://"+mon.Addr()+"/events")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	// Give the SSE connection time to register.
	time.Sleep(50 * time.Millisecond)
	mon.Publish(dds.Sample{Topic: "test/topic", Payload: []byte("hello")})

	scanner := bufio.NewScanner(resp.Body)
	found := false
	for scanner.Scan() {
		line := scanner.Text()
		if strings.Contains(line, "test/topic") {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected sample event with topic 'test/topic' over SSE")
	}
}

func TestMonitor_MetricsProvider_PushesMetrics(t *testing.T) {
	p := newMockParticipant(t)
	defer p.Close()
	mon, err := monitor.New(p, monitor.Options{
		Addr:            "127.0.0.1:0",
		MetricsInterval: 50 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer mon.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	resp, err := get(ctx, "http://"+mon.Addr()+"/events")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	scanner := bufio.NewScanner(resp.Body)
	found := false
	for scanner.Scan() {
		line := scanner.Text()
		if strings.Contains(line, "write_count") {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected metrics event with write_count over SSE")
	}
}

func TestMonitor_Close_StopsServer(t *testing.T) {
	p := newMockParticipant(t)
	defer p.Close()
	mon, err := monitor.New(p, monitor.Options{Addr: "127.0.0.1:0"})
	if err != nil {
		t.Fatal(err)
	}
	addr := mon.Addr()
	closeErr := mon.Close()
	if closeErr != nil {
		t.Fatal(closeErr)
	}
	// After close, requests should fail.
	ctx := context.Background()
	resp, reqErr := get(ctx, "http://"+addr+"/")
	if reqErr == nil {
		resp.Body.Close()
		t.Fatal("expected error after monitor closed")
	}
}
