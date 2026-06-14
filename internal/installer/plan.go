package installer

import (
	"fmt"
	"io"
	"os"
	"runtime/debug"
	"strings"
	"text/tabwriter"

	"github.com/rokasklive/ozy/internal/paths"
)

// Plan is the full, transparent description of what an install run will do. It
// is built from detection only — constructing it never mutates the system.
type Plan struct {
	Platform       Platform
	Paths          paths.Paths
	OzyVersion     string
	IsUpdate       bool   // an existing binary or config was found
	ExistingBinary string // path of an existing ozy binary, "" if none
	ExistingConfig string // path of an existing config, "" if none
	Deps           []Dependency
	Steps          []string // ordered planned step names
	Downloads      []string // planned network fetches, human-readable
	EstDiskBytes   int64    // rough total, labelled an estimate when shown
	PathChange     string   // planned PATH change, "" when already reachable
	Warnings       []string
}

// BuildPlan inspects the host and produces the install plan. It performs only
// read-only detection (DetectPlatform, ResolveInstallDirs, CheckExistingInstall,
// dependency checks) — no step in here writes anything.
func BuildPlan(plat Platform, p paths.Paths, deps DepChecker) Plan {
	plan := Plan{
		Platform:   plat,
		Paths:      p,
		OzyVersion: ozyVersion(),
		Deps:       deps.Check(),
		Steps:      stepNames(),
	}

	// CheckExistingInstall: an existing binary or config makes this an update.
	if fileExists(p.BinaryPath) {
		plan.ExistingBinary = p.BinaryPath
		plan.IsUpdate = true
	}
	if fileExists(p.ConfigFile) {
		plan.ExistingConfig = p.ConfigFile
		plan.IsUpdate = true
	}

	// PATH change is only planned when the bin dir is not already reachable.
	if !p.Reachable() {
		plan.PathChange = fmt.Sprintf("add %s to PATH", p.UserBinDir)
	}

	// Network + disk are dominated by the semantic backend. Only plan them when
	// a usable Python toolchain exists; otherwise the install is lexical-only.
	if semanticAvailable(plan.Deps) {
		plan.Downloads = []string{
			"Python packages: FastEmbed + vector store (pinned)",
			"embedding model assets",
		}
		// ponytail: fixed estimate, not a real size probe. Labelled "estimate"
		// in the plan; replace with a manifest sum if precision ever matters.
		plan.EstDiskBytes = 450 << 20
	} else {
		plan.Warnings = append(plan.Warnings,
			"No usable Python toolchain found — installing in lexical-only mode.")
	}

	return plan
}

// SemanticPlanned reports whether the plan provisions the semantic backend (a
// usable Python toolchain was detected). When false, the install is lexical-only.
func (p Plan) SemanticPlanned() bool { return semanticAvailable(p.Deps) }

func semanticAvailable(deps []Dependency) bool {
	for _, d := range deps {
		if d.Name == "Semantic backend" {
			return d.Status == DepOK
		}
	}
	return false
}

// ozyVersion reports the module version the bootstrap was built from. Under
// `go run module@v1.2.3` this is the tag; under a local `go run ./...` it is
// "(devel)", which we surface as "dev".
func ozyVersion() string {
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	return "dev"
}

// releaseVersion returns the clean released tag this bootstrap was built from
// and true, or false for dev/dirty local builds that cannot be fetched via
// `@version` (so callers fall back to @latest or the local source path).
func releaseVersion() (string, bool) {
	v := ozyVersion()
	if v == "dev" || strings.Contains(v, "+dirty") {
		return "", false
	}
	return v, true
}

func fileExists(path string) bool {
	if path == "" {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// RenderPlan writes the human-readable plan and dependency report to w. The same
// renderer serves TTY and non-TTY output (plain text, colour is layered
// elsewhere). It always ends with "Nothing has changed yet."
func RenderPlan(w io.Writer, plan Plan) {
	fmt.Fprintf(w, "Ozy install plan (version %s)\n\n", plan.OzyVersion)
	fmt.Fprintf(w, "Platform:     %s/%s\n", plan.Platform.OS, plan.Platform.Arch)
	if plan.IsUpdate {
		fmt.Fprintln(w, "Mode:         update (existing Ozy install detected)")
	} else {
		fmt.Fprintln(w, "Mode:         fresh install")
	}
	fmt.Fprintln(w)

	fmt.Fprintln(w, "Locations:")
	fmt.Fprintf(w, "  binary      %s\n", plan.Paths.BinaryPath)
	fmt.Fprintf(w, "  config      %s\n", plan.Paths.ConfigFile)
	fmt.Fprintf(w, "  data        %s\n", plan.Paths.DataDir)
	fmt.Fprintf(w, "  cache       %s\n", plan.Paths.CacheDir)
	fmt.Fprintf(w, "  venv        %s\n", plan.Paths.VenvDir)
	fmt.Fprintf(w, "  logs        %s\n", plan.Paths.LogDir)
	fmt.Fprintln(w)

	renderDeps(w, plan.Deps)
	fmt.Fprintln(w)

	fmt.Fprintln(w, "Planned actions:")
	for _, s := range plan.Steps {
		fmt.Fprintf(w, "  - %s\n", s)
	}
	if plan.PathChange != "" {
		fmt.Fprintf(w, "  - PATH: %s\n", plan.PathChange)
	}
	fmt.Fprintln(w)

	if len(plan.Downloads) > 0 {
		fmt.Fprintln(w, "Network downloads:")
		for _, dl := range plan.Downloads {
			fmt.Fprintf(w, "  - %s\n", dl)
		}
		if plan.EstDiskBytes > 0 {
			fmt.Fprintf(w, "  Estimated disk usage: ~%d MB (estimate)\n", plan.EstDiskBytes>>20)
		}
		fmt.Fprintln(w)
	}

	if plan.ExistingConfig != "" {
		fmt.Fprintf(w, "Existing config:  %s (will be preserved; backed up before any edit)\n\n", plan.ExistingConfig)
	}

	for _, warn := range plan.Warnings {
		fmt.Fprintf(w, "Warning: %s\n", warn)
	}
	if len(plan.Warnings) > 0 {
		fmt.Fprintln(w)
	}

	fmt.Fprintln(w, "Nothing has changed yet.")
}

func renderDeps(w io.Writer, deps []Dependency) {
	fmt.Fprintln(w, "Dependencies:")
	tw := tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
	fmt.Fprintln(tw, "  NAME\tNEED\tDETECTED\tREQUIRED\tSTATUS\tFALLBACK")
	for _, d := range deps {
		need := "optional"
		if d.Required {
			need = "required"
		}
		detected := d.Detected
		if detected == "" {
			detected = "-"
		}
		wanted := d.Wanted
		if wanted == "" {
			wanted = "any"
		}
		fmt.Fprintf(tw, "  %s\t%s\t%s\t%s\t%s\t%s\n",
			d.Name, need, detected, wanted, d.Status, d.Fallback)
	}
	_ = tw.Flush()
}
