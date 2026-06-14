package installer

import (
	"bytes"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rokasklive/ozy/internal/paths"
)

// fakeDeps returns a DepChecker whose detection is fully controlled: semantic
// available or not, with Go/Git present so required checks pass.
func fakeDeps(semantic bool) DepChecker {
	look := map[string]bool{"git": true}
	if semantic {
		look["python3"] = true
	}
	return DepChecker{
		look: fakeLook(look),
		run: func(name string, _ ...string) (string, error) {
			switch name {
			case "go":
				return "go version go1.26.4 linux/amd64", nil
			case "git":
				return "git version 2.40.0", nil
			case "python3":
				return "Python 3.11.6", nil
			}
			return "", errors.New("absent")
		},
	}
}

func tempPaths(t *testing.T) paths.Paths {
	t.Helper()
	dir := t.TempDir()
	return paths.Paths{
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
}

func TestBuildPlanSemanticAvailable(t *testing.T) {
	plat := Platform{OS: "linux", Arch: "amd64"}
	plan := BuildPlan(plat, tempPaths(t), fakeDeps(true))

	if plan.IsUpdate {
		t.Error("fresh temp paths should not be flagged as an update")
	}
	if plan.PathChange == "" {
		t.Error("temp bin dir is not on PATH, so a PATH change should be planned")
	}
	if !plan.SemanticPlanned() {
		t.Error("semantic should be planned when Python is present")
	}
	if len(plan.Downloads) == 0 || plan.EstDiskBytes == 0 {
		t.Error("semantic plan should list downloads and a disk estimate")
	}
	if len(plan.Steps) != len(stepNames()) {
		t.Errorf("plan steps = %d, want %d", len(plan.Steps), len(stepNames()))
	}
}

func TestBuildPlanLexicalOnlyWarns(t *testing.T) {
	plan := BuildPlan(Platform{OS: "linux"}, tempPaths(t), fakeDeps(false))
	if plan.SemanticPlanned() {
		t.Error("semantic must not be planned without Python")
	}
	if len(plan.Downloads) != 0 {
		t.Error("lexical-only plan should schedule no downloads")
	}
	if len(plan.Warnings) == 0 {
		t.Error("lexical-only plan should warn about the degraded mode")
	}
}

func TestRenderPlanEndsWithNothingChanged(t *testing.T) {
	var buf bytes.Buffer
	plan := BuildPlan(Platform{OS: "linux", Arch: "amd64"}, tempPaths(t), fakeDeps(true))
	RenderPlan(&buf, plan)
	out := buf.String()
	if !strings.Contains(out, "Nothing has changed yet.") {
		t.Error("plan must end by stating nothing has changed yet")
	}
	if !strings.Contains(out, "Locations:") || !strings.Contains(out, "Planned actions:") {
		t.Error("plan must enumerate locations and planned actions")
	}
}

func TestBuildPlanDetectsExistingConfig(t *testing.T) {
	p := tempPaths(t)
	mustMkConfig(t, p.ConfigFile)
	plan := BuildPlan(Platform{OS: "linux"}, p, fakeDeps(false))
	if !plan.IsUpdate || plan.ExistingConfig == "" {
		t.Error("an existing config should mark the run as an update")
	}
}
