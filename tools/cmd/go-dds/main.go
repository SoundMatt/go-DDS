// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Command go-dds is the RELAY-conformant CLI for the go-DDS library.
// It supports publish/subscribe operations, participant diagnostics,
// IDL code generation, and RELAY protocol introspection.
//
// Usage:
//
//	go-dds version    [--format text|json]
//	go-dds capabilities
//	go-dds status     [--format text|json] [-domain N] [-mock]
//	go-dds convert    --protocol DDS [--format json]   (dds.Sample JSON on stdin)
//	go-dds pub        -topic <name> [-payload <str>] [-count N] [-domain N] [-mock]
//	go-dds sub        -topic <name> [-count N] [-timeout D] [-domain N] [-mock]
//	go-dds discover   [-wait D] [-domain N] [-mock]
//	go-dds idl        [-out <file>] <input.idl>
//	go-dds help
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"
	"time"

	relay "github.com/SoundMatt/RELAY/v2"
	dds "github.com/SoundMatt/go-DDS"
	"github.com/SoundMatt/go-DDS/mock"
	rtps "github.com/SoundMatt/go-DDS/rtps"
	"github.com/SoundMatt/go-DDS/tools/idl"
)

// toolVersion may be overridden at build time via -ldflags "-X main.toolVersion=x.y.z".
var toolVersion = "0.52.0"

const (
	toolName    = "go-dds"
	protocolStr = "DDS"
	protocolInt = 2
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

// run is the testable entry point. It dispatches subcommands and returns the
// process exit code. All I/O is threaded through the provided streams so the
// CLI can be exercised in unit tests without touching os.Stdin/Stdout/Stderr.
func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) < 1 {
		printUsage(stderr)
		return 1
	}
	switch args[0] {
	case "version":
		return runVersion(args[1:], stdout)
	case "capabilities":
		return runCapabilities(args[1:], stdout)
	case "status":
		return runStatus(args[1:], stdout)
	case "convert":
		return runConvert(args[1:], stdin, stdout, stderr)
	case "send":
		return runSend(args[1:], stdin, stdout, stderr)
	case "subscribe":
		return runSubscribe(args[1:], stdout, stderr)
	case "pub":
		return runPub(args[1:], stdout, stderr)
	case "sub":
		return runSub(args[1:], stdout, stderr)
	case "discover":
		return runDiscover(args[1:], stdout, stderr)
	case "idl":
		return runIDL(args[1:], stdout, stderr)
	case "help", "-h", "--help":
		printUsage(stderr)
		return 0
	default:
		fmt.Fprintf(stderr, "go-dds: unknown subcommand %q\n", args[0])
		printUsage(stderr)
		return 1
	}
}

func printUsage(w io.Writer) {
	fmt.Fprint(w, `go-dds — DDS command-line tool (RELAY-conformant)

USAGE
  go-dds <subcommand> [flags]

SUBCOMMANDS
  version       Print version information
  capabilities  Print supported commands, transports, and interfaces (JSON)
  status        Print self-assessed health status
  convert       Convert a canonical dds.Sample (stdin JSON) to a relay.Message
  send          Publish a message, or stream relay.Message NDJSON from stdin
  subscribe     Subscribe to a topic and print relay.Message NDJSON / text
  pub           (alias of send) Publish a message to a DDS topic
  sub           (alias of subscribe) Subscribe and print samples
  discover      Print discovery and health diagnostics
  idl           Compile an IDL file to Go source code
  help          Show this message

GLOBAL FLAGS (pub/sub/discover/status)
  -domain int   DDS domain ID (default 0)
  -mock         Use in-process mock transport (default: RTPS/UDP)

Run 'go-dds <subcommand> -h' for per-subcommand flags.
`)
}

// commonFlags holds flags shared by subcommands that connect to a DDS domain.
type commonFlags struct {
	domain  int
	useMock bool
}

func (c *commonFlags) register(fs *flag.FlagSet) {
	fs.IntVar(&c.domain, "domain", 0, "DDS domain ID")
	fs.BoolVar(&c.useMock, "mock", false, "use in-process mock (no UDP)")
}

func (c *commonFlags) newParticipant() (dds.Participant, error) {
	if c.useMock {
		return mock.New(dds.Domain(c.domain))
	}
	return rtps.New(dds.Domain(c.domain))
}

// newFlagSet returns a FlagSet whose usage/errors are written to stderr.
func newFlagSet(name string, stderr io.Writer) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(stderr)
	return fs
}

// ── version ───────────────────────────────────────────────────────────────────

func runVersion(args []string, stdout io.Writer) int {
	fs := newFlagSet("version", stdout)
	format := fs.String("format", "text", "output format: text or json")
	if err := fs.Parse(args); err != nil {
		return 1
	}

	if *format == "json" {
		encodeJSON(stdout, versionDoc())
		return 0
	}

	fmt.Fprintf(stdout, "%s %s (DDS, RELAY spec %s, %s)\n", toolName, toolVersion, dds.SpecVersion, runtime.Version())
	return 0
}

// versionDoc builds the §12.1 cli-version document.
func versionDoc() map[string]any {
	return map[string]any{
		"tool":         toolName,
		"protocol":     protocolStr,
		"protocol_int": protocolInt,
		"version":      toolVersion,
		"spec_version": dds.SpecVersion,
		"language":     "go",
		"runtime":      runtime.Version(),
	}
}

// ── capabilities ──────────────────────────────────────────────────────────────

func runCapabilities(args []string, stdout io.Writer) int {
	fs := newFlagSet("capabilities", stdout)
	if err := fs.Parse(args); err != nil {
		return 1
	}

	encodeJSON(stdout, capabilitiesDoc())
	return 0
}

// capabilitiesDoc builds the §12.2 cli-capabilities document. The field set and
// naming follow spec/schemas/cli-capabilities.json: no protocol/protocol_int
// keys, "features" is required, and "interfaces" lists relay interfaces.
func capabilitiesDoc() map[string]any {
	return map[string]any{
		"kind":                "capabilities",
		"tool":                toolName,
		"version":             toolVersion,
		"spec_version":        dds.SpecVersion,
		"commands":            []string{"version", "capabilities", "status", "convert", "send", "subscribe", "discover", "idl"},
		"transports":          []string{"rtps", "shmem", "mock"},
		"features":            []string{"reliable", "transient-local", "fragmentation", "security", "loaning", "shmem-zerocopy", "tsn"},
		"interfaces":          []string{"Node"},
		"optional_interfaces": []string{"HealthProvider", "MetricsProvider", "Drainer", "LoaningPublisher"},
		"adapt":               true,
	}
}

// ── status ────────────────────────────────────────────────────────────────────

func runStatus(args []string, stdout io.Writer) int {
	fs := newFlagSet("status", stdout)
	var common commonFlags
	common.register(fs)
	format := fs.String("format", "text", "output format: text or json")
	if err := fs.Parse(args); err != nil {
		return 1
	}

	// Use mock to verify the library is healthy without requiring a live network.
	healthy := false
	if p, err := mock.New(dds.Domain(common.domain)); err == nil {
		healthy = true
		_ = p.Close()
	}

	if *format == "json" {
		encodeJSON(stdout, statusDoc(healthy))
		return 0
	}

	status := "healthy"
	if !healthy {
		status = "unhealthy"
	}
	fmt.Fprintf(stdout, "tool:      %s\nversion:   %s\nprotocol:  %s\nstatus:    %s\nconnected: false\n",
		toolName, toolVersion, protocolStr, status)
	return 0
}

// statusDoc builds the §12.3 cli-status document.
func statusDoc(healthy bool) map[string]any {
	return map[string]any{
		"protocol":  protocolStr,
		"tool":      toolName,
		"version":   toolVersion,
		"healthy":   healthy,
		"connected": false,
		"endpoint":  "",
		"details":   map[string]any{},
	}
}

// encodeJSON writes v as indented JSON to w.
func encodeJSON(w io.Writer, v any) {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}

// ── convert (RELAY §11.2 interop driver) ────────────────────────────────────────

// runConvert implements `go-dds convert --protocol DDS [--format json]`, the
// RELAY §11.2 black-box interop driver. It reads one canonical dds.Sample value
// as JSON on stdin (schema: RELAY spec/schemas/dds-sample.json), runs it through
// this implementation's own Sample.ToMessage() — the same code path used at
// runtime — and writes the resulting relay.Message as JSON on stdout. The
// timestamp is zeroed so results are comparable across implementations.
//
// Exit codes (spec §11.2): 0 converted, 1 invalid input, 2 invalid args.
func runConvert(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := newFlagSet("convert", stderr)
	protocol := fs.String("protocol", "", "protocol of the canonical value (DDS)")
	format := fs.String("format", "json", "output format: json")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *protocol == "" {
		fmt.Fprintln(stderr, "convert: --protocol is required")
		return 2
	}
	if strings.ToUpper(*protocol) != protocolStr {
		fmt.Fprintf(stderr, "convert: unsupported protocol %q (this driver only converts DDS)\n", *protocol)
		return 2
	}
	if *format != "json" {
		fmt.Fprintf(stderr, "convert: unsupported format %q (only json)\n", *format)
		return 2
	}

	value, err := io.ReadAll(stdin)
	if err != nil {
		fmt.Fprintf(stderr, "convert: read stdin: %v\n", err)
		return 1
	}

	var s dds.Sample
	if uerr := json.Unmarshal(value, &s); uerr != nil {
		// Invalid input: emit the RELAY §5 sentinel error name to stderr.
		fmt.Fprintln(stderr, relayErrorName(uerr))
		return 1
	}

	msg := s.ToMessage()
	msg.Timestamp = time.Time{} // normalise for cross-implementation comparison

	out, merr := json.MarshalIndent(msg, "", "    ")
	if merr != nil {
		fmt.Fprintf(stderr, "convert: %v\n", merr)
		return 1
	}
	fmt.Fprintln(stdout, string(out))
	return 0
}

// relayErrorName maps a conversion error to the closest RELAY §5 sentinel error
// name. dds.Sample has no further validator beyond JSON decoding, so any decode
// failure is reported as the generic invalid-message sentinel; the payload-size
// sentinel is mapped through in case a future ToMessage path surfaces it.
func relayErrorName(err error) string {
	switch {
	case errors.Is(err, relay.ErrPayloadTooLarge):
		return "ErrPayloadTooLarge"
	default:
		return "ErrInvalidMessage"
	}
}

// ── send (RELAY §11.2) ──────────────────────────────────────────────────────

// runSend implements `send [--topic T --payload P] [--format text|json]`.
//
// In single-message mode it publishes one payload to a topic. In streaming-sink
// mode (`send --format json` with no --topic) it reads a stream of relay.Message
// values as NDJSON on stdin and publishes each until EOF — the egress dual of
// `subscribe --format json` and the sink used by `relay crossbar` (§11.2.1).
func runSend(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := newFlagSet("send", stderr)
	var common commonFlags
	common.register(fs)
	topic := fs.String("topic", "", "DDS topic name (single-message mode)")
	payload := fs.String("payload", "", "payload string to publish (single-message mode)")
	format := fs.String("format", "text", "output format: text or json (json with no -topic = NDJSON sink)")
	if err := fs.Parse(args); err != nil {
		return 1
	}

	p, err := common.newParticipant()
	if err != nil {
		fmt.Fprintf(stderr, "send: participant: %v\n", err)
		return 1
	}
	defer func() { _ = p.Close() }()

	// Streaming NDJSON sink: `send --format json` with no protocol flags.
	if *format == "json" && *topic == "" {
		return sendSink(p, stdin, stdout, stderr)
	}

	if *topic == "" {
		fmt.Fprintln(stderr, "send: -topic is required (or use --format json for the NDJSON sink)")
		return 1
	}
	pub, err := p.NewPublisher(*topic, dds.DefaultQoS)
	if err != nil {
		fmt.Fprintf(stderr, "send: publisher: %v\n", err)
		return 1
	}
	defer func() { _ = pub.Close() }()
	if err := pub.Write([]byte(*payload)); err != nil {
		fmt.Fprintf(stderr, "send: write: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "sent: topic=%s payload=%q\n", *topic, *payload)
	return 0
}

// sendSink reads relay.Message NDJSON from r and publishes each message to its
// topic (msg.ID), caching one publisher per topic, until EOF.
func sendSink(p dds.Participant, r io.Reader, stdout, stderr io.Writer) int {
	pubs := make(map[string]dds.Publisher)
	defer func() {
		for _, pub := range pubs {
			_ = pub.Close()
		}
	}()

	dec := json.NewDecoder(r)
	count := 0
	for {
		var m relay.Message
		if err := dec.Decode(&m); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			fmt.Fprintf(stderr, "send: decode NDJSON: %v\n", err)
			return 1
		}
		s, err := dds.FromMessage(m)
		if err != nil {
			fmt.Fprintf(stderr, "send: %s\n", relayErrorName(err))
			return 1
		}
		if s.Topic == "" {
			fmt.Fprintln(stderr, "send: message has empty topic (id); skipping")
			continue
		}
		pub, ok := pubs[s.Topic]
		if !ok {
			pub, err = p.NewPublisher(s.Topic, dds.DefaultQoS)
			if err != nil {
				fmt.Fprintf(stderr, "send: publisher %q: %v\n", s.Topic, err)
				return 1
			}
			pubs[s.Topic] = pub
		}
		if err := pub.Write(s.Payload); err != nil {
			fmt.Fprintf(stderr, "send: write %q: %v\n", s.Topic, err)
			return 1
		}
		count++
	}
	fmt.Fprintf(stdout, "sent %d message(s)\n", count)
	return 0
}

// ── subscribe (RELAY §11.2) ─────────────────────────────────────────────────

// runSubscribe implements `subscribe --topic T [--format text|json] [--count N]`.
// With --format json it prints each received sample as one relay.Message NDJSON
// line on stdout — the source consumed by `relay crossbar`. A --count of 0 (the
// default) streams indefinitely until the process is stopped or --timeout fires.
func runSubscribe(args []string, stdout, stderr io.Writer) int {
	fs := newFlagSet("subscribe", stderr)
	var common commonFlags
	common.register(fs)
	topic := fs.String("topic", "", "DDS topic name (required)")
	format := fs.String("format", "text", "output format: text or json (NDJSON)")
	count := fs.Int("count", 0, "stop after N samples (0 = stream until stopped)")
	timeout := fs.Duration("timeout", 0, "exit after no sample for this duration (0 = never)")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	if *topic == "" {
		fmt.Fprintln(stderr, "subscribe: -topic is required")
		return 1
	}

	p, err := common.newParticipant()
	if err != nil {
		fmt.Fprintf(stderr, "subscribe: participant: %v\n", err)
		return 1
	}
	defer func() { _ = p.Close() }()

	sub, err := p.NewSubscriber(*topic, dds.DefaultQoS)
	if err != nil {
		fmt.Fprintf(stderr, "subscribe: subscriber: %v\n", err)
		return 1
	}
	defer func() { _ = sub.Close() }()

	enc := json.NewEncoder(stdout) // compact, one JSON object per line = NDJSON

	// A single timer, reset after each sample, bounds idle time when --timeout>0.
	var timer *time.Timer
	var idle <-chan time.Time
	if *timeout > 0 {
		timer = time.NewTimer(*timeout)
		defer timer.Stop()
		idle = timer.C
	}

	received := 0
	for {
		if *count > 0 && received >= *count {
			return 0
		}
		select {
		case s, ok := <-sub.C():
			if !ok {
				return 0
			}
			received++
			if *format == "json" {
				_ = enc.Encode(s.ToMessage()) // Encode appends '\n' → NDJSON
			} else {
				fmt.Fprintf(stdout, "[%d] topic=%s time=%s payload=%s\n",
					received, s.Topic, s.Timestamp.Format(time.RFC3339Nano), s.Payload)
			}
			if timer != nil {
				if !timer.Stop() {
					<-timer.C
				}
				timer.Reset(*timeout)
			}
		case <-idle:
			fmt.Fprintf(stderr, "subscribe: idle timeout after %v (received %d)\n", *timeout, received)
			return 0
		}
	}
}

// ── pub ───────────────────────────────────────────────────────────────────────

func runPub(args []string, stdout, stderr io.Writer) int {
	fs := newFlagSet("pub", stderr)
	var common commonFlags
	common.register(fs)
	topic := fs.String("topic", "", "DDS topic name (required)")
	payload := fs.String("payload", "ping", "payload string to publish")
	count := fs.Int("count", 1, "number of times to publish (0 = unlimited)")
	interval := fs.Duration("interval", 0, "interval between publishes (0 = none)")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	if *topic == "" {
		fmt.Fprintln(stderr, "pub: -topic is required")
		return 1
	}

	p, err := common.newParticipant()
	if err != nil {
		fmt.Fprintf(stderr, "pub: participant: %v\n", err)
		return 1
	}
	defer func() { _ = p.Close() }()

	pub, err := p.NewPublisher(*topic, dds.DefaultQoS)
	if err != nil {
		fmt.Fprintf(stderr, "pub: publisher: %v\n", err)
		return 1
	}
	defer func() { _ = pub.Close() }()

	data := []byte(*payload)
	for i := 0; *count == 0 || i < *count; i++ {
		if err := pub.Write(data); err != nil {
			fmt.Fprintf(stderr, "pub: write: %v\n", err)
			return 1
		}
		if *count > 0 {
			fmt.Fprintf(stdout, "[%d/%d] topic=%s payload=%q\n", i+1, *count, *topic, *payload)
		} else {
			fmt.Fprintf(stdout, "[%d] topic=%s payload=%q\n", i+1, *topic, *payload)
		}
		if *interval > 0 && (*count == 0 || i < *count-1) {
			time.Sleep(*interval)
		}
	}
	return 0
}

// ── sub ───────────────────────────────────────────────────────────────────────

func runSub(args []string, stdout, stderr io.Writer) int {
	fs := newFlagSet("sub", stderr)
	var common commonFlags
	common.register(fs)
	topic := fs.String("topic", "", "DDS topic name (required)")
	count := fs.Int("count", 0, "stop after N samples (0 = unlimited)")
	timeout := fs.Duration("timeout", 30*time.Second, "exit after no sample for this duration")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	if *topic == "" {
		fmt.Fprintln(stderr, "sub: -topic is required")
		return 1
	}

	p, err := common.newParticipant()
	if err != nil {
		fmt.Fprintf(stderr, "sub: participant: %v\n", err)
		return 1
	}
	defer func() { _ = p.Close() }()

	sub, err := p.NewSubscriber(*topic, dds.DefaultQoS)
	if err != nil {
		fmt.Fprintf(stderr, "sub: subscriber: %v\n", err)
		return 1
	}
	defer func() { _ = sub.Close() }()

	fmt.Fprintf(stdout, "subscribed: topic=%s domain=%d\n", *topic, common.domain)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	received := 0
	for {
		if *count > 0 && received >= *count {
			return 0
		}
		select {
		case s, ok := <-sub.C():
			if !ok {
				return 0
			}
			received++
			fmt.Fprintf(stdout, "[%d] topic=%s time=%s payload=%s\n",
				received, s.Topic, s.Timestamp.Format(time.RFC3339Nano), s.Payload)
		case <-time.After(*timeout):
			fmt.Fprintf(stderr, "sub: idle timeout after %v (received %d samples)\n",
				*timeout, received)
			return 0
		case <-ctx.Done():
			return 0
		}
	}
}

// ── idl ───────────────────────────────────────────────────────────────────────

func runIDL(args []string, stdout, stderr io.Writer) int {
	fs := newFlagSet("idl", stderr)
	out := fs.String("out", "", "output file path (default: print to stdout)")
	pkg := fs.String("package", "", "override package name in generated output")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(stderr, "idl: usage: go-dds idl [-out <file>] [-package <name>] <input.idl>")
		return 1
	}
	input := fs.Arg(0)

	m, err := idl.ParseFile(input)
	if err != nil {
		fmt.Fprintf(stderr, "idl: parse %s: %v\n", input, err)
		return 1
	}
	if *pkg != "" {
		m.Name = *pkg
	}
	src, err := idl.Generate(m)
	if err != nil {
		fmt.Fprintf(stderr, "idl: generate: %v\n", err)
		return 1
	}

	if *out == "" {
		fmt.Fprint(stdout, src)
		return 0
	}
	if err := os.WriteFile(*out, []byte(src), 0o640); err != nil {
		fmt.Fprintf(stderr, "idl: write %s: %v\n", *out, err)
		return 1
	}
	fmt.Fprintf(stderr, "idl: wrote %s\n", *out)
	return 0
}

// ── discover ──────────────────────────────────────────────────────────────────

func runDiscover(args []string, stdout, stderr io.Writer) int {
	fs := newFlagSet("discover", stderr)
	var common commonFlags
	common.register(fs)
	wait := fs.Duration("wait", 3*time.Second, "time to collect discovery information")
	if err := fs.Parse(args); err != nil {
		return 1
	}

	p, err := common.newParticipant()
	if err != nil {
		fmt.Fprintf(stderr, "discover: participant: %v\n", err)
		return 1
	}
	defer func() { _ = p.Close() }()

	fmt.Fprintf(stdout, "discovering: domain=%d wait=%v\n", common.domain, *wait)
	time.Sleep(*wait)

	if hp, ok := p.(dds.HealthProvider); ok {
		h := hp.Health()
		fmt.Fprintf(stdout, "health: %s\n", h.Status)
		if h.Details != "" {
			fmt.Fprintf(stdout, "  details: %s\n", h.Details)
		}
	}

	if dp, ok := p.(dds.DiscoveryMetricsProvider); ok {
		dm := dp.DiscoveryMetrics()
		fmt.Fprintf(stdout, "discovery:\n")
		fmt.Fprintf(stdout, "  announces_sent:     %d\n", dm.AnnouncesSent)
		fmt.Fprintf(stdout, "  announces_received: %d\n", dm.AnnouncesReceived)
		fmt.Fprintf(stdout, "  peers_known:        %d\n", dm.PeersKnown)
		fmt.Fprintf(stdout, "  peer_evictions:     %d\n", dm.PeerEvictions)
		fmt.Fprintf(stdout, "  endpoint_matches:   %d\n", dm.EndpointMatches)
	}

	if tp, ok := p.(dds.TopicMetricsProvider); ok {
		topics := tp.TopicMetrics()
		if len(topics) > 0 {
			fmt.Fprintf(stdout, "topics:\n")
			for _, tm := range topics {
				fmt.Fprintf(stdout, "  %s: writes=%d delivers=%d drops=%d\n",
					tm.Topic, tm.WriteCount, tm.DeliverCount, tm.DropCount)
			}
		}
	}

	return 0
}
