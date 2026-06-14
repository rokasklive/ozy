package installer

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadMissingIsFresh(t *testing.T) {
	store := NewStateStore(filepath.Join(t.TempDir(), "install-state.json"))
	st, err := store.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if st.SchemaVersion != SchemaVersion {
		t.Errorf("SchemaVersion = %d, want %d", st.SchemaVersion, SchemaVersion)
	}
	if len(st.Steps) != 0 {
		t.Errorf("fresh state has %d steps, want 0", len(st.Steps))
	}
	if st.Done("DetectPlatform") {
		t.Error("fresh state should report no step done")
	}
}

func TestSaveLoadRoundTripResumes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "install-state.json")
	store := NewStateStore(path)

	st, _ := store.Load()
	st.OzyVersion = "0.1.0"
	st.Mark("DetectPlatform", StepDone, "")
	st.Mark("InstallOzyBinary", StepFailed, "build failed")
	if err := store.Save(st); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// A new store simulates a rerun in a fresh process.
	reloaded, err := NewStateStore(path).Load()
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if !reloaded.Done("DetectPlatform") {
		t.Error("DetectPlatform should resume as done")
	}
	if reloaded.Done("InstallOzyBinary") {
		t.Error("failed step must not count as done (so it re-runs)")
	}
	if reloaded.OzyVersion != "0.1.0" {
		t.Errorf("OzyVersion = %q, want 0.1.0", reloaded.OzyVersion)
	}
}

func TestSchemaMismatchStartsFresh(t *testing.T) {
	path := filepath.Join(t.TempDir(), "install-state.json")
	// Write a state file claiming an incompatible schema with a "done" step.
	old := []byte(`{"schemaVersion":999,"steps":{"DetectPlatform":{"status":"done"}}}`)
	if err := os.WriteFile(path, old, 0o600); err != nil {
		t.Fatal(err)
	}
	st, err := NewStateStore(path).Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if st.Done("DetectPlatform") {
		t.Error("steps from a mismatched schema must not be trusted")
	}
	if st.SchemaVersion != SchemaVersion {
		t.Errorf("SchemaVersion = %d, want %d", st.SchemaVersion, SchemaVersion)
	}
}
