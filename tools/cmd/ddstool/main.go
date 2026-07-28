// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Command ddstool is a command-line interface for DDS publish/subscribe
// operations, participant diagnostics, and IDL code generation.
//
// Usage:
//
//	ddstool pub       -topic <name> [-payload <str>] [-count N] [-domain N] [-mock]
//	ddstool sub       -topic <name> [-count N] [-timeout D] [-domain N] [-mock]
//	ddstool discover  [-wait D] [-domain N] [-mock]
//	ddstool ros2-list [-wait D] [-domain N] [-node NAME] [-namespace NS]
//	ddstool idl       [-out <file>] <input.idl>
//	ddstool help
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	dds "github.com/SoundMatt/go-DDS"
	"github.com/SoundMatt/go-DDS/mock"
	"github.com/SoundMatt/go-DDS/ros2"
	"github.com/SoundMatt/go-DDS/rtps"
	"github.com/SoundMatt/go-DDS/tools/idl"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}
	switch os.Args[1] {
	case "pub":
		os.Exit(runPub(os.Args[2:]))
	case "sub":
		os.Exit(runSub(os.Args[2:]))
	case "discover":
		os.Exit(runDiscover(os.Args[2:]))
	case "ros2-list":
		os.Exit(runROS2List(os.Args[2:]))
	case "idl":
		os.Exit(runIDL(os.Args[2:]))
	case "help", "-h", "--help":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "ddstool: unknown subcommand %q\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Fprint(os.Stderr, `ddstool — DDS command-line tool

USAGE
  ddstool <subcommand> [flags]

SUBCOMMANDS
  pub        Publish a message to a DDS topic
  sub        Subscribe to a DDS topic and print samples
  discover   Print discovery and health diagnostics
  ros2-list  List live ROS 2 nodes/topics visible from go-DDS
  idl        Compile an IDL file to Go source code
  help       Show this message

GLOBAL FLAGS (pub/sub/discover)
  -domain int   DDS domain ID (default 0)
  -mock         Use in-process mock transport (default: RTPS/UDP)

Run 'ddstool <subcommand> -h' for per-subcommand flags.
`)
}

// commonFlags holds flags shared by all subcommands.
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

// ── pub ───────────────────────────────────────────────────────────────────────

func runPub(args []string) int {
	fs := flag.NewFlagSet("pub", flag.ContinueOnError)
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
		fmt.Fprintln(os.Stderr, "pub: -topic is required")
		fs.Usage()
		return 1
	}

	p, err := common.newParticipant()
	if err != nil {
		fmt.Fprintf(os.Stderr, "pub: participant: %v\n", err)
		return 1
	}
	defer func() { _ = p.Close() }()

	pub, err := p.NewPublisher(*topic, dds.DefaultQoS)
	if err != nil {
		fmt.Fprintf(os.Stderr, "pub: publisher: %v\n", err)
		return 1
	}
	defer func() { _ = pub.Close() }()

	data := []byte(*payload)
	for i := 0; *count == 0 || i < *count; i++ {
		if err := pub.Write(data); err != nil {
			fmt.Fprintf(os.Stderr, "pub: write: %v\n", err)
			return 1
		}
		if *count > 0 {
			fmt.Printf("[%d/%d] topic=%s payload=%q\n", i+1, *count, *topic, *payload)
		} else {
			fmt.Printf("[%d] topic=%s payload=%q\n", i+1, *topic, *payload)
		}
		if *interval > 0 && (*count == 0 || i < *count-1) {
			time.Sleep(*interval)
		}
	}
	return 0
}

// ── sub ───────────────────────────────────────────────────────────────────────

func runSub(args []string) int {
	fs := flag.NewFlagSet("sub", flag.ContinueOnError)
	var common commonFlags
	common.register(fs)
	topic := fs.String("topic", "", "DDS topic name (required)")
	count := fs.Int("count", 0, "stop after N samples (0 = unlimited)")
	timeout := fs.Duration("timeout", 30*time.Second, "exit after no sample for this duration")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	if *topic == "" {
		fmt.Fprintln(os.Stderr, "sub: -topic is required")
		fs.Usage()
		return 1
	}

	p, err := common.newParticipant()
	if err != nil {
		fmt.Fprintf(os.Stderr, "sub: participant: %v\n", err)
		return 1
	}
	defer func() { _ = p.Close() }()

	sub, err := p.NewSubscriber(*topic, dds.DefaultQoS)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sub: subscriber: %v\n", err)
		return 1
	}
	defer func() { _ = sub.Close() }()

	fmt.Printf("subscribed: topic=%s domain=%d\n", *topic, common.domain)
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
			fmt.Printf("[%d] topic=%s time=%s payload=%s\n",
				received, s.Topic, s.Timestamp.Format(time.RFC3339Nano), s.Payload)
		case <-time.After(*timeout):
			fmt.Fprintf(os.Stderr, "sub: idle timeout after %v (received %d samples)\n",
				*timeout, received)
			return 0
		case <-ctx.Done():
			return 0
		}
	}
}

// ── idl ───────────────────────────────────────────────────────────────────────

func runIDL(args []string) int {
	fs := flag.NewFlagSet("idl", flag.ContinueOnError)
	out := fs.String("out", "", "output file path (default: print to stdout)")
	pkg := fs.String("package", "", "override package name in generated output")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "idl: usage: ddstool idl [-out <file>] [-package <name>] <input.idl>")
		return 1
	}
	input := fs.Arg(0)

	m, err := idl.ParseFile(input)
	if err != nil {
		fmt.Fprintf(os.Stderr, "idl: parse %s: %v\n", input, err)
		return 1
	}
	if *pkg != "" {
		m.Name = *pkg
	}
	src, err := idl.Generate(m)
	if err != nil {
		fmt.Fprintf(os.Stderr, "idl: generate: %v\n", err)
		return 1
	}

	if *out == "" {
		fmt.Print(src)
		return 0
	}
	if err := os.WriteFile(*out, []byte(src), 0o640); err != nil {
		fmt.Fprintf(os.Stderr, "idl: write %s: %v\n", *out, err)
		return 1
	}
	fmt.Fprintf(os.Stderr, "idl: wrote %s\n", *out)
	return 0
}

// ── discover ──────────────────────────────────────────────────────────────────

func runDiscover(args []string) int {
	fs := flag.NewFlagSet("discover", flag.ContinueOnError)
	var common commonFlags
	common.register(fs)
	wait := fs.Duration("wait", 3*time.Second, "time to collect discovery information")
	if err := fs.Parse(args); err != nil {
		return 1
	}

	p, err := common.newParticipant()
	if err != nil {
		fmt.Fprintf(os.Stderr, "discover: participant: %v\n", err)
		return 1
	}
	defer func() { _ = p.Close() }()

	fmt.Printf("discovering: domain=%d wait=%v\n", common.domain, *wait)
	time.Sleep(*wait)

	if hp, ok := p.(dds.HealthProvider); ok {
		h := hp.Health()
		fmt.Printf("health: %s\n", h.Status)
		if h.Details != "" {
			fmt.Printf("  details: %s\n", h.Details)
		}
	}

	if dp, ok := p.(dds.DiscoveryMetricsProvider); ok {
		dm := dp.DiscoveryMetrics()
		fmt.Printf("discovery:\n")
		fmt.Printf("  announces_sent:     %d\n", dm.AnnouncesSent)
		fmt.Printf("  announces_received: %d\n", dm.AnnouncesReceived)
		fmt.Printf("  peers_known:        %d\n", dm.PeersKnown)
		fmt.Printf("  peer_evictions:     %d\n", dm.PeerEvictions)
		fmt.Printf("  endpoint_matches:   %d\n", dm.EndpointMatches)
	}

	if tp, ok := p.(dds.TopicMetricsProvider); ok {
		topics := tp.TopicMetrics()
		if len(topics) > 0 {
			fmt.Printf("topics:\n")
			for _, tm := range topics {
				fmt.Printf("  %s: writes=%d delivers=%d drops=%d\n",
					tm.Topic, tm.WriteCount, tm.DeliverCount, tm.DropCount)
			}
		}
	}

	return 0
}

// ── ros2-list ─────────────────────────────────────────────────────────────────

// runROS2List implements `ddstool ros2-list` (ROADMAP.md Milestone 17,
// "ROS 2 / rmw Compatibility"): it joins the ROS 2 graph as its own
// (otherwise silent — it publishes/subscribes nothing) node, waits for
// SPDP/SEDP discovery and ros_discovery_info graph exchange to settle, then
// prints every ROS 2 node and topic visible from this domain — the same
// graph `ros2 node list` / `ros2 topic list` would show, with no bridge
// process required.
func runROS2List(args []string) int {
	fs := flag.NewFlagSet("ros2-list", flag.ContinueOnError)
	domain := fs.Int("domain", 0, "DDS domain ID")
	node := fs.String("node", "ddstool_ros2_list", "ROS 2 node name this listing announces itself under")
	namespace := fs.String("namespace", "/", "ROS 2 namespace for -node")
	wait := fs.Duration("wait", 3*time.Second, "time to collect discovery information")
	if err := fs.Parse(args); err != nil {
		return 1
	}

	p, err := ros2.NewROS2Participant(dds.Domain(*domain), *node, *namespace)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ros2-list: participant: %v\n", err)
		return 1
	}
	defer func() { _ = p.Close() }()

	fmt.Printf("ros2-list: domain=%d node=%s wait=%v\n", *domain, p.FullyQualifiedNodeName(), *wait)
	time.Sleep(*wait)

	nodes := p.Nodes()
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].FullyQualifiedName < nodes[j].FullyQualifiedName })
	fmt.Printf("nodes (%d):\n", len(nodes))
	for _, n := range nodes {
		tag := ""
		if n.Local {
			tag = "  (this process)"
		}
		fmt.Printf("  %s%s\n", n.FullyQualifiedName, tag)
	}

	topics := p.Topics()
	sort.Slice(topics, func(i, j int) bool { return topics[i].Name < topics[j].Name })
	fmt.Printf("topics (%d):\n", len(topics))
	for _, tp := range topics {
		types := strings.Join(tp.Types, ", ")
		if types == "" {
			types = "?"
		}
		fmt.Printf("  %s [%s] (publishers=%d subscribers=%d)\n",
			tp.Name, types, tp.PublisherCount, tp.SubscriberCount)
	}

	return 0
}
