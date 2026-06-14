package installer

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestRedact(t *testing.T) {
	cases := []struct {
		in       string
		mustGone string // substring that must not survive ("" = nothing to check)
		want     string // exact expected when deterministic ("" = skip exact)
	}{
		{"token=abc123", "abc123", "token=****"},
		{"--api-key SECRET123 done", "SECRET123", ""},
		{"Authorization: Bearer xyzsecret", "xyzsecret", ""},
		{"password: hunter2", "hunter2", "password: ****"},
		{"installing fastembed==0.8.0", "", "installing fastembed==0.8.0"}, // benign untouched
	}
	for _, tc := range cases {
		got := Redact(tc.in)
		if tc.mustGone != "" && strings.Contains(got, tc.mustGone) {
			t.Errorf("Redact(%q) = %q, still contains secret %q", tc.in, got, tc.mustGone)
		}
		if tc.want != "" && got != tc.want {
			t.Errorf("Redact(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestLoggerWritesFileAndRedacts(t *testing.T) {
	dir := t.TempDir()
	var term bytes.Buffer
	lg, err := NewLogger(dir, "install", &term)
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}

	lg.Logf("step InstallOzyBinary started")
	lg.Sayf("downloading with token=supersecret")
	if err := lg.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	data, err := os.ReadFile(lg.Path())
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	log := string(data)

	if !strings.Contains(log, "InstallOzyBinary") {
		t.Error("file log missing the Logf line")
	}
	if strings.Contains(log, "supersecret") {
		t.Error("secret leaked into the log file")
	}
	// Logf goes to the file only; Sayf goes to both.
	if strings.Contains(term.String(), "InstallOzyBinary") {
		t.Error("Logf must not write to the terminal")
	}
	if !strings.Contains(term.String(), "downloading") {
		t.Error("Sayf should write to the terminal")
	}
	if strings.Contains(term.String(), "supersecret") {
		t.Error("secret leaked into the terminal stream")
	}
}
