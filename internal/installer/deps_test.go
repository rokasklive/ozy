package installer

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func fakeLook(present map[string]bool) lookPath {
	return func(name string) (string, error) {
		if present[name] {
			return "/usr/bin/" + name, nil
		}
		return "", errors.New("not found")
	}
}

func TestStatusFor(t *testing.T) {
	cases := []struct {
		detected           string
		minMajor, minMinor int
		want               DepStatus
	}{
		{"3.11.0", 3, 11, DepOK},
		{"3.12", 3, 11, DepOK},
		{"3.9.1", 3, 11, DepOutOfDate},
		{"2.7", 3, 11, DepOutOfDate},
		{"go1.26.4", 1, 26, DepOK},
		{"", 3, 11, DepMissing},
		{"weird", 3, 11, DepMissing},
	}
	for _, tc := range cases {
		if got := statusFor(tc.detected, tc.minMajor, tc.minMinor); got != tc.want {
			t.Errorf("statusFor(%q, %d.%d) = %v, want %v", tc.detected, tc.minMajor, tc.minMinor, got, tc.want)
		}
	}
}

func TestDepCheckerDetectsPresent(t *testing.T) {
	d := DepChecker{
		look: fakeLook(map[string]bool{"git": true, "python3": true, "sqlite3": true}),
		run: func(name string, _ ...string) (string, error) {
			switch name {
			case "go":
				return "go version go1.26.4 darwin/arm64", nil
			case "git":
				return "git version 2.40.0", nil
			case "python3":
				return "Python 3.11.6", nil
			case "sqlite3":
				return "3.45.0 2024-01-15", nil
			}
			return "", errors.New("unexpected " + name)
		},
	}
	byName := indexDeps(d.Check())
	if byName["Python"].Status != DepOK {
		t.Errorf("Python status = %v, want ok", byName["Python"].Status)
	}
	if byName["Semantic backend"].Status != DepOK {
		t.Errorf("Semantic backend should be available when Python is present")
	}
	if byName["Go toolchain"].Status != DepOK {
		t.Errorf("Go status = %v, want ok", byName["Go toolchain"].Status)
	}
}

func TestDepCheckerPythonMissingDegrades(t *testing.T) {
	d := DepChecker{
		look: fakeLook(map[string]bool{"git": true}), // no uv/python3/python
		run: func(name string, _ ...string) (string, error) {
			if name == "go" {
				return "go version go1.26.4 linux/amd64", nil
			}
			if name == "git" {
				return "git version 2.40.0", nil
			}
			return "", errors.New("not found")
		},
	}
	byName := indexDeps(d.Check())
	if byName["Python"].Status != DepMissing {
		t.Errorf("Python should be missing, got %v", byName["Python"].Status)
	}
	if byName["Semantic backend"].Status != DepMissing {
		t.Errorf("Semantic backend should be missing when Python is absent")
	}
	// The fallback text must name lexical-only so the report is actionable.
	if !strings.Contains(byName["Python"].Fallback, "lexical") {
		t.Errorf("Python fallback should mention lexical-only: %q", byName["Python"].Fallback)
	}
}

func TestRenderDepsTable(t *testing.T) {
	var buf bytes.Buffer
	renderDeps(&buf, []Dependency{
		{Name: "Python", Required: false, Detected: "3.11.6", Wanted: "3.11+", Status: DepOK, Fallback: "lexical-only"},
		{Name: "Go toolchain", Required: true, Detected: "", Wanted: "1.26+", Status: DepMissing, Fallback: "install Go"},
	})
	out := buf.String()
	for _, want := range []string{"NAME", "DETECTED", "STATUS", "Python", "3.11.6", "missing", "optional", "required"} {
		if !strings.Contains(out, want) {
			t.Errorf("dep table missing %q\n%s", want, out)
		}
	}
}

func indexDeps(deps []Dependency) map[string]Dependency {
	m := make(map[string]Dependency, len(deps))
	for _, d := range deps {
		m[d.Name] = d
	}
	return m
}
