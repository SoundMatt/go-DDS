// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// latmon continuously measures go-DDS end-to-end latency and GC pause
// statistics, printing a rolling summary to stdout (and optionally writing
// JSON to a file).
//
// Usage:
//
//	go run ./cmd/latmon [flags]
//
// Flags:
//
//	--hz N          Publisher rate in Hz (default 30)
//	--payload N     Payload size in bytes (default 512)
//	--window N      Reporting window in seconds (default 5)
//	--duration N    Run for N seconds then exit; 0 = run forever (default 0)
//	--output FILE   Append each window's JSON record to FILE (optional)
//	--gc-pressure   Enable 64 MiB/s GC allocation pressure (default false)
//
// Output format (one line per window):
//
//	[T+5s] gc=12 gc_p99=112µs gc_max=145µs lat_p50=19µs lat_p99=94µs lat_max=305µs msgs=150/150 PASS
//
// JSON record (written to --output file, one object per line):
//
//	{"ts":"2026-06-09T15:00:05Z","window_s":5,"gc_cycles":12,...}

package main

//fusa:req REQ-SAFETY-014

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"os"
	"runtime"
	"sort"
	"sync/atomic"
	"time"

	dds "github.com/SoundMatt/go-DDS"
	"github.com/SoundMatt/go-DDS/mock"
)

// ── flags ─────────────────────────────────────────────────────────────────────

var (
	flagHz         = flag.Int("hz", 30, "publisher rate in Hz")
	flagPayload    = flag.Int("payload", 512, "payload size in bytes")
	flagWindow     = flag.Int("window", 5, "reporting window in seconds")
	flagDuration   = flag.Int("duration", 0, "total run seconds (0 = forever)")
	flagOutput     = flag.String("output", "", "append JSON records to this file")
	flagGCPressure = flag.Bool("gc-pressure", false, "apply 64 MiB/s GC allocation pressure")
)

// ── JSON record ───────────────────────────────────────────────────────────────

type windowRecord struct {
	TS            string  `json:"ts"`
	WindowS       int     `json:"window_s"`
	ElapsedS      float64 `json:"elapsed_s"`
	GCCycles      uint32  `json:"gc_cycles"`
	GCPauseP50ns  uint64  `json:"gc_pause_p50_ns"`
	GCPauseP99ns  uint64  `json:"gc_pause_p99_ns"`
	GCPauseMaxNs  uint64  `json:"gc_pause_max_ns"`
	LatP50ns      uint64  `json:"lat_p50_ns"`
	LatP99ns      uint64  `json:"lat_p99_ns"`
	LatMaxNs      uint64  `json:"lat_max_ns"`
	MsgsDelivered int64   `json:"msgs_delivered"`
	MsgsExpected  int64   `json:"msgs_expected"`
	Pass          bool    `json:"pass"`
}

// ── percentile helpers ────────────────────────────────────────────────────────

func pctUint64(sorted []uint64, p float64) uint64 {
	if len(sorted) == 0 {
		return 0
	}
	idx := int(math.Ceil(p/100.0*float64(len(sorted)))) - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}

func maxUint64(s []uint64) uint64 {
	var m uint64
	for _, v := range s {
		if v > m {
			m = v
		}
	}
	return m
}

// newPausesFromStats returns GC pause durations between two MemStats snapshots.
func newPausesFromStats(before, after *runtime.MemStats) []uint64 {
	n := int(after.NumGC - before.NumGC)
	if n > 256 {
		n = 256
	}
	out := make([]uint64, 0, n)
	for i := 0; i < n; i++ {
		idx := (int(after.NumGC) - 1 - i + 256) % 256
		if p := after.PauseNs[idx]; p > 0 {
			out = append(out, p)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// ── window state ──────────────────────────────────────────────────────────────

type windowState struct {
	msgCount  atomic.Int64
	latencies []int64
	latCh     chan int64
	done      chan struct{}
}

func newWindowState(bufSize int) *windowState {
	ws := &windowState{
		latCh: make(chan int64, bufSize),
		done:  make(chan struct{}),
	}
	go func() {
		defer close(ws.done)
		for lat := range ws.latCh {
			ws.latencies = append(ws.latencies, lat)
		}
	}()
	return ws
}

func (ws *windowState) close() {
	close(ws.latCh)
	<-ws.done
}

func (ws *windowState) recordLat(lat int64) {
	ws.msgCount.Add(1)
	select {
	case ws.latCh <- lat:
	default:
	}
}

// ── main ──────────────────────────────────────────────────────────────────────

func main() {
	flag.Parse()

	hz := *flagHz
	payloadSize := *flagPayload
	windowSecs := *flagWindow
	durationSecs := *flagDuration
	outputFile := *flagOutput
	gcPressure := *flagGCPressure

	if hz <= 0 || payloadSize < 8 || windowSecs <= 0 {
		fmt.Fprintln(os.Stderr, "latmon: --hz, --payload (>= 8), and --window must be > 0")
		os.Exit(1)
	}

	// Half-period budget for the configured Hz.
	budgetNs := uint64(time.Second/time.Duration(hz)) / 2

	var outFile *os.File
	if outputFile != "" {
		var err error
		outFile, err = os.OpenFile(outputFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o640)
		if err != nil {
			fmt.Fprintf(os.Stderr, "latmon: open %s: %v\n", outputFile, err)
			os.Exit(1)
		}
		defer outFile.Close()
	}

	// Two participants: one publishes, one subscribes.
	pub, sub := makeParticipants()

	rootCtx := context.Background()
	if durationSecs > 0 {
		var cancel context.CancelFunc
		rootCtx, cancel = context.WithTimeout(rootCtx, time.Duration(durationSecs)*time.Second)
		defer cancel()
	}

	pubCtx, pubCancel := context.WithCancel(rootCtx)
	defer pubCancel()

	go publishLoop(pubCtx, pub, hz, payloadSize)
	if gcPressure {
		go pressureLoop(pubCtx)
	}

	start := time.Now()
	windowNum := 0

	fmt.Printf("latmon: hz=%d payload=%dB window=%ds budget=%s\n",
		hz, payloadSize, windowSecs, time.Duration(budgetNs))
	if gcPressure {
		fmt.Println("latmon: GC pressure enabled (64 MiB/s)")
	}

	for {
		ws := newWindowState(hz * windowSecs * 2)
		var msBefore runtime.MemStats
		runtime.ReadMemStats(&msBefore)
		windowStart := time.Now()

		subDone := make(chan struct{})
		go func() {
			defer close(subDone)
			subscribeWindow(pubCtx, sub, ws)
		}()

		select {
		case <-rootCtx.Done():
			ws.close()
			<-subDone
			return
		case <-time.After(time.Duration(windowSecs) * time.Second):
		}

		runtime.GC()
		var msAfter runtime.MemStats
		runtime.ReadMemStats(&msAfter)
		pubCancel() // stop subscriber too
		ws.close()
		<-subDone

		pauses := newPausesFromStats(&msBefore, &msAfter)
		gcCycles := msAfter.NumGC - msBefore.NumGC

		gcP50 := pctUint64(pauses, 50)
		gcP99 := pctUint64(pauses, 99)
		gcMax := maxUint64(pauses)

		lats := make([]uint64, 0, len(ws.latencies))
		for _, v := range ws.latencies {
			if v > 0 {
				lats = append(lats, uint64(v))
			}
		}
		sort.Slice(lats, func(i, j int) bool { return lats[i] < lats[j] })
		latP50 := pctUint64(lats, 50)
		latP99 := pctUint64(lats, 99)
		latMax := maxUint64(lats)

		windowNum++
		elapsed := time.Since(start)
		expectedMsgs := int64(hz * windowSecs)
		delivered := ws.msgCount.Load()
		pass := gcMax <= budgetNs && latMax <= budgetNs

		verdict := "PASS"
		if !pass {
			verdict = "FAIL"
		}

		fmt.Printf("[T+%.0fs] gc=%d gc_p99=%s gc_max=%s lat_p50=%s lat_p99=%s lat_max=%s msgs=%d/%d %s\n",
			elapsed.Seconds(),
			gcCycles,
			time.Duration(gcP99), time.Duration(gcMax),
			time.Duration(latP50), time.Duration(latP99), time.Duration(latMax),
			delivered, expectedMsgs,
			verdict,
		)

		if outFile != nil {
			rec := windowRecord{
				TS:            windowStart.UTC().Format(time.RFC3339),
				WindowS:       windowSecs,
				ElapsedS:      elapsed.Seconds(),
				GCCycles:      gcCycles,
				GCPauseP50ns:  gcP50,
				GCPauseP99ns:  gcP99,
				GCPauseMaxNs:  gcMax,
				LatP50ns:      latP50,
				LatP99ns:      latP99,
				LatMaxNs:      latMax,
				MsgsDelivered: delivered,
				MsgsExpected:  expectedMsgs,
				Pass:          pass,
			}
			line, err := json.Marshal(rec)
			if err == nil {
				_, _ = fmt.Fprintln(outFile, string(line))
			}
		}

		// Recreate context for the next window.
		pubCtx, pubCancel = context.WithCancel(rootCtx)
		go publishLoop(pubCtx, pub, hz, payloadSize)
		if gcPressure {
			go pressureLoop(pubCtx)
		}

		_ = windowNum
	}
}

// ── DDS setup ─────────────────────────────────────────────────────────────────

func makeParticipants() (dds.Publisher, dds.Subscriber) {
	pp, err := mock.New(dds.Domain(98))
	if err != nil {
		fmt.Fprintf(os.Stderr, "latmon: publisher participant: %v\n", err)
		os.Exit(1)
	}
	sp, err := mock.New(dds.Domain(98))
	if err != nil {
		fmt.Fprintf(os.Stderr, "latmon: subscriber participant: %v\n", err)
		os.Exit(1)
	}
	pub, err := pp.NewPublisher("latmon/probe", dds.DefaultQoS)
	if err != nil {
		fmt.Fprintf(os.Stderr, "latmon: NewPublisher: %v\n", err)
		os.Exit(1)
	}
	sub, err := sp.NewSubscriber("latmon/probe", dds.DefaultQoS)
	if err != nil {
		fmt.Fprintf(os.Stderr, "latmon: NewSubscriber: %v\n", err)
		os.Exit(1)
	}
	return pub, sub
}

// ── goroutines ────────────────────────────────────────────────────────────────

func publishLoop(ctx context.Context, pub dds.Publisher, hz, size int) {
	ticker := time.NewTicker(time.Second / time.Duration(hz))
	defer ticker.Stop()
	payload := make([]byte, size)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			binary.LittleEndian.PutUint64(payload[:8], uint64(time.Now().UnixNano()))
			if err := pub.Write(payload); err != nil {
				return
			}
		}
	}
}

func subscribeWindow(ctx context.Context, sub dds.Subscriber, ws *windowState) {
	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-sub.C():
			if !ok {
				return
			}
			if len(msg.Payload) >= 8 {
				sent := int64(binary.LittleEndian.Uint64(msg.Payload[:8]))
				ws.recordLat(time.Now().UnixNano() - sent)
			}
		}
	}
}

func pressureLoop(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	const allocPerTick = (64 * 1024 * 1024) / 100
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			buf := make([]byte, allocPerTick)
			_ = buf
		}
	}
}
