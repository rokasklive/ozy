package installer

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/rokasklive/ozy/internal/paths"
)

// TestRunDryRunMakesNoMutations drives the public entrypoint with --dry-run in a
// fully isolated HOME/XDG sandbox and asserts that not a single file is created.
func TestRunDryRunMakesNoMutations(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "cfg"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(dir, "data"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(dir, "state"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(dir, "cache"))
	t.Setenv("APPDATA", filepath.Join(dir, "AppData", "Roaming"))
	t.Setenv("LOCALAPPDATA", filepath.Join(dir, "AppData", "Local"))
	t.Setenv("OZY_CONFIG", "")

	// Swallow the plan printed to stdout so the test output stays clean.
	restore := swapStdout(t)
	err := Run(Options{DryRun: true})
	restore()
	if err != nil {
		t.Fatalf("Run(--dry-run): %v", err)
	}

	// No Ozy-managed location may exist. (Go's own telemetry under ~/Library is
	// written by the `go version` dependency probe, not by the installer, so we
	// assert on the resolved Ozy paths rather than the whole HOME.)
	p := paths.Resolve()
	for _, loc := range []string{p.ConfigDir, p.ConfigFile, p.DataDir, p.StateDir, p.CacheDir, p.UserBinDir, p.LogDir} {
		if _, err := os.Stat(loc); err == nil {
			t.Errorf("--dry-run created an Ozy location: %s", loc)
		}
	}
}

// swapStdout redirects os.Stdout to a discarding pipe and returns a restore func.
func swapStdout(t *testing.T) func() {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = w
	done := make(chan struct{})
	go func() { _, _ = io.Copy(io.Discard, r); close(done) }()
	return func() {
		os.Stdout = old
		w.Close()
		<-done
		r.Close()
	}
}
