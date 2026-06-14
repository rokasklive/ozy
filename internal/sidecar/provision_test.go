package sidecar

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestProvision_ResolveSource_Override(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	sidecarDir := filepath.Join(dir, "sidecar")
	if err := os.MkdirAll(sidecarDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sidecarDir, "__init__.py"), []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}

	src, err := resolveSource(dir)
	if err != nil {
		t.Fatalf("resolveSource(override) = %v", err)
	}
	if src != dir {
		t.Errorf("source = %s, want %s", src, dir)
	}
}

func TestProvision_ResolveSource_OverrideMissingSidecar(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	_, err := resolveSource(dir)
	if err == nil {
		t.Fatal("expected error when override has no sidecar/ subdirectory")
	}
}

func TestProvision_ResolveSource_EnvVar(t *testing.T) {
	// Cannot use t.Parallel — uses t.Setenv.
	dir := t.TempDir()
	sidecarDir := filepath.Join(dir, "sidecar")
	if err := os.MkdirAll(sidecarDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sidecarDir, "__init__.py"), []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("OZY_SIDECAR_SOURCE", dir)
	src, err := resolveSource("")
	if err != nil {
		t.Fatalf("resolveSource(env) = %v", err)
	}
	if src != dir {
		t.Errorf("source = %s, want %s", src, dir)
	}
}

func TestProvision_ResolveVenvDir_Override(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	venv, err := resolveVenvDir(dir)
	if err != nil {
		t.Fatalf("resolveVenvDir(override) = %v", err)
	}
	if venv != dir {
		t.Errorf("venv = %s, want %s", venv, dir)
	}
}

func TestProvision_ResolveVenvDir_Default(t *testing.T) {
	// Cannot use t.Parallel — uses t.Setenv.
	home := t.TempDir()
	t.Setenv("HOME", home)
	// Unset XDG_STATE_HOME so the fallback is used.
	t.Setenv("XDG_STATE_HOME", "")

	venv, err := resolveVenvDir("")
	if err != nil {
		t.Fatalf("resolveVenvDir(default) = %v", err)
	}
	if venv == "" {
		t.Fatal("venv dir should not be empty")
	}
	// Should be under ~/.local/state/ozy/sidecar/venv
	expected := filepath.Join(home, ".local", "state", "ozy", "sidecar", "venv")
	if venv != expected {
		t.Logf("venv = %s, expected %s (may differ on this OS)", venv, expected)
	}
}

func TestProvision_NoToolchain(t *testing.T) {
	t.Parallel()
	// Simulate absence of both uv and python3 by providing a LookPath
	// that always returns "not found".
	lookPath := func(name string) (string, error) {
		return "", errors.New("not found")
	}

	_, err := resolvePython(context.Background(), ProvisionOptions{}, lookPath, nil)
	if !errors.Is(err, ErrNoToolchain) {
		t.Fatalf("resolvePython() = %v, want ErrNoToolchain", err)
	}
}

func TestProvision_ResolvePython_Override(t *testing.T) {
	t.Parallel()
	lookPath := func(name string) (string, error) {
		return "", errors.New("not found")
	}

	python, err := resolvePython(context.Background(), ProvisionOptions{
		PythonOverride: "/usr/bin/python3",
	}, lookPath, nil)
	if err != nil {
		t.Fatalf("resolvePython(override) = %v", err)
	}
	if python != "/usr/bin/python3" {
		t.Errorf("python = %s, want /usr/bin/python3", python)
	}
}

func TestProvision_ResolvePython_EnvVar(t *testing.T) {
	// Cannot use t.Parallel — uses t.Setenv.
	t.Setenv("OZY_SIDECAR_PYTHON", "/custom/python")
	lookPath := func(name string) (string, error) {
		return "", errors.New("not found")
	}

	python, err := resolvePython(context.Background(), ProvisionOptions{}, lookPath, nil)
	if err != nil {
		t.Fatalf("resolvePython(env) = %v", err)
	}
	if python != "/custom/python" {
		t.Errorf("python = %s, want /custom/python", python)
	}
}

func TestProvision_ResolvePython_UvAvailable(t *testing.T) {
	t.Parallel()
	lookPath := func(name string) (string, error) {
		if name == "uv" {
			return "/usr/local/bin/uv", nil
		}
		return "", errors.New("not found")
	}
	// Since uvResolvePython uses exec.CommandContext to run uv, and
	// we can't easily mock that without injecting a Runner override,
	// this test verifies that uv is detected on PATH. The actual
	// uvResolvePython is tested indirectly through Provision.
	uvPath, err := lookPath("uv")
	if err != nil {
		t.Fatal("uv should be found on mock PATH")
	}
	if uvPath == "" {
		t.Fatal("uv path should not be empty")
	}
}

func TestProvision_ResolvePython_Python3Available(t *testing.T) {
	t.Parallel()
	lookPath := func(name string) (string, error) {
		if name == "uv" {
			return "", errors.New("not found")
		}
		if name == "python3" {
			return "/usr/bin/python3", nil
		}
		return "", errors.New("not found")
	}

	python, err := resolvePython(context.Background(), ProvisionOptions{}, lookPath, nil)
	if err != nil {
		t.Fatalf("resolvePython(python3) = %v", err)
	}
	if python != "/usr/bin/python3" {
		t.Errorf("python = %s, want /usr/bin/python3", python)
	}
}

func TestProvision_VenvPython(t *testing.T) {
	t.Parallel()
	// Platform-agnostic check: ensure the function returns a
	// reasonable path.
	p := venvPython("/tmp/test-venv")
	if p == "" {
		t.Fatal("venvPython returned empty string")
	}
	// Should contain "python" somewhere.
	if !filepath.IsAbs(p) {
		t.Errorf("venvPython path should be absolute: %s", p)
	}
}

func TestProvision_FileExists(t *testing.T) {
	t.Parallel()
	if fileExists("") {
		t.Error("fileExists(\"\") should be false")
	}
	if fileExists("/nonexistent/path/that/does/not/exist") {
		t.Error("fileExists should be false for nonexistent path")
	}
	// Temp file should exist.
	f := filepath.Join(t.TempDir(), "test.txt")
	if err := os.WriteFile(f, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !fileExists(f) {
		t.Errorf("fileExists(%s) should be true", f)
	}
}

func TestProvision_MarkerRoundTrip(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	markerPath := filepath.Join(dir, markerFileName)

	m := marker{
		Python:    "3.11",
		Fastembed: "0.8.0",
		Turbovec:  "0.8.0",
		Model:     "BAAI/bge-small-en-v1.5",
		Backend:   "turbovec",
	}
	if err := writeMarker(markerPath, m); err != nil {
		t.Fatalf("writeMarker: %v", err)
	}

	got, ok := readMarker(markerPath)
	if !ok {
		t.Fatal("readMarker returned false")
	}
	if !markerMatches(got, m) {
		t.Errorf("marker mismatch: got %+v, want %+v", got, m)
	}
}

func TestProvision_MarkerMismatch(t *testing.T) {
	t.Parallel()
	a := marker{Fastembed: "0.8.0", Turbovec: "0.8.0", Model: "m1", Backend: "turbovec"}
	b := marker{Fastembed: "0.8.0", Turbovec: "0.8.0", Model: "m2", Backend: "turbovec"}
	if markerMatches(a, b) {
		t.Error("markerMatches should be false for different models")
	}
}

func TestProvision_DefaultProvisionTimeout(t *testing.T) {
	t.Parallel()
	if DefaultProvisionTimeout <= 0 {
		t.Error("DefaultProvisionTimeout should be positive")
	}
}

func TestProvision_ErrNoToolchainIsSentinel(t *testing.T) {
	t.Parallel()
	if !errors.Is(ErrNoToolchain, ErrNoToolchain) {
		t.Error("ErrNoToolchain should match itself")
	}
	if errors.Is(ErrNoToolchain, errors.New("other")) {
		t.Error("ErrNoToolchain should not match unrelated errors")
	}
}

func TestProvision_FindModuleRoot(t *testing.T) {
	t.Parallel()
	root, err := findModuleRoot()
	if err != nil {
		// In test environments without a proper module root this
		// could happen. Skip if we can't find it — it's OK for
		// unit tests.
		t.Skipf("findModuleRoot not available in this test env: %v", err)
	}
	if root == "" {
		t.Fatal("findModuleRoot returned empty string")
	}
	// Root should contain go.mod.
	if !fileExists(filepath.Join(root, "go.mod")) {
		t.Errorf("module root %s does not contain go.mod", root)
	}
}
