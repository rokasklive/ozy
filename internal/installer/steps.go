package installer

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rokasklive/ozy/internal/catalog"
	"github.com/rokasklive/ozy/internal/config"
	"github.com/rokasklive/ozy/internal/paths"
	"github.com/rokasklive/ozy/internal/sidecar"
)

// step is one unit of installation work. Its metadata drives both the plan
// listing and --manual mode; run performs the work; valid reports whether the
// step's output already exists and is current (used to skip or revalidate on a
// rerun). A nil valid means a pure step that always re-runs cheaply.
type step struct {
	name   string
	title  string
	what   string
	why    string
	verify string
	next   string
	risk   Risk
	run    func(*execContext) error
	valid  func(*execContext) bool
}

// execContext is the shared state every step reads and writes. It is built once
// per run after consent and threaded through the step runner.
type execContext struct {
	ctx     context.Context
	plan    *Plan
	paths   paths.Paths
	log     *Logger
	consent ConsentPolicy
	stdin   io.Reader
	term    io.Writer
	run     runner
	prog    *Progress
	state   *State
	store   *StateStore
	retry   string // safe-retry command shown on failure

	semanticOK bool   // set by SetupPythonEnvironment for the summary
	doctorOK   bool   // set by RunDoctor
	doctorOut  string // captured doctor output
}

// allow resolves consent for a risky action, prompting when interactive and
// skipping (with a printed manual note) when consent cannot be obtained.
func (c *execContext) allow(r Risk, what string) bool {
	return askConsent(c.consent, r, c.stdin, c.term, c.log, what)
}

// execute runs steps in order. A step recorded done whose output still
// validates is skipped; otherwise it runs and its result is persisted, so a
// rerun resumes from the last safe point and stale output is re-executed.
func (c *execContext) execute(steps []step) error {
	for _, s := range steps {
		if c.state.Done(s.name) && s.valid != nil && s.valid(c) {
			c.prog.Skip(s.title)
			c.log.Logf("step %s already done; skipped", s.name)
			continue
		}
		c.prog.Start(s.title)
		c.log.Logf("step %s: start", s.name)
		if err := s.run(c); err != nil {
			c.state.Mark(s.name, StepFailed, err.Error())
			_ = c.store.Save(c.state)
			c.prog.Fail(s.title)
			return &StepError{
				Step: s.name, Cause: err, Impact: s.why,
				SafeRetry: true, Next: c.retry, LogPath: c.log.Path(),
			}
		}
		c.state.Mark(s.name, StepDone, "")
		if err := c.store.Save(c.state); err != nil {
			return fmt.Errorf("save install state: %w", err)
		}
		c.prog.Done(s.title)
		c.log.Logf("step %s: done", s.name)
	}
	return nil
}

// installSteps is the single source of truth for the install state machine. The
// plan listing, --manual checklist, and runner all derive from it.
func installSteps() []step {
	return []step{
		{name: "DetectPlatform", title: "Detect platform",
			what:   "Identify your OS and architecture.",
			why:    "Ozy resolves install locations and binaries per-platform.",
			verify: "Shown as <os>/<arch> in the plan.", next: "ResolveInstallDirs",
			run: stepDetectPlatform},
		{name: "ResolveInstallDirs", title: "Resolve install directories",
			what:   "Compute config, data, cache, state, venv, and bin paths.",
			why:    "Every later step writes only inside these resolved locations.",
			verify: "Listed under Locations in the plan.", next: "CheckExistingInstall",
			run: stepResolveInstallDirs},
		{name: "CheckExistingInstall", title: "Check for an existing install",
			what:   "Look for an existing ozy binary and config.",
			why:    "Determines whether this run is a fresh install or an update.",
			verify: "Mode line in the plan (fresh/update).", next: "CheckDependencies",
			run: stepCheckExistingInstall},
		{name: "CheckDependencies", title: "Check dependencies",
			what:   "Detect Go, Git, Python, SQLite, and the semantic backend.",
			why:    "Missing optional deps degrade gracefully; a missing required dep stops the build before any mutation.",
			verify: "Dependency table in the plan.", next: "CreateInstallRoot",
			run: stepCheckDependencies},
		{name: "CreateInstallRoot", title: "Create install directories",
			what:   "Create the resolved directories if absent.",
			why:    "Later steps need these directories to exist.",
			verify: "List the locations shown in the plan.", next: "InstallOzyBinary",
			run: stepCreateInstallRoot, valid: validInstallRoot},
		{name: "InstallOzyBinary", title: "Install the ozy binary",
			what:   "Build ozy from source with your Go toolchain into the bin dir.",
			why:    "This is the command you will run as `ozy`.",
			verify: "`ozy version` once PATH is set.", next: "CreateOrUpdateConfig",
			run: stepInstallOzyBinary, valid: validBinary},
		{name: "CreateOrUpdateConfig", title: "Create or preserve config",
			what:   "Write a default ozy.jsonc, or validate and keep an existing one.",
			why:    "Ozy reads downstream MCP servers and search settings from here.",
			verify: "`ozy doctor` reports config valid.", next: "SetupPythonEnvironment",
			run: stepCreateOrUpdateConfig, valid: validConfig},
		{name: "SetupPythonEnvironment", title: "Set up the Python environment",
			what:   "Provision a managed venv (uv → python3) for embeddings.",
			why:    "Enables semantic/hybrid search; absent Python degrades to lexical-only.",
			verify: "`ozy doctor` reports semantic available.", next: "DownloadEmbeddingAssets",
			risk: Risky, run: stepSetupPythonEnvironment},
		{name: "DownloadEmbeddingAssets", title: "Download embedding assets",
			what:   "Ensure the pinned embedding packages and model are present.",
			why:    "The semantic backend needs them to build vectors.",
			verify: "`ozy doctor` reports semantic available.", next: "BuildOrLoadToolCatalog",
			risk: Risky, run: stepDownloadEmbeddingAssets},
		{name: "BuildOrLoadToolCatalog", title: "Initialise the tool catalog",
			what:   "Open (or create on first run) the tool catalog store.",
			why:    "Holds the indexed downstream MCP tools.",
			verify: "`ozy doctor` reports the catalog.", next: "ConfigurePath",
			run: stepBuildOrLoadToolCatalog},
		{name: "ConfigurePath", title: "Configure PATH",
			what:   "Make sure the bin dir is reachable on PATH.",
			why:    "So you can run `ozy` from any shell.",
			verify: "`ozy version` resolves in a new shell.", next: "RunDoctor",
			risk: Risky, run: stepConfigurePath},
		{name: "RunDoctor", title: "Verify with ozy doctor",
			what:   "Run `ozy doctor` to verify the install.",
			why:    "Confirms config, catalog, and search mode end-to-end.",
			verify: "Doctor output below.", next: "PrintNextSteps",
			run: stepRunDoctor},
		{name: "PrintNextSteps", title: "Summary and next steps",
			what:   "Print resolved paths and the MCP-harness handoff.",
			why:    "Tells you how to wire Ozy into your agent harness.",
			verify: "n/a", next: "done",
			run: stepPrintNextSteps},
	}
}

func stepNames() []string {
	steps := installSteps()
	names := make([]string, len(steps))
	for i, s := range steps {
		names[i] = s.title
	}
	return names
}

// --- pure detection steps (cheap, no mutation, always re-run) ---

func stepDetectPlatform(c *execContext) error {
	c.log.Logf("platform: %s/%s tty=%v", c.plan.Platform.OS, c.plan.Platform.Arch, c.plan.Platform.TTY)
	return nil
}

func stepResolveInstallDirs(c *execContext) error {
	c.log.Logf("bin=%s config=%s data=%s state=%s", c.paths.BinaryPath, c.paths.ConfigFile, c.paths.DataDir, c.paths.StateDir)
	return nil
}

func stepCheckExistingInstall(c *execContext) error {
	if c.plan.IsUpdate {
		c.log.Logf("existing install detected (binary=%q config=%q)", c.plan.ExistingBinary, c.plan.ExistingConfig)
	} else {
		c.log.Logf("no existing install; fresh")
	}
	return nil
}

func stepCheckDependencies(c *execContext) error {
	for _, d := range c.plan.Deps {
		c.log.Logf("dep %s: status=%s detected=%q", d.Name, d.Status, d.Detected)
		if d.Required && d.Status == DepMissing {
			return fmt.Errorf("required dependency missing: %s — %s", d.Name, d.Fallback)
		}
	}
	return nil
}

// --- mutating steps ---

// installDirs are the directories the installer creates and owns. The venv is
// created by the sidecar provisioner, so it is not pre-created here.
func installDirs(p paths.Paths) []string {
	return []string{p.UserBinDir, p.ConfigDir, p.DataDir, p.CacheDir, p.StateDir, p.LogDir}
}

func stepCreateInstallRoot(c *execContext) error {
	for _, d := range installDirs(c.paths) {
		if err := os.MkdirAll(d, 0o700); err != nil {
			return fmt.Errorf("create %s: %w", d, err)
		}
	}
	return nil
}

func validInstallRoot(c *execContext) bool {
	for _, d := range installDirs(c.paths) {
		if !dirExists(d) {
			return false
		}
	}
	return true
}

const ozyMainPkg = "github.com/rokasklive/ozy/cmd/ozy"

func stepInstallOzyBinary(c *execContext) error {
	// A published bootstrap installs its own tag with `go install`. A dev or
	// private-repo build (no tag, or a +dirty pseudo-version) builds from the
	// local checkout instead — so the flow works before anything is published.
	if v, ok := releaseVersion(); ok {
		return installPublishedOzy(c, v)
	}
	pkgDir, ok := localOzySource(c)
	if !ok {
		return fmt.Errorf("this is a dev build but the ozy module source was not found from the " +
			"working directory — run `go run ./cmd/ozy-install` from a clone of the repo, or " +
			"publish a tag and run `go run github.com/rokasklive/ozy/cmd/ozy-install@<tag>`")
	}
	return buildLocalOzy(c, pkgDir)
}

// buildLocalOzy compiles the ozy binary from the local source tree straight into
// the resolved bin path. No GOBIN or published module is needed.
func buildLocalOzy(c *execContext, pkgDir string) error {
	c.log.Sayf("Building ozy from local source → %s", c.paths.BinaryPath)
	out, err := c.run("go", "build", "-o", c.paths.BinaryPath, pkgDir)
	if err != nil {
		return fmt.Errorf("go build %s: %w\n%s", pkgDir, err, strings.TrimSpace(out))
	}
	return nil
}

// installPublishedOzy installs a released tag with the user's Go toolchain. GOBIN
// places the binary in the user bin dir; the installer is single-goroutine, so
// mutating the process env here is safe.
func installPublishedOzy(c *execContext, version string) error {
	target := ozyMainPkg + "@" + version
	c.log.Sayf("Installing ozy (%s) → %s", target, c.paths.BinaryPath)
	prev, had := os.LookupEnv("GOBIN")
	_ = os.Setenv("GOBIN", c.paths.UserBinDir)
	defer func() {
		if had {
			_ = os.Setenv("GOBIN", prev)
		} else {
			_ = os.Unsetenv("GOBIN")
		}
	}()
	out, err := c.run("go", "install", target)
	if err != nil {
		return fmt.Errorf("go install %s: %w\n%s", target, err, strings.TrimSpace(out))
	}
	return nil
}

// localOzySource returns the local cmd/ozy package directory when the installer
// runs inside the ozy module checkout, so a dev/private build can compile from
// source. It relies on `go env GOMOD` to locate the module root.
func localOzySource(c *execContext) (string, bool) {
	out, err := c.run("go", "env", "GOMOD")
	if err != nil {
		return "", false
	}
	gomod := strings.TrimSpace(out)
	if gomod == "" || gomod == os.DevNull {
		return "", false // not inside a module
	}
	pkgDir := filepath.Join(filepath.Dir(gomod), "cmd", "ozy")
	if !dirExists(pkgDir) {
		return "", false
	}
	return pkgDir, true
}

// validBinary reports whether the ozy binary is present at the resolved path.
// ponytail: presence check only; add a `--version` comparison once self-update
// or version pinning makes a stale-but-present binary a real case.
func validBinary(c *execContext) bool { return fileExists(c.paths.BinaryPath) }

func stepCreateOrUpdateConfig(c *execContext) error {
	return NewConfigManager(c.paths.ConfigFile).Ensure(c.log)
}

func validConfig(c *execContext) bool {
	if !fileExists(c.paths.ConfigFile) {
		return false
	}
	_, cerr := config.Load(c.paths.ConfigFile)
	return cerr == nil
}

func stepSetupPythonEnvironment(c *execContext) error {
	if !c.plan.SemanticPlanned() {
		c.log.Sayf("No Python toolchain detected — continuing in lexical-only mode.")
		return nil
	}
	if !c.allow(Risky, "Provision a managed Python venv and install pinned embedding packages") {
		c.log.Sayf("Skipped Python setup — semantic search disabled for now.")
		return nil
	}
	res, err := sidecar.Provision(c.ctx, sidecar.ProvisionOptions{
		VenvDir: c.paths.VenvDir,
		Backend: config.DefaultVectorBackend,
		Model:   config.DefaultEmbeddingModel,
		Logger:  c.log,
	})
	if errors.Is(err, sidecar.ErrNoToolchain) {
		c.log.Sayf("Python toolchain unavailable — continuing in lexical-only mode.")
		return nil
	}
	if err != nil {
		return fmt.Errorf("provision venv: %w", err)
	}
	c.semanticOK = true
	c.log.Logf("venv ready at %s (python=%s)", res.VenvDir, res.PythonPath)
	return nil
}

// embeddingWarmUp downloads and verifies the embedding model. It is a package
// var so tests can drive success/failure without a real model download.
var embeddingWarmUp = realEmbeddingWarmUp

func stepDownloadEmbeddingAssets(c *execContext) error {
	if !c.semanticOK {
		// No Python / venv from the previous step: stay lexical-only. We never
		// claim the model is present without a verified load.
		c.log.Sayf("Lexical-only mode — no embedding assets to download.")
		return nil
	}
	if serr := embeddingWarmUp(c); serr != nil {
		// Semantic is optional: a warm-up failure degrades to lexical-only with
		// an actionable notice rather than aborting an otherwise-complete
		// install. The StepError content (cause, retry-safety, next command,
		// log path) is surfaced; we return nil so later steps still run.
		c.semanticOK = false
		c.log.Sayf("⚠ embedding model could not be verified — continuing in lexical-only mode.")
		c.log.Sayf("%s", serr.Error())
		c.log.Logf("embedding warm-up failed: %v", serr.Cause)
		return nil
	}
	c.log.Sayf("Embedding model downloaded and verified — semantic search ready.")
	return nil
}

// realEmbeddingWarmUp starts the (already-provisioned, marker-cached) sidecar
// and runs the readiness probe under a generous timeout: a fast liveness check
// then a warm-up that loads the model and runs a probe query. It returns an
// actionable *StepError on failure and nil on a verified success.
func realEmbeddingWarmUp(c *execContext) *StepError {
	res, err := sidecar.Provision(c.ctx, sidecar.ProvisionOptions{
		VenvDir: c.paths.VenvDir,
		Backend: config.DefaultVectorBackend,
		Model:   config.DefaultEmbeddingModel,
		Logger:  c.log,
	})
	if err != nil {
		return c.embeddingStepError(fmt.Errorf("provision venv: %w", err))
	}
	client, err := sidecar.NewClient(sidecar.Options{
		DataDir: res.VenvDir,
		Backend: config.DefaultVectorBackend,
		Model:   config.DefaultEmbeddingModel,
		Logger:  c.log,
		ProcessOptions: sidecar.ProcessOptions{
			PythonPath: res.PythonPath,
			SourceDir:  res.SourceDir,
			DataDir:    res.VenvDir,
			Backend:    config.DefaultVectorBackend,
			Model:      config.DefaultEmbeddingModel,
		},
	})
	if err != nil {
		return c.embeddingStepError(fmt.Errorf("start sidecar: %w", err))
	}
	defer func() { _ = client.Close() }()

	lctx, lcancel := context.WithTimeout(c.ctx, 15*time.Second)
	hr := client.Health(lctx)
	lcancel()
	if !hr.OK {
		return c.embeddingStepError(fmt.Errorf("sidecar liveness check failed: %w", hr.Err))
	}
	// Warm-up gets its own generous deadline so a cold model download is not
	// aborted by the short liveness budget above.
	wctx, wcancel := context.WithTimeout(c.ctx, sidecar.DefaultProvisionTimeout)
	rr := client.Ready(wctx)
	wcancel()
	if !rr.OK {
		return c.embeddingStepError(fmt.Errorf("model warm-up failed: %v", rr.Err))
	}
	return nil
}

func (c *execContext) embeddingStepError(cause error) *StepError {
	return &StepError{
		Step:      "DownloadEmbeddingAssets",
		Cause:     cause,
		Impact:    "Semantic search needs the embedding model; without it Ozy serves lexical-only.",
		SafeRetry: true,
		Next:      "ozy doctor   # diagnose, then re-run the installer or `ozy index`",
		LogPath:   c.log.Path(),
	}
}

func stepBuildOrLoadToolCatalog(c *execContext) error {
	path := filepath.Join(c.paths.StateDir, "catalog.json")
	if _, err := catalog.NewFile(path); err != nil {
		return fmt.Errorf("open tool catalog %s: %w", path, err)
	}
	// ponytail: the catalog is populated from downstream servers on first
	// `ozy mcp`; here we only confirm the store opens.
	c.log.Sayf("Tool catalog ready at %s (indexed on first `ozy mcp`).", path)
	return nil
}

func stepRunDoctor(c *execContext) error {
	out, err := c.run(c.paths.BinaryPath, "doctor")
	c.doctorOut = strings.TrimSpace(out)
	c.doctorOK = err == nil
	c.log.Logf("doctor (ok=%v):\n%s", c.doctorOK, out)
	// Doctor is verification, not mutation: a non-zero result is surfaced in the
	// summary rather than aborting an otherwise-complete install.
	if !c.doctorOK {
		c.log.Sayf("⚠ ozy doctor reported issues — see the summary and log.")
	}
	return nil
}

func stepPrintNextSteps(c *execContext) error {
	p := c.paths
	mode := "lexical-only"
	if c.semanticOK {
		mode = "semantic + lexical (hybrid)"
	}
	w := c.term
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Ozy is installed.")
	fmt.Fprintf(w, "  binary   %s\n", p.BinaryPath)
	fmt.Fprintf(w, "  config   %s\n", p.ConfigFile)
	fmt.Fprintf(w, "  data     %s\n", p.DataDir)
	fmt.Fprintf(w, "  logs     %s\n", p.LogDir)
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Status:")
	fmt.Fprintf(w, "  PATH       %s\n", boolLabel(p.Reachable(), "on PATH", "not on PATH (see above)"))
	fmt.Fprintf(w, "  config     %s\n", boolLabel(validConfig(c), "valid", "needs attention"))
	fmt.Fprintf(w, "  doctor     %s\n", boolLabel(c.doctorOK, "passed", "reported issues"))
	fmt.Fprintf(w, "  search     %s\n", mode)
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Next:")
	fmt.Fprintln(w, "  ozy doctor      # re-check anytime")
	fmt.Fprintln(w, "  ozy mcp         # run the MCP adapter")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Wire Ozy into your agent harness (MCP):")
	fmt.Fprintln(w, "  command: ozy")
	fmt.Fprintln(w, `  args:    ["mcp"]`)
	fmt.Fprintf(w, "Add downstream MCP servers under \"mcp\" in %s\n", p.ConfigFile)
	fmt.Fprintf(w, "Log: %s\n", c.log.Path())
	return nil
}

func boolLabel(ok bool, yes, no string) string {
	if ok {
		return yes
	}
	return no
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
