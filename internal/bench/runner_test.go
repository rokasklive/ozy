package bench

import (
	"context"
	"io"
	"os/exec"
	"testing"
	"time"
)

// TestSetProcessGroupKillReapsChildren reproduces the run-hang bug: a process
// that spawns a background grandchild which inherits stdout and outlives it. With
// the default CommandContext teardown, killing only the parent orphans the
// grandchild, which holds the pipe open and makes cmd.Wait() block until the
// grandchild exits (here 30s) — the "run stuck past its timeout" symptom.
// setProcessGroupKill must tear the whole group down promptly.
func TestSetProcessGroupKillReapsChildren(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	// `sleep 30 &` runs in the shell's process group (non-interactive sh has no
	// job control), so a group-wide kill reaps it; the trailing `sleep 30` keeps
	// the parent alive past the 1s timeout so cancel is what ends it.
	cmd := exec.CommandContext(ctx, "sh", "-c", "sleep 30 & echo started; sleep 30")
	setProcessGroupKill(cmd)

	pr, pw := io.Pipe()
	cmd.Stdout = pw
	cmd.Stderr = pw
	teeDone := make(chan struct{})
	go func() {
		defer close(teeDone)
		_, _ = io.Copy(io.Discard, pr)
	}()

	if err := cmd.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}

	start := time.Now()
	_ = cmd.Wait()
	_ = pw.Close()
	<-teeDone
	elapsed := time.Since(start)

	// A fast group kill returns ~immediately after the 1s timeout — well under the
	// grandchild's 30s sleep and under the 10s WaitDelay backstop. Exceeding 8s
	// means the child was orphaned and the run would hang.
	if elapsed > 8*time.Second {
		t.Fatalf("cmd.Wait took %v — process group was not reaped (the run would hang past its timeout)", elapsed)
	}
}
