package installer

import (
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

// DepStatus is the detection outcome for one dependency.
type DepStatus string

// Dependency detection statuses.
const (
	DepOK        DepStatus = "ok"       // present and new enough
	DepMissing   DepStatus = "missing"  // not found
	DepOutOfDate DepStatus = "outdated" // present but below the required version
)

// Dependency is one row of the dependency report: everything the user needs to
// understand what Ozy found and what it will do about it.
type Dependency struct {
	Name      string    // human label, e.g. "Python"
	Required  bool      // required vs optional
	Detected  string    // detected version, "" when missing
	Wanted    string    // required version expression, "" when any version works
	Status    DepStatus // ok / missing / outdated
	Why       string    // why Ozy needs it
	CanManage bool      // whether Ozy can provision/manage it
	Fallback  string    // what happens if it stays missing
}

// runner runs name with args and returns combined output. It is the single
// command-execution seam in the installer so tests can inject fakes.
type runner func(name string, args ...string) (string, error)

// lookPath reports the resolved path of an executable. The seam mirrors runner.
type lookPath func(name string) (string, error)

func execRunner(name string, args ...string) (string, error) {
	out, err := exec.Command(name, args...).CombinedOutput() //nolint:gosec // fixed tool names
	return string(out), err
}

// DepChecker detects Ozy's build- and runtime dependencies. It never mutates
// anything; it only reports what it finds.
type DepChecker struct {
	look lookPath
	run  runner
}

// NewDepChecker returns a checker backed by the real exec.LookPath / exec.Command.
func NewDepChecker() DepChecker {
	return DepChecker{look: exec.LookPath, run: execRunner}
}

// Check runs every detection and returns the dependency report in display order.
func (d DepChecker) Check() []Dependency {
	python := d.checkPython()
	return []Dependency{
		d.checkGo(),
		d.checkGit(),
		python,
		d.checkSQLite(),
		semanticFrom(python),
	}
}

func (d DepChecker) checkGo() Dependency {
	dep := Dependency{
		Name:     "Go toolchain",
		Required: true,
		Wanted:   "1.26+",
		Why:      "builds the ozy binary from source",
		Fallback: "cannot install ozy; install Go from https://go.dev/dl",
	}
	out, err := d.run("go", "version")
	if err != nil {
		dep.Status = DepMissing
		return dep
	}
	dep.Detected = firstVersion(out)
	dep.Status = statusFor(dep.Detected, 1, 26)
	return dep
}

func (d DepChecker) checkGit() Dependency {
	dep := Dependency{
		Name:     "Git",
		Required: true,
		Why:      "fetches the ozy module sources for go run/go install",
		Fallback: "go run @version cannot resolve the module; install Git",
	}
	if _, err := d.look("git"); err != nil {
		dep.Status = DepMissing
		return dep
	}
	dep.Detected = firstVersion(mustOut(d.run("git", "--version")))
	dep.Status = DepOK
	return dep
}

func (d DepChecker) checkPython() Dependency {
	dep := Dependency{
		Name:      "Python",
		Required:  false,
		Wanted:    "3.11+",
		Why:       "runs the embedding sidecar for semantic search",
		CanManage: true,
		Fallback:  "semantic search is unavailable; Ozy runs lexical-only",
	}
	for _, name := range []string{"uv", "python3", "python"} {
		if _, err := d.look(name); err != nil {
			continue
		}
		if name == "uv" {
			// uv can provision a managed 3.11+ interpreter even if none is installed.
			dep.Detected = "via uv"
			dep.Status = DepOK
			return dep
		}
		out, err := d.run(name, "--version")
		if err != nil {
			continue
		}
		dep.Detected = firstVersion(out)
		dep.Status = statusFor(dep.Detected, 3, 11)
		return dep
	}
	dep.Status = DepMissing
	return dep
}

func (d DepChecker) checkSQLite() Dependency {
	dep := Dependency{
		Name:     "SQLite",
		Required: false,
		Why:      "the tool index uses SQLite; the driver is bundled in the binary",
		Fallback: "none needed — the sqlite3 CLI is optional, used only for inspection",
	}
	if _, err := d.look("sqlite3"); err != nil {
		dep.Status = DepMissing
		dep.Detected = ""
		return dep
	}
	dep.Detected = firstVersion(mustOut(d.run("sqlite3", "--version")))
	dep.Status = DepOK
	return dep
}

// semanticFrom derives the semantic-backend availability from the Python row,
// since the installer provisions the venv from whatever Python it finds.
func semanticFrom(python Dependency) Dependency {
	dep := Dependency{
		Name:      "Semantic backend",
		Required:  false,
		Why:       "FastEmbed + vector store for semantic/hybrid search",
		CanManage: true,
		Fallback:  "Ozy runs lexical-only until a Python toolchain is available",
	}
	if python.Status == DepMissing {
		dep.Status = DepMissing
		return dep
	}
	// Provisioned on demand; mark available since a usable interpreter exists.
	dep.Status = DepOK
	dep.Detected = "provisionable"
	return dep
}

var versionRe = regexp.MustCompile(`\d+\.\d+(\.\d+)?`)

// firstVersion extracts the first dotted version number from tool output.
func firstVersion(s string) string {
	return versionRe.FindString(strings.TrimSpace(s))
}

func mustOut(out string, _ error) string { return out }

// statusFor compares a detected version against a minimum major.minor.
func statusFor(detected string, minMajor, minMinor int) DepStatus {
	major, minor, ok := majorMinor(detected)
	if !ok {
		return DepMissing
	}
	if major > minMajor || (major == minMajor && minor >= minMinor) {
		return DepOK
	}
	return DepOutOfDate
}

func majorMinor(v string) (int, int, bool) {
	m := versionRe.FindString(v)
	if m == "" {
		return 0, 0, false
	}
	parts := strings.SplitN(m, ".", 3)
	if len(parts) < 2 {
		return 0, 0, false
	}
	major, err1 := strconv.Atoi(parts[0])
	minor, err2 := strconv.Atoi(parts[1])
	if err1 != nil || err2 != nil {
		return 0, 0, false
	}
	return major, minor, true
}
