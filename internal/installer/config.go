package installer

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/rokasklive/ozy/internal/config"
)

// ConfigManager creates or safely preserves the user's ozy.jsonc. It never
// clobbers an existing config: a fresh file is written only when none exists,
// and any future edit goes through a timestamped backup first.
type ConfigManager struct {
	path string
}

// NewConfigManager manages the config at path (paths.ConfigFile).
func NewConfigManager(path string) ConfigManager { return ConfigManager{path: path} }

// Ensure creates a default config when none exists, or validates and reports an
// existing one without overwriting it. It returns the (unchanged) config path.
func (m ConfigManager) Ensure(log *Logger) error {
	if fileExists(m.path) {
		if _, cerr := config.Load(m.path); cerr != nil {
			// Never silently rewrite — report and leave the user's file intact.
			log.Sayf("Existing config at %s has an issue: %s (left unchanged)", m.path, cerr.Message)
			return nil
		}
		log.Sayf("Existing config is valid: %s (preserved)", m.path)
		return nil
	}
	if err := config.WriteStarter(m.path); err != nil {
		return fmt.Errorf("write default config: %w", err)
	}
	log.Sayf("Wrote default config: %s", m.path)
	return nil
}

// BackupConfig copies the config to a timestamped sibling and returns the backup
// path. It is the required first move before any in-place edit of an existing
// config. Callers invoke it only when a change is actually needed.
func BackupConfig(path string) (string, error) {
	data, err := os.ReadFile(path) //nolint:gosec // user-owned config path
	if err != nil {
		return "", err
	}
	backup := fmt.Sprintf("%s.bak-%s", path, time.Now().UTC().Format("20060102-150405"))
	//nolint:gosec // backup path derives from the user's own config path we just read
	if err := os.WriteFile(backup, data, 0o600); err != nil {
		return "", err
	}
	_ = filepath.Dir(backup)
	return backup, nil
}
