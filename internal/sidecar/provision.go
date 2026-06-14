package sidecar

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// Pinned dependency versions installed into the venv. Kept as constants
// so the sidecar package and the provisioner agree on what is installed.
const (
	fastembedVersion  = "0.8.0"
	turbovecVersion   = "0.8.0"
	faissCPUVersion   = "1.14.3"
	defaultPythonVers = "3.11"
	markerFileName    = ".ozy-provisioned"
)

// ErrNoToolchain is returned by Provision when neither uv nor a system
// python3 / py interpreter is available, or when OZY_SIDECAR_PYTHON is
// set but does not exist. Callers treat it as a soft "unavailable"
// signal — never a fatal configuration error.
var ErrNoToolchain = errors.New("sidecar: no usable Python toolchain found")

// ProvisionOptions controls the venv resolver. Tests inject LookPath
// and Runner to simulate absent tools; production callers leave them
// nil to get the exec.LookPath and os/exec defaults.
type ProvisionOptions struct {
	// VenvDir overrides the default XDG-state-based venv location.
	// Empty means "compute the default".
	VenvDir string

	// SourceDir overrides the discovered sidecar source location.
	// Empty means "discover from the running binary".
	SourceDir string

	// PythonOverride forces a specific interpreter; when non-empty it
	// short-circuits the resolution order. Honors OZY_SIDECAR_PYTHON
	// automatically.
	PythonOverride string

	// Backend is the vector backend name. Drives the dependency set
	// (faiss-cpu is installed only for the faiss backend).
	Backend string

	// Model is the FastEmbed model id. Recorded in the marker file.
	Model string

	// Logger receives progress messages. nil drops the messages.
	Logger Logger

	// LookPath defaults to exec.LookPath when nil.
	LookPath func(name string) (string, error)

	// Runner runs a subprocess and returns its exit error. Defaults
	// to a function that invokes exec.CommandContext(ctx, name, args...).
	Runner func(ctx context.Context, name string, args ...string) error
}

// Resolved is the outcome of provisioning: where the venv lives, the
// interpreter to invoke, and where the sidecar source tree is.
type Resolved struct {
	PythonPath string
	SourceDir  string
	VenvDir    string
	MarkerPath string
}

// marker is the on-disk record of what was installed. The provisioner
// reads it on subsequent runs to short-circuit reinstall when the
// pinned versions and the model/backend still match.
type marker struct {
	Python    string `json:"python"`
	Fastembed string `json:"fastembed"`
	Turbovec  string `json:"turbovec"`
	FaissCPU  string `json:"faissCpu,omitempty"`
	Model     string `json:"model"`
	Backend   string `json:"backend"`
}

// Provision resolves a Python interpreter, ensures a venv is present
// at the default or supplied location, and returns the paths the
// sidecar process needs. It is safe to call repeatedly: the marker file
// makes a no-op when the venv is up to date.
//
// ErrNoToolchain is returned when no usable toolchain can be found.
// Any other error indicates a configuration or filesystem problem the
// caller should surface to the user.
func Provision(ctx context.Context, opts ProvisionOptions) (*Resolved, error) {
	lookPath := opts.LookPath
	if lookPath == nil {
		lookPath = exec.LookPath
	}
	runner := opts.Runner
	if runner == nil {
		runner = defaultRunner
	}
	logger := opts.Logger

	python, err := resolvePython(ctx, opts, lookPath, runner, logger)
	if err != nil {
		return nil, err
	}
	source, err := resolveSource(opts.SourceDir)
	if err != nil {
		return nil, err
	}
	venv, err := resolveVenvDir(opts.VenvDir)
	if err != nil {
		return nil, err
	}
	markerPath := filepath.Join(venv, markerFileName)

	want := marker{
		Python:    fastembedVersion,
		Fastembed: fastembedVersion,
		Turbovec:  turbovecVersion,
		FaissCPU:  faissCPUVersion,
		Model:     opts.Model,
		Backend:   opts.Backend,
	}

	if existing, ok := readMarker(markerPath); ok && markerMatches(existing, want) && venvHasPython(venv) {
		logf(logger, "sidecar: reusing existing venv at %s", venv)
		return &Resolved{PythonPath: venvPython(venv), SourceDir: source, VenvDir: venv, MarkerPath: markerPath}, nil
	}

	if err := ensureVenv(ctx, python, venv, lookPath, runner, logger); err != nil {
		return nil, err
	}
	if err := installDeps(ctx, venv, opts.Backend, lookPath, runner, logger); err != nil {
		return nil, err
	}
	if err := writeMarker(markerPath, want); err != nil {
		return nil, fmt.Errorf("sidecar: write marker: %w", err)
	}
	logf(logger, "sidecar: provisioned venv at %s", venv)
	return &Resolved{PythonPath: venvPython(venv), SourceDir: source, VenvDir: venv, MarkerPath: markerPath}, nil
}

// resolvePython walks the documented resolution order. When
// PythonOverride (or OZY_SIDECAR_PYTHON) is set it is returned without
// any PATH check. Otherwise the order is: cached venv interpreter,
// `uv run --python <ver>` if uv is on PATH, then `python3`, then
// `py -3` on Windows.
func resolvePython(ctx context.Context, opts ProvisionOptions, lookPath func(string) (string, error), runner func(context.Context, string, ...string) error, logger Logger) (string, error) {
	if override := opts.PythonOverride; override != "" {
		return override, nil
	}
	if override := os.Getenv("OZY_SIDECAR_PYTHON"); override != "" {
		return override, nil
	}

	if opts.VenvDir != "" {
		if p := venvPython(opts.VenvDir); fileExists(p) {
			return p, nil
		}
	}

	if uvPath, err := lookPath("uv"); err == nil && uvPath != "" {
		logf(logger, "sidecar: resolving Python via uv at %s", uvPath)
		if out, err := uvResolvePython(ctx, uvPath, defaultPythonVers); err == nil && out != "" {
			return out, nil
		}
	}

	for _, name := range basePythonNames() {
		if p, err := lookPath(name); err == nil && p != "" {
			return p, nil
		}
	}

	return "", ErrNoToolchain
}

func basePythonNames() []string {
	if runtime.GOOS == "windows" {
		return []string{"py", "python3", "python"}
	}
	return []string{"python3", "python"}
}

// uvResolvePython invokes `uv run --python <ver> python -c 'import sys;
// print(sys.executable)'` and returns the printed path.
func uvResolvePython(ctx context.Context, uvPath, pyVersion string) (string, error) {
	cmd := exec.CommandContext(ctx, uvPath, "run", "--python", pyVersion, "python", "-c", "import sys; print(sys.executable)") //nolint:gosec
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// resolveSource finds the directory the `sidecar` Python package
// lives in. The default walks up from the running binary until it
// finds a go.mod; the package is the sibling `sidecar/` directory.
// OZY_SIDECAR_SOURCE overrides the default.
func resolveSource(override string) (string, error) {
	if override != "" {
		if !fileExists(filepath.Join(override, "sidecar")) {
			return "", fmt.Errorf("sidecar: OZY_SIDECAR_SOURCE=%s has no sidecar/ subdirectory", override)
		}
		return override, nil
	}
	if env := os.Getenv("OZY_SIDECAR_SOURCE"); env != "" {
		if !fileExists(filepath.Join(env, "sidecar")) {
			return "", fmt.Errorf("sidecar: OZY_SIDECAR_SOURCE=%s has no sidecar/ subdirectory", env)
		}
		return env, nil
	}
	root, err := findModuleRoot()
	if err != nil {
		return "", err
	}
	return root, nil
}

// findModuleRoot locates the repo root by walking up from this
// source file until it finds a go.mod. Returns the directory
// containing go.mod, which is the parent of the internal/ tree.
func findModuleRoot() (string, error) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return "", errors.New("sidecar: runtime.Caller unavailable")
	}
	dir := filepath.Dir(file)
	for i := 0; i < 16; i++ {
		if fileExists(filepath.Join(dir, "go.mod")) {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", errors.New("sidecar: could not locate repo root (no go.mod found)")
}

// resolveVenvDir computes the venv path. Honors VenvDir when set;
// otherwise uses $XDG_STATE_HOME/ozy/sidecar/venv or
// ~/.local/state/ozy/sidecar/venv, with a Windows equivalent.
func resolveVenvDir(override string) (string, error) {
	if override != "" {
		return override, nil
	}
	state := os.Getenv("XDG_STATE_HOME")
	if state == "" && runtime.GOOS != "windows" {
		if home, err := os.UserHomeDir(); err == nil && home != "" {
			state = filepath.Join(home, ".local", "state")
		}
	}
	if state == "" {
		base, err := os.UserConfigDir()
		if err != nil || base == "" {
			return "", errors.New("sidecar: cannot determine venv location")
		}
		state = filepath.Join(base, "ozy")
	}
	return filepath.Join(state, "ozy", "sidecar", "venv"), nil
}

// ensureVenv creates the venv directory if it does not already exist.
// uv is preferred; if uv is not on PATH it falls back to
// `python -m venv`.
func ensureVenv(ctx context.Context, python, venv string, lookPath func(string) (string, error), runner func(context.Context, string, ...string) error, logger Logger) error {
	if venvHasPython(venv) {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(venv), 0o700); err != nil {
		return fmt.Errorf("sidecar: mkdir venv parent: %w", err)
	}
	if uvPath, err := lookPath("uv"); err == nil && uvPath != "" {
		logf(logger, "sidecar: creating venv with uv at %s", venv)
		return runner(ctx, uvPath, "venv", venv, "--python", defaultPythonVers)
	}
	logf(logger, "sidecar: creating venv with %s -m venv", python)
	return runner(ctx, python, "-m", "venv", venv)
}

// installDeps installs the pinned packages into the venv. Uses uv
// when available (much faster); otherwise falls back to pip.
func installDeps(ctx context.Context, venv, backend string, lookPath func(string) (string, error), runner func(context.Context, string, ...string) error, logger Logger) error {
	packages := []string{
		"fastembed==" + fastembedVersion,
		"turbovec==" + turbovecVersion,
	}
	if backend == "faiss" {
		packages = append(packages, "faiss-cpu=="+faissCPUVersion)
	}
	venvPy := venvPython(venv)

	if uvPath, err := lookPath("uv"); err == nil && uvPath != "" {
		logf(logger, "sidecar: installing deps with uv pip")
		args := []string{"pip", "install", "--python", venvPy}
		args = append(args, packages...)
		return runner(ctx, uvPath, args...)
	}
	logf(logger, "sidecar: installing deps with pip")
	args := []string{"-m", "pip", "install"}
	args = append(args, packages...)
	return runner(ctx, venvPy, args...)
}

func readMarker(path string) (marker, bool) {
	data, err := os.ReadFile(path) //nolint:gosec
	if err != nil {
		return marker{}, false
	}
	var m marker
	if err := json.Unmarshal(data, &m); err != nil {
		return marker{}, false
	}
	return m, true
}

func markerMatches(have, want marker) bool {
	return have == want
}

func writeMarker(path string, m marker) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

func venvHasPython(venv string) bool {
	return fileExists(venvPython(venv))
}

// venvPython returns the conventional interpreter path inside a venv.
func venvPython(venv string) string {
	if runtime.GOOS == "windows" {
		return filepath.Join(venv, "Scripts", "python.exe")
	}
	return filepath.Join(venv, "bin", "python")
}

func fileExists(path string) bool {
	if path == "" {
		return false
	}
	_, err := os.Stat(path) //nolint:gosec
	return err == nil
}

func defaultRunner(ctx context.Context, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...) //nolint:gosec
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func logf(logger Logger, format string, args ...any) {
	if logger == nil {
		return
	}
	logger.Log(fmt.Sprintf(format, args...))
}

// DefaultProvisionTimeout caps a single provisioning attempt.
// First-run installs can be slow (model downloads), so the default is
// generous; callers that need to bound startup tighter should pass
// their own context.
const DefaultProvisionTimeout = 5 * time.Minute
