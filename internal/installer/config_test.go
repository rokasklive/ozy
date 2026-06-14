package installer

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rokasklive/ozy/internal/config"
)

func testLogger(t *testing.T) *Logger {
	t.Helper()
	log, err := NewLogger(t.TempDir(), "test", io.Discard)
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}
	t.Cleanup(func() { log.Close() })
	return log
}

// mustMkConfig writes a valid starter config at path (also used by plan_test).
func mustMkConfig(t *testing.T, path string) {
	t.Helper()
	if err := config.WriteStarter(path); err != nil {
		t.Fatalf("WriteStarter: %v", err)
	}
}

func TestEnsureWritesFreshConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ozy.jsonc")
	if err := NewConfigManager(path).Ensure(testLogger(t)); err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	if !fileExists(path) {
		t.Fatal("config not written")
	}
	if _, cerr := config.Load(path); cerr != nil {
		t.Fatalf("written config is invalid: %v", cerr)
	}
}

func TestEnsurePreservesExistingValid(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ozy.jsonc")
	mustMkConfig(t, path)
	before, _ := os.ReadFile(path)

	if err := NewConfigManager(path).Ensure(testLogger(t)); err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	after, _ := os.ReadFile(path)
	if !bytes.Equal(before, after) {
		t.Error("an existing valid config must not be modified")
	}
}

func TestEnsureDoesNotClobberInvalid(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ozy.jsonc")
	if err := os.WriteFile(path, []byte("{ this is not valid"), 0o600); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(path)

	// An invalid existing config is reported, not overwritten — and not an error.
	if err := NewConfigManager(path).Ensure(testLogger(t)); err != nil {
		t.Fatalf("Ensure should not fail on an invalid existing config: %v", err)
	}
	after, _ := os.ReadFile(path)
	if !bytes.Equal(before, after) {
		t.Error("an invalid existing config must not be clobbered")
	}
}

func TestBackupConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ozy.jsonc")
	mustMkConfig(t, path)

	backup, err := BackupConfig(path)
	if err != nil {
		t.Fatalf("BackupConfig: %v", err)
	}
	if !strings.Contains(backup, ".bak-") {
		t.Errorf("backup name should carry a timestamp marker: %s", backup)
	}
	orig, _ := os.ReadFile(path)
	bak, _ := os.ReadFile(backup)
	if !bytes.Equal(orig, bak) {
		t.Error("backup content must match the original")
	}
}
