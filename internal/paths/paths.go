// Package paths resolves every Ozy install location behind one abstraction so
// the installer and uninstaller never hardcode directories. It follows the
// conventions already used by internal/config (XDG config home) and
// internal/sidecar (state-dir venv): XDG-style locations on Linux and macOS,
// the %APPDATA%/%LOCALAPPDATA% conventions on Windows. Documented overrides
// (OZY_CONFIG and the XDG_* variables) take precedence.
package paths

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// Paths holds every resolved Ozy location. Consumers read these fields instead
// of recomputing or hardcoding paths.
type Paths struct {
	ConfigDir  string // directory holding ozy.jsonc
	ConfigFile string // ozy.jsonc path (honors OZY_CONFIG)
	DataDir    string // models / vector assets live under here
	CacheDir   string // disposable cache
	StateDir   string // durable runtime state (install state, logs, venv)
	LogDir     string // installer/uninstaller logs
	VenvDir    string // managed Python venv (passed to sidecar.Provision)
	AssetDir   string // downloaded embedding/vector assets
	UserBinDir string // where the ozy binary is installed
	BinaryPath string // resolved ozy binary path
}

// Resolve returns the locations for the current OS and environment.
func Resolve() Paths {
	home, _ := os.UserHomeDir()
	return resolve(runtime.GOOS, os.Getenv, home)
}

// resolve is the pure form of Resolve, taking the OS, an environment lookup, and
// the home directory so it can be exercised for any platform in tests.
func resolve(goos string, getenv func(string) string, home string) Paths {
	var p Paths
	if goos == "windows" {
		appData := getenv("APPDATA")
		localAppData := getenv("LOCALAPPDATA")
		p.ConfigDir = filepath.Join(appData, "Ozy")
		p.DataDir = filepath.Join(localAppData, "Ozy")
		p.CacheDir = filepath.Join(localAppData, "Ozy", "Cache")
		p.StateDir = filepath.Join(localAppData, "Ozy", "state")
		p.UserBinDir = filepath.Join(localAppData, "Ozy", "bin")
	} else {
		p.ConfigDir = xdgDir(getenv, "XDG_CONFIG_HOME", home, ".config")
		p.DataDir = xdgDir(getenv, "XDG_DATA_HOME", home, filepath.Join(".local", "share"))
		p.CacheDir = xdgDir(getenv, "XDG_CACHE_HOME", home, ".cache")
		p.StateDir = xdgDir(getenv, "XDG_STATE_HOME", home, filepath.Join(".local", "state"))
		p.UserBinDir = filepath.Join(home, ".local", "bin")
	}
	p.LogDir = filepath.Join(p.StateDir, "logs")
	// Matches internal/sidecar's resolveVenvDir: <state>/ozy/sidecar/venv.
	p.VenvDir = filepath.Join(p.StateDir, "sidecar", "venv")
	p.AssetDir = filepath.Join(p.DataDir, "assets")
	p.BinaryPath = filepath.Join(p.UserBinDir, binaryName(goos))

	// OZY_CONFIG points at an explicit file and wins over the default location,
	// matching config.DefaultPath().
	if c := getenv("OZY_CONFIG"); c != "" {
		p.ConfigFile = c
		p.ConfigDir = filepath.Dir(c)
	} else {
		p.ConfigFile = filepath.Join(p.ConfigDir, "ozy.jsonc")
	}
	return p
}

// xdgDir returns $VAR/ozy when VAR is set, else home/<fallback>/ozy.
func xdgDir(getenv func(string) string, varName, home, fallback string) string {
	if v := getenv(varName); v != "" {
		return filepath.Join(v, "ozy")
	}
	return filepath.Join(home, fallback, "ozy")
}

func binaryName(goos string) string {
	if goos == "windows" {
		return "ozy.exe"
	}
	return "ozy"
}

// Reachable reports whether the user bin directory is on the current PATH.
// It never mutates the environment.
func (p Paths) Reachable() bool {
	return OnPath(p.UserBinDir, os.Getenv("PATH"), runtime.GOOS)
}

// OnPath reports whether dir is one of the entries in a PATH-style list. goos
// selects the list separator and case sensitivity so it is testable for any
// platform from any host.
func OnPath(dir, pathList, goos string) bool {
	if dir == "" || pathList == "" {
		return false
	}
	sep := ":"
	if goos == "windows" {
		sep = ";"
	}
	for _, entry := range strings.Split(pathList, sep) {
		if entry == "" {
			continue
		}
		if pathEqual(entry, dir, goos) {
			return true
		}
	}
	return false
}

func pathEqual(a, b, goos string) bool {
	a = filepath.Clean(a)
	b = filepath.Clean(b)
	if goos == "windows" {
		return strings.EqualFold(a, b)
	}
	return a == b
}
