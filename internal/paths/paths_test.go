package paths

import (
	"path/filepath"
	"testing"
)

func envFrom(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestResolveLinuxHonorsXDG(t *testing.T) {
	p := resolve("linux", envFrom(map[string]string{
		"XDG_CONFIG_HOME": "/x/config",
		"XDG_DATA_HOME":   "/x/data",
	}), "/home/u")
	// ToSlash normalizes the separator so the assertion holds on a Windows host
	// too (resolve uses the host's filepath separator; only the structure matters).
	if filepath.ToSlash(p.ConfigDir) != "/x/config/ozy" {
		t.Errorf("ConfigDir = %q, want /x/config/ozy", p.ConfigDir)
	}
	if filepath.ToSlash(p.ConfigFile) != "/x/config/ozy/ozy.jsonc" {
		t.Errorf("ConfigFile = %q", p.ConfigFile)
	}
	if filepath.ToSlash(p.DataDir) != "/x/data/ozy" {
		t.Errorf("DataDir = %q, want /x/data/ozy", p.DataDir)
	}
}

func TestResolveLinuxDefaults(t *testing.T) {
	p := resolve("linux", envFrom(nil), "/home/u")
	want := map[string]string{
		"ConfigDir":  "/home/u/.config/ozy",
		"DataDir":    "/home/u/.local/share/ozy",
		"CacheDir":   "/home/u/.cache/ozy",
		"StateDir":   "/home/u/.local/state/ozy",
		"UserBinDir": "/home/u/.local/bin",
		"VenvDir":    "/home/u/.local/state/ozy/sidecar/venv",
		"BinaryPath": "/home/u/.local/bin/ozy",
	}
	got := map[string]string{
		"ConfigDir": p.ConfigDir, "DataDir": p.DataDir, "CacheDir": p.CacheDir,
		"StateDir": p.StateDir, "UserBinDir": p.UserBinDir, "VenvDir": p.VenvDir,
		"BinaryPath": p.BinaryPath,
	}
	for k, v := range want {
		if filepath.ToSlash(got[k]) != v {
			t.Errorf("%s = %q, want %q", k, got[k], v)
		}
	}
}

func TestResolveWindows(t *testing.T) {
	roaming := `C:\Users\u\AppData\Roaming`
	local := `C:\Users\u\AppData\Local`
	p := resolve("windows", envFrom(map[string]string{
		"APPDATA":      roaming,
		"LOCALAPPDATA": local,
	}), `C:\Users\u`)
	// filepath.Join is used on both sides so the test is host-OS independent.
	if p.ConfigDir != filepath.Join(roaming, "Ozy") {
		t.Errorf("ConfigDir = %q", p.ConfigDir)
	}
	if p.DataDir != filepath.Join(local, "Ozy") {
		t.Errorf("DataDir = %q", p.DataDir)
	}
	if p.CacheDir != filepath.Join(local, "Ozy", "Cache") {
		t.Errorf("CacheDir = %q", p.CacheDir)
	}
	if p.UserBinDir != filepath.Join(local, "Ozy", "bin") {
		t.Errorf("UserBinDir = %q", p.UserBinDir)
	}
	if p.BinaryPath != filepath.Join(p.UserBinDir, "ozy.exe") {
		t.Errorf("BinaryPath = %q", p.BinaryPath)
	}
}

func TestResolveOzyConfigOverrideWins(t *testing.T) {
	p := resolve("linux", envFrom(map[string]string{
		"OZY_CONFIG":      "/custom/place/my.jsonc",
		"XDG_CONFIG_HOME": "/x/config",
	}), "/home/u")
	if filepath.ToSlash(p.ConfigFile) != "/custom/place/my.jsonc" {
		t.Errorf("ConfigFile = %q, want override", p.ConfigFile)
	}
	if filepath.ToSlash(p.ConfigDir) != "/custom/place" {
		t.Errorf("ConfigDir = %q, want /custom/place", p.ConfigDir)
	}
}

func TestOnPath(t *testing.T) {
	cases := []struct {
		name    string
		dir     string
		pathEnv string
		goos    string
		want    bool
	}{
		{"present unix", "/home/u/.local/bin", "/usr/bin:/home/u/.local/bin:/bin", "linux", true},
		{"absent unix", "/home/u/.local/bin", "/usr/bin:/bin", "linux", false},
		{"trailing slash unix", "/home/u/.local/bin", "/home/u/.local/bin/", "linux", true},
		{"present windows case-insensitive", `C:\Users\u\AppData\Local\Ozy\bin`, `C:\Windows;c:\users\u\appdata\local\ozy\bin`, "windows", true},
		{"empty dir", "", "/usr/bin", "linux", false},
		{"empty path", "/home/u/.local/bin", "", "linux", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := OnPath(tc.dir, tc.pathEnv, tc.goos); got != tc.want {
				t.Errorf("OnPath(%q, %q, %q) = %v, want %v", tc.dir, tc.pathEnv, tc.goos, got, tc.want)
			}
		})
	}
}
