package installer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rokasklive/ozy/internal/paths"
)

// sandboxPaths points HOME/XDG at a temp dir and returns the resolved paths, so
// Uninstall (which calls paths.Resolve internally) and the test agree.
func sandboxPaths(t *testing.T) paths.Paths {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "cfg"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(dir, "data"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(dir, "state"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(dir, "cache"))
	// Windows resolves via APPDATA/LOCALAPPDATA, so the sandbox overrides those
	// too — otherwise these tests would touch the real %APPDATA%\Ozy on Windows.
	t.Setenv("APPDATA", filepath.Join(dir, "AppData", "Roaming"))
	t.Setenv("LOCALAPPDATA", filepath.Join(dir, "AppData", "Local"))
	t.Setenv("OZY_CONFIG", "")
	t.Setenv("SHELL", "/bin/zsh")
	return paths.Resolve()
}

func populate(t *testing.T, p paths.Paths) {
	t.Helper()
	mkFile(t, p.BinaryPath)
	mkFile(t, p.ConfigFile)
	mkFile(t, filepath.Join(p.DataDir, "model.bin"))
	mkFile(t, filepath.Join(p.CacheDir, "c"))
	mkFile(t, filepath.Join(p.StateDir, "catalog.json"))
}

func mkFile(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestPlanRemovalsConservativeKeepsConfig(t *testing.T) {
	p := tempPaths(t)
	mkFile(t, p.BinaryPath)
	mkFile(t, p.ConfigFile)
	mkFile(t, filepath.Join(p.DataDir, "m"))
	mkFile(t, filepath.Join(p.StateDir, "s"))

	remove, preserved := planRemovals(p, UninstallOptions{})
	if hasCategory(remove, catConfig) {
		t.Error("config must be preserved without --purge")
	}
	if !containsCat(preserved, catConfig) {
		t.Error("config should be listed as preserved")
	}
	if !hasCategory(remove, catBinary) || !hasCategory(remove, catData) {
		t.Error("binary and data should be removed by default")
	}
}

func TestPlanRemovalsKeepDataAndPurge(t *testing.T) {
	p := tempPaths(t)
	mkFile(t, p.ConfigFile)
	mkFile(t, filepath.Join(p.DataDir, "m"))

	remove, preserved := planRemovals(p, UninstallOptions{KeepData: true, Purge: true})
	if hasCategory(remove, catData) {
		t.Error("--keep-data must preserve data")
	}
	if !containsCat(preserved, catData) {
		t.Error("data should be listed as preserved under --keep-data")
	}
	if !hasCategory(remove, catConfig) {
		t.Error("--purge should schedule config removal")
	}
	// The config removal must be marked risky so it demands its own consent.
	for _, r := range remove {
		if r.cat == catConfig && !r.risky {
			t.Error("config removal must be risky")
		}
	}
}

func TestPlanRemovalsOnlyPresentTargets(t *testing.T) {
	p := tempPaths(t) // nothing created
	remove, _ := planRemovals(p, UninstallOptions{Purge: true})
	if len(remove) != 0 {
		t.Errorf("absent targets must not appear in the plan: %+v", remove)
	}
}

func TestUninstallDryRunRemovesNothing(t *testing.T) {
	p := sandboxPaths(t)
	populate(t, p)

	restore := swapStdout(t)
	err := Uninstall(UninstallOptions{DryRun: true})
	restore()
	if err != nil {
		t.Fatalf("Uninstall(--dry-run): %v", err)
	}
	for _, f := range []string{p.BinaryPath, p.ConfigFile} {
		if !pathPresent(f) {
			t.Errorf("--dry-run removed %s", f)
		}
	}
}

func TestUninstallConservativeYesPreservesConfig(t *testing.T) {
	p := sandboxPaths(t)
	populate(t, p)

	// --yes is non-interactive here (test stdin is not a TTY): ordinary removals
	// proceed, but config is never purged without the distinct confirmation.
	restore := swapStdout(t)
	err := Uninstall(UninstallOptions{AssumeYes: true})
	restore()
	if err != nil {
		t.Fatalf("Uninstall(--yes): %v", err)
	}

	if pathPresent(p.BinaryPath) {
		t.Error("binary should have been removed")
	}
	if pathPresent(p.DataDir) {
		t.Error("data should have been removed")
	}
	if !pathPresent(p.ConfigFile) {
		t.Error("config must be preserved — --yes alone never purges")
	}
}

func TestUninstallInterruptedRerunIsIdempotent(t *testing.T) {
	p := sandboxPaths(t)
	populate(t, p)

	for i := 0; i < 2; i++ {
		restore := swapStdout(t)
		err := Uninstall(UninstallOptions{AssumeYes: true})
		restore()
		if err != nil {
			t.Fatalf("uninstall run %d: %v", i, err)
		}
	}
	if pathPresent(p.BinaryPath) {
		t.Error("binary should be gone after idempotent reruns")
	}
}

func containsCat(cats []removalCategory, want removalCategory) bool {
	for _, c := range cats {
		if c == want {
			return true
		}
	}
	return false
}

func TestUninstallPlanMentionsPreserved(t *testing.T) {
	p := tempPaths(t)
	mkFile(t, p.BinaryPath) // a removable artifact, so the plan is non-empty
	mkFile(t, p.ConfigFile) // preserved by default
	var b strings.Builder
	remove, preserved := planRemovals(p, UninstallOptions{})
	printUninstallPlan(&b, remove, preserved)
	if !strings.Contains(b.String(), "Will preserve") || !strings.Contains(b.String(), "Nothing has been removed yet.") {
		t.Errorf("uninstall plan should list preserved items and the no-op notice:\n%s", b.String())
	}
}
