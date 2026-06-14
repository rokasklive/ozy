package sidecar

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"time"
)

// ScriptedResponse is the on-the-wire response a ScriptedSidecar
// generates for a single request. The zero ScriptedResponse with
// Response=nil and Garbage="" means "close stdout and exit
// successfully" — useful for tests that simulate the sidecar
// crashing after a successful first response.
type ScriptedResponse struct {
	// Response is the JSON object to write back as the response.
	// nil means "do not write a response" (combine with Exit to
	// simulate a hung sidecar).
	Response map[string]any

	// Garbage, when non-empty, is written verbatim instead of
	// Response. A trailing newline is appended. Use it to drive
	// the "garbage response" failure path.
	Garbage string

	// Delay pauses before the response is written. It exists so
	// tests can exercise the request-timeout path without making
	// the test depend on real wall-clock time for the body of
	// the call.
	Delay time.Duration

	// Exit causes the driver to close stdout and stop processing
	// further requests after this response is written. The next
	// Client call will see EOF on stdout and become unavailable.
	Exit bool
}

// ScriptedSidecar is the test-side programmable sidecar. Tests
// supply an OnRequest function that maps each incoming request to a
// scripted response.
type ScriptedSidecar struct {
	// OnRequest is invoked once per request. It must not block on
	// external resources for the typical "happy path" test; the
	// "timeout" test deliberately blocks inside OnRequest to
	// exercise the request-timeout path.
	OnRequest func(req map[string]any) ScriptedResponse
}

// fakeDriver is the in-memory replacement for realDriver used by
// tests. It runs a goroutine that reads JSONL from the "client
// stdin" pipe, dispatches to a ScriptedSidecar, and writes the
// scripted response (or garbage, or nothing) to the "client stdout"
// pipe.
type fakeDriver struct {
	scripted *ScriptedSidecar

	clientStdinWriter *io.PipeWriter
	fakeStdinReader   *io.PipeReader
	fakeStdoutWriter  *io.PipeWriter
	clientStdoutRead  *io.PipeReader
	fakeStderrWriter  *io.PipeWriter
	clientStderrRead  *io.PipeReader

	done    chan struct{}
	exitErr error

	closeOnce sync.Once
}

// newFakeDriver constructs a fakeDriver with the supplied scripted
// behavior. The driver is not started until Start is called.
func newFakeDriver(s *ScriptedSidecar) *fakeDriver {
	if s == nil {
		s = &ScriptedSidecar{}
	}
	if s.OnRequest == nil {
		s.OnRequest = func(map[string]any) ScriptedResponse { return ScriptedResponse{} }
	}
	return &fakeDriver{scripted: s}
}

// Start wires the in-memory pipe pairs and launches the goroutine
// that emulates the sidecar. The pipes are arranged so the Client
// can write to clientStdinWriter and read from clientStdoutRead —
// mirroring the real os/exec wiring — while the emulated sidecar
// reads from fakeStdinReader and writes to fakeStdoutWriter.
func (d *fakeDriver) Start(ctx context.Context) (io.WriteCloser, io.ReadCloser, io.ReadCloser, func() error, error) {
	d.fakeStdinReader, d.clientStdinWriter = io.Pipe()
	d.clientStdoutRead, d.fakeStdoutWriter = io.Pipe()
	d.clientStderrRead, d.fakeStderrWriter = io.Pipe()
	d.done = make(chan struct{})

	go d.run(ctx)
	return d.clientStdinWriter, d.clientStdoutRead, d.clientStderrRead, d.wait, nil
}

func (d *fakeDriver) run(ctx context.Context) {
	defer close(d.done)
	defer func() { _ = d.fakeStdoutWriter.Close() }()
	defer func() { _ = d.fakeStderrWriter.Close() }()

	scanner := bufio.NewScanner(d.fakeStdinReader)
	scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var req map[string]any
		_ = json.Unmarshal(line, &req)

		var resp ScriptedResponse
		if d.scripted.OnRequest != nil {
			resp = d.scripted.OnRequest(req)
		}

		if resp.Delay > 0 {
			select {
			case <-time.After(resp.Delay):
			case <-ctx.Done():
				return
			}
		}

		switch {
		case resp.Garbage != "":
			_, _ = io.WriteString(d.fakeStdoutWriter, resp.Garbage)
			if len(resp.Garbage) > 0 && resp.Garbage[len(resp.Garbage)-1] != '\n' {
				_, _ = d.fakeStdoutWriter.Write([]byte{'\n'})
			}
		case resp.Response == nil:
			return
		default:
			data, err := json.Marshal(resp.Response)
			if err != nil {
				_, _ = fmt.Fprintf(d.fakeStderrWriter, "fake: marshal error: %v\n", err)
				return
			}
			data = append(data, '\n')
			if _, err := d.fakeStdoutWriter.Write(data); err != nil {
				return
			}
		}

		if resp.Exit {
			return
		}
	}
}

// Close terminates the fake sidecar. It is idempotent. The Client
// 's reader will see EOF on stdout and the Client will be marked
// unavailable.
func (d *fakeDriver) Close() error {
	d.closeOnce.Do(func() {
		if d.clientStdinWriter != nil {
			_ = d.clientStdinWriter.Close()
		}
		if d.fakeStdoutWriter != nil {
			_ = d.fakeStdoutWriter.Close()
		}
		if d.fakeStderrWriter != nil {
			_ = d.fakeStderrWriter.Close()
		}
	})
	return nil
}

func (d *fakeDriver) wait() error {
	<-d.done
	return d.exitErr
}
