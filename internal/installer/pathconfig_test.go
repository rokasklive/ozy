package installer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAppendBlockOnceIdempotent(t *testing.T) {
	rc := filepath.Join(t.TempDir(), ".zshrc")
	if err := os.WriteFile(rc, []byte("# existing\nexport FOO=1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	block := pathBlock("/home/u/.local/bin", "zsh")
	if err := appendBlockOnce(rc, block); err != nil {
		t.Fatalf("first append: %v", err)
	}
	if err := appendBlockOnce(rc, block); err != nil {
		t.Fatalf("second append: %v", err)
	}
	data, _ := os.ReadFile(rc)
	if n := strings.Count(string(data), pathBlockBegin); n != 1 {
		t.Errorf("block appears %d times, want exactly 1 (idempotent)", n)
	}
	if !strings.Contains(string(data), "export FOO=1") {
		t.Error("existing rc content must be preserved")
	}
}

func TestRemovePathBlock(t *testing.T) {
	rc := filepath.Join(t.TempDir(), ".bashrc")
	if err := os.WriteFile(rc, []byte("# existing\nexport FOO=1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := appendBlockOnce(rc, pathBlock("/home/u/.local/bin", "bash")); err != nil {
		t.Fatal(err)
	}

	removed, err := removePathBlock(rc)
	if err != nil || !removed {
		t.Fatalf("remove: removed=%v err=%v", removed, err)
	}
	data, _ := os.ReadFile(rc)
	if strings.Contains(string(data), pathBlockBegin) || strings.Contains(string(data), ".local/bin") {
		t.Errorf("installer block not fully removed:\n%s", data)
	}
	if !strings.Contains(string(data), "export FOO=1") {
		t.Error("unrelated rc content must survive removal")
	}

	// Removing again is a safe no-op (idempotent uninstall reruns).
	again, err := removePathBlock(rc)
	if err != nil || again {
		t.Errorf("second remove should be a no-op: removed=%v err=%v", again, err)
	}
}

func TestRemovePathBlockMissingFileIsNoOp(t *testing.T) {
	removed, err := removePathBlock(filepath.Join(t.TempDir(), "nonexistent"))
	if err != nil || removed {
		t.Errorf("missing file: removed=%v err=%v, want false,nil", removed, err)
	}
}

func TestWindowsPathEditIsUserScopeOnly(t *testing.T) {
	script := winPathScript(`C:\Users\u\AppData\Local\Ozy\bin`)
	// The load-bearing guarantee: edit the User PATH, never the Machine PATH
	// (which would need admin rights).
	if !strings.Contains(script, "'User'") {
		t.Errorf("windows PATH edit must target User scope: %q", script)
	}
	if strings.Contains(script, "Machine") {
		t.Errorf("windows PATH edit must never touch Machine scope: %q", script)
	}
	if !strings.Contains(winPathCommand(`C:\x`), "powershell") {
		t.Errorf("windows fallback command should invoke powershell: %q", winPathCommand(`C:\x`))
	}
}

func TestRcFileForShells(t *testing.T) {
	cases := []struct {
		shell, wantSuffix, wantTag string
	}{
		{"/bin/zsh", ".zshrc", "zsh"},
		{"/usr/bin/bash", ".bashrc", "bash"},
		{"/usr/local/bin/fish", filepath.Join(".config", "fish", "config.fish"), "fish"},
		{"/bin/sh", ".profile", "sh"},
		{"", ".profile", "sh"},
	}
	for _, tc := range cases {
		file, tag := rcFileFor(tc.shell, "/home/u")
		if !strings.HasSuffix(file, tc.wantSuffix) {
			t.Errorf("rcFileFor(%q) file=%q, want suffix %q", tc.shell, file, tc.wantSuffix)
		}
		if tag != tc.wantTag {
			t.Errorf("rcFileFor(%q) tag=%q, want %q", tc.shell, tag, tc.wantTag)
		}
	}
}

func TestPathExportLineSyntax(t *testing.T) {
	if got := pathExportLine("/x/bin", "fish"); !strings.Contains(got, "fish_add_path") {
		t.Errorf("fish syntax = %q, want fish_add_path", got)
	}
	if got := pathExportLine("/x/bin", "zsh"); !strings.Contains(got, `export PATH="/x/bin`) {
		t.Errorf("posix syntax = %q", got)
	}
}
