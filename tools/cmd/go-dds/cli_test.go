// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package main

// Behavioural tests for the go-dds CLI: subcommand dispatch, output documents,
// the RELAY §11.2 convert driver, and the pub/sub/discover/idl commands over the
// in-process mock transport. All I/O is captured via bytes.Buffer so no real
// process streams are touched.

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	relay "github.com/SoundMatt/RELAY/v2"
	dds "github.com/SoundMatt/go-DDS"
	"github.com/SoundMatt/go-DDS/mock"
)

// runCLI invokes run() with the given stdin text and returns exit code, stdout,
// stderr.
func runCLI(stdin string, args ...string) (int, string, string) {
	var out, errOut bytes.Buffer
	code := run(args, strings.NewReader(stdin), &out, &errOut)
	return code, out.String(), errOut.String()
}

func TestRun_Dispatch(t *testing.T) {
	cases := []struct {
		name     string
		args     []string
		wantCode int
		wantOut  string // substring expected on stdout (empty = skip)
		wantErr  string // substring expected on stderr (empty = skip)
	}{
		{"no args", nil, 1, "", "USAGE"},
		{"help", []string{"help"}, 0, "", "SUBCOMMANDS"},
		{"-h", []string{"-h"}, 0, "", "SUBCOMMANDS"},
		{"--help", []string{"--help"}, 0, "", "SUBCOMMANDS"},
		{"unknown", []string{"bogus"}, 1, "", "unknown subcommand"},
		{"version text", []string{"version"}, 0, "go-dds", ""},
		{"version json", []string{"version", "--format", "json"}, 0, `"spec_version"`, ""},
		{"version bad flag", []string{"version", "--nope"}, 1, "", ""},
		{"capabilities", []string{"capabilities"}, 0, `"capabilities"`, ""},
		{"capabilities bad flag", []string{"capabilities", "--nope"}, 1, "", ""},
		{"status text", []string{"status"}, 0, "status:    healthy", ""},
		{"status json", []string{"status", "--format", "json"}, 0, `"healthy"`, ""},
		{"status bad flag", []string{"status", "--nope"}, 1, "", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			code, out, errOut := runCLI("", c.args...)
			if code != c.wantCode {
				t.Errorf("exit = %d, want %d (stderr: %s)", code, c.wantCode, errOut)
			}
			if c.wantOut != "" && !strings.Contains(out, c.wantOut) {
				t.Errorf("stdout %q does not contain %q", out, c.wantOut)
			}
			if c.wantErr != "" && !strings.Contains(errOut, c.wantErr) {
				t.Errorf("stderr %q does not contain %q", errOut, c.wantErr)
			}
		})
	}
}

func TestVersion_JSON_TracksSpec(t *testing.T) {
	code, out, _ := runCLI("", "version", "--format", "json")
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	var doc map[string]any
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if doc["spec_version"] != relay.SpecVersion {
		t.Errorf("spec_version = %v, want %q", doc["spec_version"], relay.SpecVersion)
	}
}

// TestConvert_GoldenVector drives the §11.2 convert driver with the canonical
// dds-sample and asserts the emitted relay.Message matches Sample.ToMessage().
func TestConvert_GoldenVector(t *testing.T) {
	var guid dds.GUID
	for i := range guid {
		guid[i] = byte(i + 1)
	}
	in := dds.Sample{
		Topic:          "rt/chatter",
		Payload:        []byte("hello dds"),
		Timestamp:      time.Now(), // must be zeroed in the output
		SequenceNumber: 7,
		WriterGUID:     guid,
	}
	stdin, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}

	code, out, errOut := runCLI(string(stdin), "convert", "--protocol", "DDS")
	if code != 0 {
		t.Fatalf("exit = %d, stderr=%s", code, errOut)
	}

	var got relay.Message
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("output is not a relay.Message: %v\noutput: %s", err, out)
	}
	want := in.ToMessage()
	want.Timestamp = time.Time{}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("convert mismatch:\n got  %+v\n want %+v", got, want)
	}
	if !got.Timestamp.IsZero() {
		t.Errorf("convert output timestamp not zeroed: %v", got.Timestamp)
	}
}

// TestConvert_LowercaseProtocol confirms the protocol match is case-insensitive.
func TestConvert_LowercaseProtocol(t *testing.T) {
	in := `{"topic":"t","payload":"","timestamp":"0001-01-01T00:00:00Z","seq":0,"writer_guid":[0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0]}`
	code, out, errOut := runCLI(in, "convert", "--protocol", "dds")
	if code != 0 {
		t.Fatalf("exit = %d, stderr=%s", code, errOut)
	}
	if !strings.Contains(out, `"protocol": 2`) {
		t.Errorf("expected DDS protocol int 2 in output, got %s", out)
	}
}

func TestConvert_Errors(t *testing.T) {
	cases := []struct {
		name     string
		stdin    string
		args     []string
		wantCode int
		wantErr  string
	}{
		{"missing protocol", "", []string{"convert"}, 2, "--protocol is required"},
		{"unsupported protocol", "", []string{"convert", "--protocol", "CAN"}, 2, "unsupported protocol"},
		{"bad format", "", []string{"convert", "--protocol", "DDS", "--format", "yaml"}, 2, "unsupported format"},
		{"bad flag", "", []string{"convert", "--nope"}, 2, ""},
		{"invalid json", "{not json", []string{"convert", "--protocol", "DDS"}, 1, "ErrInvalidMessage"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			code, _, errOut := runCLI(c.stdin, c.args...)
			if code != c.wantCode {
				t.Errorf("exit = %d, want %d (stderr: %s)", code, c.wantCode, errOut)
			}
			if c.wantErr != "" && !strings.Contains(errOut, c.wantErr) {
				t.Errorf("stderr %q does not contain %q", errOut, c.wantErr)
			}
		})
	}
}

// errReader always fails, exercising the stdin-read error path of convert.
type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, errTest }

var errTest = &readErr{}

type readErr struct{}

func (*readErr) Error() string { return "boom" }

func TestConvert_StdinReadError(t *testing.T) {
	var out, errOut bytes.Buffer
	code := run([]string{"convert", "--protocol", "DDS"}, errReader{}, &out, &errOut)
	if code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	if !strings.Contains(errOut.String(), "read stdin") {
		t.Errorf("stderr %q does not mention read stdin", errOut.String())
	}
}

func TestRelayErrorName(t *testing.T) {
	if got := relayErrorName(relay.ErrPayloadTooLarge); got != "ErrPayloadTooLarge" {
		t.Errorf("relayErrorName(ErrPayloadTooLarge) = %q, want ErrPayloadTooLarge", got)
	}
	if got := relayErrorName(errTest); got != "ErrInvalidMessage" {
		t.Errorf("relayErrorName(other) = %q, want ErrInvalidMessage", got)
	}
}

func TestNewParticipant_RTPS(t *testing.T) {
	// Default (non-mock) path constructs an RTPS participant. Skip if the
	// platform cannot bind UDP (e.g. restricted CI sandboxes).
	c := &commonFlags{}
	p, err := c.newParticipant()
	if err != nil {
		t.Skipf("rtps participant unavailable on this host: %v", err)
	}
	_ = p.Close()
}

func TestPub_Interval(t *testing.T) {
	code, out, errOut := runCLI("", "pub", "-mock", "-topic", "cli/iv", "-count", "2", "-interval", "1ms")
	if code != 0 {
		t.Fatalf("exit = %d, stderr=%s", code, errOut)
	}
	if !strings.Contains(out, "[2/2]") {
		t.Errorf("interval pub output incomplete: %s", out)
	}
}

func TestIDL_WriteError(t *testing.T) {
	dir := t.TempDir()
	idlPath := filepath.Join(dir, "x.idl")
	if err := os.WriteFile(idlPath, []byte("module m { struct S { long a; }; };\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	// Output path inside a non-existent directory triggers a write error.
	bad := filepath.Join(dir, "nope", "out.go")
	if code, _, errOut := runCLI("", "idl", "-out", bad, idlPath); code != 1 {
		t.Errorf("write error: exit = %d, want 1 (stderr: %s)", code, errOut)
	}
}

func TestSend_SingleMessage(t *testing.T) {
	code, out, errOut := runCLI("", "send", "-mock", "-topic", "rt/send", "-payload", "hi")
	if code != 0 {
		t.Fatalf("exit = %d, stderr=%s", code, errOut)
	}
	if !strings.Contains(out, "sent: topic=rt/send") {
		t.Errorf("unexpected send output: %s", out)
	}
}

func TestSend_MissingTopic(t *testing.T) {
	// text mode without a topic is an error.
	if code, _, _ := runCLI("", "send", "-mock"); code != 1 {
		t.Errorf("send without topic: exit = %d, want 1", code)
	}
}

func TestSend_NDJSONSink(t *testing.T) {
	// Two relay.Message NDJSON lines on stdin → published, "sent 2".
	in := `{"protocol":2,"id":"rt/sink","payload":"aGk=","seq":1}
{"protocol":2,"id":"rt/sink","payload":"Ynll","seq":2}
`
	code, out, errOut := runCLI(in, "send", "--format", "json", "-mock")
	if code != 0 {
		t.Fatalf("exit = %d, stderr=%s", code, errOut)
	}
	if !strings.Contains(out, "sent 2 message(s)") {
		t.Errorf("unexpected sink output: %s", out)
	}
}

func TestSend_NDJSONSink_BadLine(t *testing.T) {
	code, _, errOut := runCLI("{not json", "send", "--format", "json", "-mock")
	if code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	if !strings.Contains(errOut, "decode NDJSON") {
		t.Errorf("stderr %q missing decode error", errOut)
	}
}

func TestSubscribe_JSON_NDJSON(t *testing.T) {
	const topic = "rt/subjson"
	p, err := mock.New(dds.Domain(0))
	if err != nil {
		t.Fatal(err)
	}
	pub, err := p.NewPublisher(topic, dds.DefaultQoS)
	if err != nil {
		t.Fatal(err)
	}
	stop := make(chan struct{})
	go func() {
		tk := time.NewTicker(10 * time.Millisecond)
		defer tk.Stop()
		for {
			select {
			case <-stop:
				return
			case <-tk.C:
				_ = pub.Write([]byte("hello"))
			}
		}
	}()

	code, out, errOut := runCLI("", "subscribe", "-mock", "-topic", topic, "-format", "json", "-count", "1", "-timeout", "3s")
	close(stop)
	_ = pub.Close()
	_ = p.Close()

	if code != 0 {
		t.Fatalf("exit = %d, stderr=%s", code, errOut)
	}
	// The emitted line must be a valid relay.Message (NDJSON).
	line := strings.TrimSpace(out)
	var m relay.Message
	if err := json.Unmarshal([]byte(line), &m); err != nil {
		t.Fatalf("subscribe output is not relay.Message NDJSON: %v\nout=%q", err, out)
	}
	if m.ID != topic || string(m.Payload) != "hello" {
		t.Errorf("got id=%q payload=%q, want %q/hello", m.ID, m.Payload, topic)
	}
}

func TestSubscribe_Errors(t *testing.T) {
	if code, _, _ := runCLI("", "subscribe", "-mock"); code != 1 {
		t.Errorf("missing topic: exit = %d, want 1", code)
	}
	if code, _, _ := runCLI("", "subscribe", "--nope"); code != 1 {
		t.Errorf("bad flag: exit = %d, want 1", code)
	}
}

func TestSubscribe_Timeout(t *testing.T) {
	code, _, errOut := runCLI("", "subscribe", "-mock", "-topic", "rt/idle2", "-timeout", "50ms")
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if !strings.Contains(errOut, "idle timeout") {
		t.Errorf("expected idle timeout, got %s", errOut)
	}
}

func TestPub_Mock(t *testing.T) {
	code, out, errOut := runCLI("", "pub", "-mock", "-topic", "cli/pub", "-payload", "hi", "-count", "2")
	if code != 0 {
		t.Fatalf("exit = %d, stderr=%s", code, errOut)
	}
	if !strings.Contains(out, "[1/2]") || !strings.Contains(out, "[2/2]") {
		t.Errorf("unexpected pub output: %s", out)
	}
}

func TestPub_Errors(t *testing.T) {
	if code, _, _ := runCLI("", "pub", "-mock"); code != 1 {
		t.Errorf("missing topic: exit = %d, want 1", code)
	}
	if code, _, _ := runCLI("", "pub", "--nope"); code != 1 {
		t.Errorf("bad flag: exit = %d, want 1", code)
	}
}

func TestSub_Mock_ReceivesSample(t *testing.T) {
	const topic = "cli/sub"
	// A background publisher writes until the subscriber under test exits.
	p, err := mock.New(dds.Domain(0))
	if err != nil {
		t.Fatal(err)
	}
	pub, err := p.NewPublisher(topic, dds.DefaultQoS)
	if err != nil {
		t.Fatal(err)
	}
	stop := make(chan struct{})
	go func() {
		ticker := time.NewTicker(10 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				_ = pub.Write([]byte("payload"))
			}
		}
	}()

	code, out, errOut := runCLI("", "sub", "-mock", "-topic", topic, "-count", "1", "-timeout", "3s")
	close(stop)
	_ = pub.Close()
	_ = p.Close()

	if code != 0 {
		t.Fatalf("exit = %d, stderr=%s", code, errOut)
	}
	if !strings.Contains(out, "subscribed: topic="+topic) {
		t.Errorf("missing subscribe banner: %s", out)
	}
	if !strings.Contains(out, "[1]") {
		t.Errorf("expected one received sample: %s", out)
	}
}

func TestSub_Timeout(t *testing.T) {
	code, _, errOut := runCLI("", "sub", "-mock", "-topic", "cli/idle", "-timeout", "50ms")
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if !strings.Contains(errOut, "idle timeout") {
		t.Errorf("expected idle-timeout message, got %s", errOut)
	}
}

func TestSub_Errors(t *testing.T) {
	if code, _, _ := runCLI("", "sub", "-mock"); code != 1 {
		t.Errorf("missing topic: exit = %d, want 1", code)
	}
	if code, _, _ := runCLI("", "sub", "--nope"); code != 1 {
		t.Errorf("bad flag: exit = %d, want 1", code)
	}
}

func TestDiscover_Mock(t *testing.T) {
	code, out, errOut := runCLI("", "discover", "-mock", "-wait", "10ms")
	if code != 0 {
		t.Fatalf("exit = %d, stderr=%s", code, errOut)
	}
	if !strings.Contains(out, "discovering: domain=0") {
		t.Errorf("unexpected discover output: %s", out)
	}
}

func TestDiscover_BadFlag(t *testing.T) {
	if code, _, _ := runCLI("", "discover", "--nope"); code != 1 {
		t.Errorf("bad flag: exit = %d, want 1", code)
	}
}

func TestIDL_StdoutAndFile(t *testing.T) {
	dir := t.TempDir()
	idlPath := filepath.Join(dir, "x.idl")
	const src = "module demo {\n  struct Point {\n    long x;\n    long y;\n  };\n};\n"
	if err := os.WriteFile(idlPath, []byte(src), 0o600); err != nil {
		t.Fatal(err)
	}

	// To stdout.
	code, out, errOut := runCLI("", "idl", idlPath)
	if code != 0 {
		t.Fatalf("idl stdout: exit = %d, stderr=%s", code, errOut)
	}
	if !strings.Contains(out, "type Point struct") {
		t.Errorf("generated source missing struct: %s", out)
	}

	// To a file with -package override.
	outPath := filepath.Join(dir, "x.go")
	code, _, errOut = runCLI("", "idl", "-out", outPath, "-package", "custom", idlPath)
	if code != 0 {
		t.Fatalf("idl file: exit = %d, stderr=%s", code, errOut)
	}
	gen, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(gen), "package custom") {
		t.Errorf("generated file missing package override: %s", gen)
	}
}

func TestIDL_Errors(t *testing.T) {
	if code, _, _ := runCLI("", "idl"); code != 1 {
		t.Errorf("no input: exit = %d, want 1", code)
	}
	if code, _, _ := runCLI("", "idl", "--nope"); code != 1 {
		t.Errorf("bad flag: exit = %d, want 1", code)
	}
	if code, _, errOut := runCLI("", "idl", "/no/such/file.idl"); code != 1 {
		t.Errorf("missing file: exit = %d, want 1 (stderr: %s)", code, errOut)
	}
}
