// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// command-response demonstrates the OMG DDS-RPC request-reply pattern using
// rpc.Requester and rpc.Replier built on two DDS topics.
//
// Run with:
//
//	go run .
package main

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	dds "github.com/SoundMatt/go-DDS"
	"github.com/SoundMatt/go-DDS/mock"
	"github.com/SoundMatt/go-DDS/rpc"
)

type EchoRequest struct {
	Message string
}

type EchoReply struct {
	Echo        string
	ProcessedBy string
}

const service = "echo"

func main() {
	// Use isolated brokers so requester and replier have separate participants
	// that communicate through the shared mock broker.
	server, err := mock.New(dds.Domain(0))
	if err != nil {
		log.Fatal(err)
	}
	defer server.Close()

	client, err := mock.New(dds.Domain(0))
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	qos := dds.ReliableQoS

	replier, err := rpc.NewReplier[EchoRequest, EchoReply](
		server, service,
		dds.JSONCodec[EchoRequest]{}, dds.JSONCodec[EchoReply]{},
		qos,
	)
	if err != nil {
		log.Fatal(err)
	}
	defer replier.Close()

	requester, err := rpc.NewRequester[EchoRequest, EchoReply](
		client, service,
		dds.JSONCodec[EchoRequest]{}, dds.JSONCodec[EchoReply]{},
		qos,
	)
	if err != nil {
		log.Fatal(err)
	}
	defer requester.Close()

	// Server goroutine: handle requests until the replier closes.
	go func() {
		for req := range replier.Requests() {
			reply := EchoReply{
				Echo:        strings.ToUpper(req.Value.Message),
				ProcessedBy: "echo-server-1",
			}
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			if err := replier.Reply(ctx, req, reply); err != nil {
				fmt.Printf("reply error: %v\n", err)
			}
			cancel()
		}
	}()

	// Client: send five requests.
	messages := []string{"hello", "world", "go-dds", "rpc", "works"}
	ctx := context.Background()
	for _, msg := range messages {
		ctx2, cancel := context.WithTimeout(ctx, 2*time.Second)
		reply, err := requester.Request(ctx2, EchoRequest{Message: msg})
		cancel()
		if err != nil {
			log.Fatalf("request %q failed: %v", msg, err)
		}
		fmt.Printf("sent: %-10s  got: %-10s  from: %s\n",
			msg, reply.Echo, reply.ProcessedBy)
	}
}
