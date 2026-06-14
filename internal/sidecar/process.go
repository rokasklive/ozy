package sidecar

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"
)

// Logger receives the sidecar's stderr output, one line per message.
// A nil Logger drops the messages silently. The default Client value
// is a no-op Logger; callers that want diagnostics pass a real one
// (e.g. wrapping a *slog.Logger or an *slog.Default()).
type Logger interface {
	// Log receives one line of stderr output, with the trailing
	// newline already stripped. Implementations must not block.
	Log(line string)
}

// logFunc adapts a plain function to the Logger interface so callers
// can pass a closure such as func(line string) { log.Print(line) }.
type logFunc func(line string)

func (f logFunc) Log(line string) { f(line) }

// ProcessOptions configures a realDriver — the subprocess that actually
// runs the Python sidecar. A zero ProcessOptions is invalid: at minimum
// PythonPath must be set.
type ProcessOptions struct {
	PythonPath string
	SourceDir  string
	DataDir    string
	Backend    string
	Model      string
	ExtraEnv   []string
}

// driver is the abstraction a Client uses to talk to the sidecar.
// realDriver wraps a real subprocess; fakeDriver (in fake.go) wraps
// in-memory pipes driven by a ScriptedSidecar.
//
// Start is called once per Client lifetime. It returns the writer for
// stdin, the reader for stdout, the reader for stderr, and a Wait
// function that blocks until the process exits (returning its error).
// Close terminates the process; Wait may then return.
//
// Implementations must:
//   - return non-nil pipes for stdin/stdout/stderr
//   - allow Close to be called multiple times (idempotent)
//   - close stdout (causing the reader to see EOF) when the process exits
type driver interface {
	Start(ctx context.Context) (stdin io.WriteCloser, stdout io.ReadCloser, stderr io.ReadCloser, wait func() error, err error)
	Close() error
}

// realDriver owns a real os/exec subprocess running the Python sidecar.
type realDriver struct {
	opts    ProcessOptions
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	stdout  io.ReadCloser
	stderr  io.ReadCloser
	closeMu sync.Mutex
	closed  bool
}

// newRealDriver constructs a realDriver. The process is not started until
// Start is called.
func newRealDriver(opts ProcessOptions) *realDriver {
	return &realDriver{opts: opts}
}

// Start spawns the sidecar subprocess. The working directory is the
// resolved source directory so "python -m sidecar" can import the
// package. The child inherits the parent environment plus any
// ProcessOptions.ExtraEnv entries.
func (d *realDriver) Start(ctx context.Context) (io.WriteCloser, io.ReadCloser, io.ReadCloser, func() error, error) {
	if d.opts.PythonPath == "" {
		return nil, nil, nil, nil, errors.New("sidecar: PythonPath is required")
	}
	if d.opts.SourceDir == "" {
		return nil, nil, nil, nil, errors.New("sidecar: SourceDir is required")
	}
	if d.opts.DataDir == "" {
		return nil, nil, nil, nil, errors.New("sidecar: DataDir is required")
	}
	if _, err := os.Stat(d.opts.SourceDir); err != nil {
		return nil, nil, nil, nil, fmt.Errorf("sidecar: source dir not found: %w", err)
	}

	args := []string{
		"-m", "sidecar",
		"--data-dir", d.opts.DataDir,
		"--backend", d.opts.Backend,
		"--model", d.opts.Model,
	}
	cmd := exec.CommandContext(ctx, d.opts.PythonPath, args...)
	cmd.Dir = d.opts.SourceDir
	if len(d.opts.ExtraEnv) > 0 {
		cmd.Env = append(os.Environ(), d.opts.ExtraEnv...)
	}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("sidecar: stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return nil, nil, nil, nil, fmt.Errorf("sidecar: stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		_ = stdin.Close()
		_ = stdout.Close()
		return nil, nil, nil, nil, fmt.Errorf("sidecar: stderr pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		_ = stdout.Close()
		_ = stderr.Close()
		return nil, nil, nil, nil, fmt.Errorf("sidecar: start: %w", err)
	}
	d.cmd = cmd
	d.stdin = stdin
	d.stdout = stdout
	d.stderr = stderr
	return stdin, stdout, stderr, cmd.Wait, nil
}

// Close terminates the sidecar. It sends SIGTERM first, escalates to
// SIGKILL after two seconds, and then closes the pipes so any blocked
// readers/writers unblock. Close is idempotent and safe to call from
// multiple goroutines.
func (d *realDriver) Close() error {
	d.closeMu.Lock()
	defer d.closeMu.Unlock()
	if d.closed {
		return nil
	}
	d.closed = true

	if d.cmd == nil || d.cmd.Process == nil {
		d.closePipes()
		return nil
	}

	if d.stdin != nil {
		_ = d.stdin.Close()
	}
	if err := d.cmd.Process.Signal(syscall.SIGTERM); err != nil && !errors.Is(err, os.ErrProcessDone) {
		_ = err
	}

	done := make(chan struct{})
	go func() {
		_ = d.cmd.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		_ = d.cmd.Process.Signal(syscall.SIGKILL)
		<-done
	}
	d.closePipes()
	return nil
}

func (d *realDriver) closePipes() {
	if d.stdin != nil {
		_ = d.stdin.Close()
	}
	if d.stdout != nil {
		_ = d.stdout.Close()
	}
	if d.stderr != nil {
		_ = d.stderr.Close()
	}
}

// drainStderr copies the child's stderr into the supplied Logger. The
// scanner error is intentionally dropped because a closing pipe is
// the normal termination (process exit), not a failure.
func drainStderr(r io.ReadCloser, logger Logger) {
	if r == nil {
		return
	}
	defer func() { _ = r.Close() }()
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		if logger == nil {
			continue
		}
		line := scanner.Text()
		if line == "" {
			continue
		}
		logger.Log(line)
	}
}
