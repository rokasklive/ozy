package installer

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/rokasklive/ozy/internal/paths"
)

// newTestCtx builds an execContext backed by a temp log + state store, with no
// real terminal. Callers override fields (consent, plan, paths, run) as needed.
func newTestCtx(t *testing.T) *execContext {
	t.Helper()
	dir := t.TempDir()
	log, err := NewLogger(filepath.Join(dir, "logs"), "test", io.Discard)
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}
	t.Cleanup(func() { log.Close() })
	store := NewStateStore(filepath.Join(dir, "state.json"))
	state, _ := store.Load()
	return &execContext{
		ctx:   context.Background(),
		log:   log,
		stdin: strings.NewReader(""),
		term:  io.Discard,
		prog:  NewProgress(io.Discard, Platform{}),
		state: state,
		store: store,
		plan:  &Plan{Platform: Platform{OS: runtime.GOOS}},
		paths: paths.Resolve(),
	}
}

func TestExecuteResumesAndRevalidates(t *testing.T) {
	c := newTestCtx(t)

	var aRuns, bRuns int
	aValid := true
	steps := func(failB bool) []step {
		return []step{
			{name: "A", title: "A",
				valid: func(*execContext) bool { return aValid },
				run:   func(*execContext) error { aRuns++; return nil }},
			{name: "B", title: "B",
				run: func(*execContext) error {
					bRuns++
					if failB {
						return errors.New("boom")
					}
					return nil
				}},
		}
	}

	// First run: B fails. A is recorded done; B is not.
	var se *StepError
	if err := c.execute(steps(true)); !errors.As(err, &se) {
		t.Fatalf("want *StepError, got %v", err)
	}
	if se.Step != "B" || !se.SafeRetry || se.LogPath == "" {
		t.Errorf("actionable error missing fields: %+v", se)
	}
	if aRuns != 1 || bRuns != 1 {
		t.Fatalf("first run: a=%d b=%d, want 1,1", aRuns, bRuns)
	}
	if !c.state.Done("A") || c.state.Done("B") {
		t.Fatal("A should be done, B should not")
	}

	// Rerun with B fixed: A is done+valid → skipped; B resumes.
	if err := c.execute(steps(false)); err != nil {
		t.Fatalf("rerun: %v", err)
	}
	if aRuns != 1 {
		t.Errorf("A re-ran (%d); a completed-and-valid step must be skipped", aRuns)
	}
	if bRuns != 2 {
		t.Errorf("B runs=%d, want 2 (resumed from the failed step)", bRuns)
	}

	// Stale revalidation: A recorded done but its output no longer validates.
	aValid = false
	if err := c.execute(steps(false)); err != nil {
		t.Fatalf("stale rerun: %v", err)
	}
	if aRuns != 2 {
		t.Errorf("A runs=%d; a stale completed step must re-execute", aRuns)
	}
}

// TestInstallOzyBinaryLocalBuild proves a dev build compiles from the local
// checkout (go build) rather than installing a published tag (go install) — the
// path that lets the flow run before anything is published.
func TestInstallOzyBinaryLocalBuild(t *testing.T) {
	dir := t.TempDir()
	moduleDir := filepath.Join(dir, "repo")
	pkgDir := filepath.Join(moduleDir, "cmd", "ozy")
	if err := os.MkdirAll(pkgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	gomod := filepath.Join(moduleDir, "go.mod")
	if err := os.WriteFile(gomod, []byte("module github.com/rokasklive/ozy\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	binPath := filepath.Join(dir, "bin", "ozy")
	if err := os.MkdirAll(filepath.Dir(binPath), 0o755); err != nil {
		t.Fatal(err)
	}

	var didBuild, didInstall bool
	c := newTestCtx(t)
	c.paths.BinaryPath = binPath
	c.paths.UserBinDir = filepath.Dir(binPath)
	c.run = func(name string, args ...string) (string, error) {
		switch {
		case name == "go" && len(args) > 0 && args[0] == "env":
			return gomod + "\n", nil
		case name == "go" && len(args) > 0 && args[0] == "build":
			didBuild = true
			_ = os.WriteFile(binPath, []byte("bin"), 0o755)
			return "", nil
		case name == "go" && len(args) > 0 && args[0] == "install":
			didInstall = true
			return "", nil
		}
		return "", nil
	}

	if err := stepInstallOzyBinary(c); err != nil {
		t.Fatalf("stepInstallOzyBinary: %v", err)
	}
	if !didBuild || didInstall {
		t.Errorf("dev build should `go build` locally, not `go install`: build=%v install=%v", didBuild, didInstall)
	}
	if !fileExists(binPath) {
		t.Error("local build did not produce the binary")
	}
}

func TestAllowConsentBoundary(t *testing.T) {
	// --yes must NOT auto-accept a risky action, even non-interactively.
	c := newTestCtx(t)
	c.consent = ConsentPolicy{AssumeYes: true, Interactive: false}
	if c.allow(Risky, "edit shell profile") {
		t.Error("risky action proceeded under --yes without real consent")
	}
	// Ordinary under --yes proceeds.
	if !c.allow(Ordinary, "create managed dir") {
		t.Error("ordinary action should proceed under --yes")
	}

	// Interactive consent: 'y' proceeds, 'n' is skipped.
	yes := newTestCtx(t)
	yes.consent = ConsentPolicy{Interactive: true}
	yes.stdin = strings.NewReader("y\n")
	if !yes.allow(Risky, "download asset") {
		t.Error("risky action should proceed when the user consents")
	}
	no := newTestCtx(t)
	no.consent = ConsentPolicy{Interactive: true}
	no.stdin = strings.NewReader("n\n")
	if no.allow(Risky, "download asset") {
		t.Error("risky action must not proceed when the user declines")
	}
}

// TestInstallStepsHappyPath drives the full step machine with fake exec and a
// temp filesystem: no real go install, no network, no real terminal. It proves
// the steps wire together and only touch the resolved (temp) locations.
func TestInstallStepsHappyPath(t *testing.T) {
	dir := t.TempDir()
	p := paths.Paths{
		UserBinDir: filepath.Join(dir, "bin"),
		ConfigDir:  filepath.Join(dir, "cfg"),
		ConfigFile: filepath.Join(dir, "cfg", "ozy.jsonc"),
		DataDir:    filepath.Join(dir, "data"),
		CacheDir:   filepath.Join(dir, "cache"),
		StateDir:   filepath.Join(dir, "state"),
		LogDir:     filepath.Join(dir, "state", "logs"),
		VenvDir:    filepath.Join(dir, "state", "venv"),
		BinaryPath: filepath.Join(dir, "bin", "ozy"),
	}
	// Semantic backend "missing" so SetupPythonEnvironment degrades without any
	// real provisioning or network.
	plan := Plan{
		Platform: Platform{OS: runtime.GOOS},
		Paths:    p,
		Deps:     []Dependency{{Name: "Semantic backend", Status: DepMissing}},
	}

	// A fake module checkout so the dev local-build path resolves.
	pkgDir := filepath.Join(dir, "repo", "cmd", "ozy")
	if err := os.MkdirAll(pkgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	gomod := filepath.Join(dir, "repo", "go.mod")
	if err := os.WriteFile(gomod, []byte("module github.com/rokasklive/ozy\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	fakeRun := func(name string, args ...string) (string, error) {
		switch {
		case name == "go" && len(args) > 0 && args[0] == "env":
			return gomod + "\n", nil
		case name == "go" && len(args) > 0 && args[0] == "build":
			// Simulate the toolchain producing the binary from local source.
			if err := os.WriteFile(p.BinaryPath, []byte("#!/bin/sh\n"), 0o755); err != nil {
				return "", err
			}
			return "", nil
		case filepath.Base(name) == "ozy" && len(args) > 0 && args[0] == "doctor":
			return "all checks passed", nil
		}
		return "", nil
	}

	c := newTestCtx(t)
	c.plan = &plan
	c.paths = p
	c.run = fakeRun
	c.consent = ConsentPolicy{} // non-interactive: risky PATH edit is skipped, not performed

	if err := c.execute(installSteps()); err != nil {
		t.Fatalf("happy path failed: %v", err)
	}

	for _, d := range installDirs(p) {
		if !dirExists(d) {
			t.Errorf("dir not created: %s", d)
		}
	}
	if !fileExists(p.ConfigFile) {
		t.Error("config not written")
	}
	if !fileExists(p.BinaryPath) {
		t.Error("binary not installed")
	}
	if !c.doctorOK {
		t.Error("doctor should have passed")
	}
	// Everything stayed inside the temp dir.
	if entries, _ := os.ReadDir(dir); len(entries) == 0 {
		t.Error("expected install artifacts under temp dir")
	}
}
