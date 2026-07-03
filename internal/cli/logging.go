package cli

import (
	"log/slog"
	"os"
	"path/filepath"
)

// logsDir resolves the operational log directory: a `logs/` directory beside the
// loaded configuration file (runtime-logging capability). When the config is
// loaded from a custom --config path, logs live next to that file.
func logsDir(configPath string) string {
	return filepath.Join(filepath.Dir(configPath), "logs")
}

// newLogger returns a structured JSON logger writing to <config-dir>/logs/ozy.log.
// It falls back to a stderr logger when the directory or file cannot be opened,
// so logging never blocks startup. Records carry an `action` field on failures
// so a reader sees the next step, not just the error.
func newLogger(configPath string) *slog.Logger {
	dir := logsDir(configPath)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return stderrLogger()
	}
	// #nosec G304 -- log path is the user's own config directory with a constant filename.
	f, err := os.OpenFile(filepath.Join(dir, "ozy.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return stderrLogger()
	}
	// ponytail: process-lifetime log file, never explicitly closed — the OS
	// closes it on exit; add rotation if size ever bites.
	return slog.New(slog.NewJSONHandler(f, &slog.HandlerOptions{Level: slog.LevelInfo}))
}

func stderrLogger() *slog.Logger {
	return slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
}
