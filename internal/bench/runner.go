package bench

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/rokasklive/ozy/internal/eval"
)

// RunResult captures the output of a single agent run.
type RunResult struct {
	RunID       string         `json:"runId"`
	Mode        string         `json:"mode"`
	Success     bool           `json:"success"`
	TimedOut    bool           `json:"timedOut"`
	ParseFailed bool           `json:"parseFailed"`
	DurationSec float64        `json:"durationSec"`
	Transcript  string         `json:"-"`
	FinalAnswer string         `json:"finalAnswer"`
	ToolCalls   []ToolCallLog  `json:"toolCalls"`
	Grading     *GradingResult `json:"grading,omitempty"`
}

// Runner launches an agent with a task prompt and captures output.
type Runner struct {
	OpenCodePath string
	ConfigPath   string
	WorkDir      string        // per-run artifact dir (written into directly)
	FixtureDir   string        // fixture root the MCP servers read
	Servers      []benchServer // the scenario's resolved server set (direct-mode wiring)
	CallLogPath  string        // per-run server-side invocation log (OZY_BENCH_CALL_LOG)
	Timeout      time.Duration
}

// NewRunner creates a runner for the given config.
func NewRunner(configPath, workDir, fixtureDir string, timeout time.Duration) *Runner {
	return &Runner{
		OpenCodePath: os.Getenv("OPENCODE_PATH"),
		ConfigPath:   configPath,
		WorkDir:      workDir,
		FixtureDir:   fixtureDir,
		Timeout:      timeout,
	}
}

// Run launches OpenCode in non-interactive mode with the task prompt. It writes
// a project-level opencode.json (model + MCP + instructions) into a per-run
// workspace loaded via --dir, never touching user config.
//
//nolint:gosec // G301,G306: broad permissions and subprocess calls are intentional in bench harness.
func (r *Runner) Run(ctx context.Context, mode, runID, taskPrompt string) (*RunResult, error) {
	ctx, cancel := context.WithTimeout(ctx, r.Timeout)
	defer cancel()

	start := time.Now()

	openCode := r.OpenCodePath
	if openCode == "" {
		openCode = "opencode"
	}

	// The runner writes directly into the run dir it is given (e.g.
	// ozy/run-1/), with no mode-runID re-join — that re-join caused the
	// ozy/run-1/ozy-run-1/ double nesting (D8).
	absWork, err := filepath.Abs(r.WorkDir)
	if err != nil {
		return nil, fmt.Errorf("resolve work dir: %w", err)
	}
	if err := os.MkdirAll(absWork, 0o755); err != nil {
		return nil, fmt.Errorf("create work dir: %w", err)
	}

	// The agent's project dir is a clean scratch workspace — deliberately NOT the
	// fixture. The code, DB and git history live only behind the MCP servers
	// (which read OZY_BENCH_FIXTURE_DIR), so the agent can't bash/grep its way
	// around the broker: the evidence is reachable only through tools. This
	// mirrors a real deployment where the target system is remote, and is what
	// keeps the direct-vs-ozy comparison honest instead of collapsing to bash.
	workspaceDir := filepath.Join(absWork, "workspace")
	if err := os.MkdirAll(workspaceDir, 0o755); err != nil {
		return nil, fmt.Errorf("create workspace dir: %w", err)
	}
	//nolint:gosec // G306: 0644 is intentional for agent instructions in bench workspace.
	if err := os.WriteFile(filepath.Join(workspaceDir, "AGENTS.md"), []byte(remoteSystemInstruction), 0o644); err != nil {
		return nil, fmt.Errorf("write agent instructions: %w", err)
	}

	// OpenCode HOME/config/data live in an absolute per-run dir so each run is an
	// independent sample with no shared agent state. A *relative* HOME would be
	// resolved against OpenCode's own cwd and nest state where it never looks.
	stateDir := filepath.Join(absWork, "agent-home")
	dataDir := filepath.Join(stateDir, ".local", "share", "opencode")
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, fmt.Errorf("create agent state dir: %w", err)
	}

	// Model + MCP + instructions go in the workspace as project config (loaded
	// via --dir). No provider block and no auth.json: the live tier drives
	// OpenCode's built-in models, which need neither (D3).
	if err := writeOpenCodeConfig(workspaceDir, mode, r.Servers); err != nil {
		return nil, fmt.Errorf("write opencode config: %w", err)
	}

	// Write the task prompt for reference (in the artifact dir).
	//nolint:gosec // G306: 0644 is intentional for bench artifact files.
	_ = os.WriteFile(filepath.Join(absWork, "task.md"), []byte(taskPrompt), 0o644)

	cmd := exec.CommandContext(ctx,
		openCode,
		"run",
		"--format", "json",
		"--dangerously-skip-permissions",
		"--dir", workspaceDir,
		"--model", benchModel(),
		taskPrompt,
	)

	// OpenCode spawns the MCP servers as child processes that inherit its stdout
	// pipe. On the default CommandContext teardown only OpenCode is SIGKILLed, so
	// on a timeout its MCP children are orphaned holding the pipe open — and
	// cmd.Wait() blocks forever waiting for EOF that never comes (the run "hangs"
	// past its timeout).
	setProcessGroupKill(cmd)

	cmd.Dir = workspaceDir
	cmd.Env = append(os.Environ(),
		"HOME="+stateDir,
		"XDG_CONFIG_HOME="+filepath.Join(stateDir, ".config"),
		"XDG_DATA_HOME="+filepath.Join(stateDir, ".local", "share"),
		// Hermeticity: point npm at an unroutable registry so OpenCode's
		// opportunistic plugin auto-install never fetches at run time. Verified
		// that `opencode run` completes fine offline — the model gateway is the
		// only egress a run needs. Without this, each fresh per-run HOME triggers
		// a plugin fetch that both breaks hermeticity and litters .npm/_cacache
		// + node_modules debris into the run dir.
		"npm_config_registry=http://127.0.0.1:0",
	)
	// Server-side invocation log: every fixture server (direct-mode children and
	// ozy-mode downstream grandchildren alike) inherits this path and appends its
	// calls, giving a mode-symmetric record of downstream tool selection (D4).
	if r.CallLogPath != "" {
		cmd.Env = append(cmd.Env, callLogEnv+"="+r.CallLogPath)
	}
	// Functional pdf-toolkit tools resolve relative output paths under this dir,
	// so the grader finds the deliverable (e.g. output/report.pdf) at a known
	// per-run path. Inherited by all fixture servers in both modes.
	cmd.Env = append(cmd.Env, outputDirEnv+"="+workspaceDir)

	// Tee OpenCode's combined output to both the transcript file (for later
	// analysis) and stderr (for real-time visibility in docker logs).
	transcriptPath := filepath.Join(absWork, "transcript.jsonl")
	tf, err := os.Create(transcriptPath)
	if err != nil {
		return nil, fmt.Errorf("create transcript file: %w", err)
	}
	defer func() { _ = tf.Close() }()

	// Use a pipe to capture combined stdout+stderr.
	pr, pw := io.Pipe()
	cmd.Stdout = pw
	cmd.Stderr = pw

	// Tee goroutine: copy pipe to both stderr and transcript.
	teeDone := make(chan struct{})
	go func() {
		defer close(teeDone)
		tee := io.TeeReader(pr, os.Stderr)
		_, _ = io.Copy(tf, tee)
	}()

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start opencode: %w", err)
	}

	// Heartbeat goroutine — polls transcript file size.
	done := make(chan struct{})
	go func() {
		tick := time.NewTicker(15 * time.Second)
		defer tick.Stop()
		for {
			select {
			case <-done:
				return
			case <-tick.C:
				elapsed := time.Since(start).Round(time.Second)
				fi, _ := os.Stat(transcriptPath)
				size := int64(0)
				if fi != nil {
					size = fi.Size()
				}
				fmt.Fprintf(os.Stderr, "[%s %s] %v elapsed, transcript: %d bytes\n",
					mode, runID, elapsed, size)
			}
		}
	}()

	// Wait for process to finish, then close the write end so the tee goroutine
	// drains and finishes.
	_ = cmd.Wait()
	_ = pw.Close()
	<-teeDone
	close(done)

	duration := time.Since(start).Seconds()
	timedOut := ctx.Err() != nil

	// Re-read transcript for final answer and tool calls. A non-empty transcript
	// that parses to zero events is an output-format drift — a harness failure,
	// not an agent failure (scenario-bench: "Agent output parsing fails loudly").
	finalAnswer, toolCalls, events := parseTranscript(transcriptPath)
	parseFailed := false
	if fi, err := os.Stat(transcriptPath); err == nil && fi.Size() > 0 && events == 0 {
		parseFailed = true
		fmt.Fprintf(os.Stderr, "[%s %s] PARSE FAILED: %d-byte transcript yielded no parsable events\n",
			mode, runID, fi.Size())
	}

	result := &RunResult{
		RunID:       runID,
		Mode:        mode,
		TimedOut:    timedOut,
		ParseFailed: parseFailed,
		DurationSec: duration,
		Transcript:  transcriptPath,
		FinalAnswer: finalAnswer,
		ToolCalls:   toolCalls,
	}

	if timedOut {
		fmt.Fprintf(os.Stderr, "[%s %s] TIMED OUT after %.0fs\n", mode, runID, duration)
	}

	return result, nil
}

// setProcessGroupKill makes a context timeout actually tear a run down. It puts
// the command in its own process group and, on cancel, SIGKILLs the whole group
// so any child processes (OpenCode's MCP servers) die with it instead of being
// orphaned holding the stdout pipe — which is what makes cmd.Wait() hang forever
// past the timeout. WaitDelay bounds the post-exit I/O wait as a final backstop.
func setProcessGroupKill(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process != nil {
			// Negative pid targets the process group led by cmd (its own, distinct
			// from ozy-bench's), never the harness itself.
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		}
		return nil
	}
	cmd.WaitDelay = 10 * time.Second
}

// parseTranscript reads the transcript file and extracts the final answer
// (all JSON text events concatenated) and tool call names. The third return is
// the count of recognized events (text + tool_use) — a non-empty transcript with
// zero recognized events signals output-format drift to the caller.
//
//nolint:gosec // G304: path is a controlled artifact path in bench output.
func parseTranscript(path string) (string, []ToolCallLog, int) {
	f, err := os.Open(path)
	if err != nil {
		return "", nil, 0
	}
	defer func() { _ = f.Close() }()

	var answer strings.Builder
	var calls []ToolCallLog
	events := 0
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()
		var event struct {
			Type string `json:"type"`
			Part struct {
				Text string `json:"text"`
				Name string `json:"name"`
				Tool string `json:"tool"`
			} `json:"part"`
		}
		if json.Unmarshal(line, &event) != nil {
			continue
		}
		if event.Type == "text" && event.Part.Text != "" {
			answer.WriteString(event.Part.Text)
			events++
		}
		// OpenCode emits tool calls as {"type":"tool_use","part":{"type":"tool",
		// "tool":"<server>_<name>"}}. Names are namespaced by MCP server.
		if event.Type == "tool_use" {
			name := event.Part.Tool
			if name == "" {
				name = event.Part.Name
			}
			if name != "" {
				calls = append(calls, ToolCallLog{Tool: name})
				events++
			}
		}
	}
	return strings.TrimSpace(answer.String()), calls, events
}

// Orchestrator manages the full benchmark lifecycle.
type Orchestrator struct {
	Scenario    *ScenarioConfig
	FixtureDir  string
	CorpusDir   string
	RunDir      string
	NumRuns     int
	Mode        string              // "direct", "ozy", or "both"
	SurfaceOnly bool                // skip the live tier; surface + surface-only comparison only
	Estimator   eval.TokenEstimator // nil → eval.DefaultEstimator
}

// Run executes the full benchmark: the always-on static surface tier, then the
// live tier per mode (unless SurfaceOnly), then aggregates, provenance, and the
// cross-mode comparison. It returns an error only for harness failures; a mode
// or run that fails the task is recorded, never a non-nil error.
//
//nolint:gosec // G301,G306,G204: broad permissions and subprocess calls are intentional in bench harness.
func (o *Orchestrator) Run(ctx context.Context) error {
	est := o.Estimator
	if est == nil {
		est = eval.DefaultEstimator
	}

	modes := []string{o.Mode}
	switch o.Mode {
	case "all":
		modes = []string{"direct", "direct-lean", "ozy"}
	case "both":
		modes = []string{"direct", "ozy"}
	}

	if err := os.MkdirAll(o.RunDir, 0o755); err != nil {
		return fmt.Errorf("create run dir: %w", err)
	}

	// Resolve the scenario's estate once: functional toolsets + corpus servers.
	// Direct config, ozy downstream, and the surface all derive from this set.
	servers, err := scenarioServers(o.Scenario.Toolsets, o.Scenario.CorpusEnabled(), o.FixtureDir, o.CorpusDir)
	if err != nil {
		return fmt.Errorf("resolve scenario servers: %w", err)
	}

	// Ground truth is scenario-level: its required tools are the canonical set
	// the surface's irrelevant-token accounting and retrieval metrics score
	// against; it also drives grading.
	gt, err := LoadGroundTruth(o.Scenario.ResolvePath(o.Scenario.GroundTruth))
	if err != nil {
		return fmt.Errorf("load ground truth: %w", err)
	}

	// --- Static surface tier: always, before any live run, no model. ---
	surface, err := ComputeSurfaceComparison(servers, o.FixtureDir, o.CorpusDir, gt.RequiredTools, est)
	if err != nil {
		return fmt.Errorf("compute surface: %w", err)
	}
	if err := WriteSurface(filepath.Join(o.RunDir, "surface.json"), surface); err != nil {
		return fmt.Errorf("write surface: %w", err)
	}
	leanTools, leanTok := 0, 0
	if surface.DirectLean != nil {
		leanTools, leanTok = surface.DirectLean.ToolsVisible, surface.DirectLean.SchemaTokens
	}
	fmt.Fprintf(os.Stderr, "surface: direct=%d tools/%d tok, direct-lean=%d tools/%d tok, ozy=%d tools/%d tok\n",
		surface.Direct.ToolsVisible, surface.Direct.SchemaTokens,
		leanTools, leanTok,
		surface.Ozy.ToolsVisible, surface.Ozy.SchemaTokens)

	// --- Surface-only: no model, no live runs. Write provenance + comparison. ---
	if o.SurfaceOnly {
		prov, err := BuildProvenance(o.Scenario, modes, o.NumRuns, est.Name(), "skipped")
		if err != nil {
			return fmt.Errorf("build provenance: %w", err)
		}
		if err := WriteProvenance(filepath.Join(o.RunDir, "environment.json"), prov); err != nil {
			return fmt.Errorf("write provenance: %w", err)
		}
		return WriteComparison(o.RunDir, surface, nil, prov)
	}

	// --- Preflight: an unresolvable model fails fast before any live run. ---
	openCode := os.Getenv("OPENCODE_PATH")
	if openCode == "" {
		openCode = "opencode"
	}
	if err := resolveModel(ctx, openCode, benchModel()); err != nil {
		return fmt.Errorf("model preflight: %w", err)
	}

	// Culprit hash reaches grading through the fixture metadata, never hardcoded.
	culpritHash := ""
	if meta, err := ReadFixtureMeta(o.FixtureDir); err == nil {
		culpritHash = meta.CulpritHash
	} else {
		fmt.Fprintf(os.Stderr, "warning: no fixture-meta.json (%v); culprit check falls back to subject match\n", err)
	}

	// --- Live tier per mode. ---
	aggregates := map[string]*Aggregate{}
	for _, mode := range modes {
		modeDir := filepath.Join(o.RunDir, mode)
		if err := os.MkdirAll(modeDir, 0o755); err != nil {
			return fmt.Errorf("create mode dir: %w", err)
		}

		surfaceTokens := surface.Direct.SchemaTokens
		switch {
		case mode == "ozy":
			surfaceTokens = surface.Ozy.SchemaTokens
		case mode == "direct-lean" && surface.DirectLean != nil:
			surfaceTokens = surface.DirectLean.SchemaTokens
		}

		var metrics []*RunMetrics
		for i := 1; i <= o.NumRuns; i++ {
			runID := fmt.Sprintf("run-%d", i)
			runDir := filepath.Join(modeDir, runID)
			if err := os.MkdirAll(runDir, 0o755); err != nil {
				return fmt.Errorf("create run dir: %w", err)
			}

			// ozy mode is self-setup, re-indexed into a fresh per-run catalog so
			// no catalog or Ozy state carries over from a prior run (the fixture
			// is immutable, so the index is deterministic — this is isolation, not
			// a different result).
			if mode == "ozy" {
				if err := setupOzy(ctx, o.FixtureDir, servers, runDir); err != nil {
					fmt.Fprintf(os.Stderr, "mode=ozy %d/%d: setup failed, skipping run: %v\n", i, o.NumRuns, err)
					continue
				}
			}

			fmt.Fprintf(os.Stderr, "mode=%s %d/%d: starting...\n", mode, i, o.NumRuns)

			timeout := time.Duration(o.Scenario.Limits.TimeoutSeconds) * time.Second
			if s := os.Getenv("BENCH_TIMEOUT"); s != "" {
				if n, err := strconv.Atoi(s); err == nil && n > 0 {
					timeout = time.Duration(n) * time.Second
				}
			}

			configPath := "bench/configs/opencode." + mode + ".jsonc"
			// Absolute: the fixture servers run with their own cwd (ozy's cwd in
			// ozy mode), so a relative path would be written outside the run dir —
			// or fail silently — and never reach the grader.
			callLogPath, err := filepath.Abs(filepath.Join(runDir, "calls.jsonl"))
			if err != nil {
				return fmt.Errorf("resolve call log path: %w", err)
			}
			runner := NewRunner(configPath, runDir, o.FixtureDir, timeout)
			runner.Servers = servers
			runner.CallLogPath = callLogPath

			taskData, err := os.ReadFile(o.Scenario.ResolvePath(o.Scenario.TaskFile))
			if err != nil {
				return fmt.Errorf("read task file: %w", err)
			}

			result, err := runner.Run(ctx, mode, runID, string(taskData))
			if err != nil {
				fmt.Fprintf(os.Stderr, "mode=%s %d/%d: error: %v\n", mode, i, o.NumRuns, err)
				continue
			}

			// The server-side invocation log is the mode-symmetric record of
			// downstream tool selection — used by both grading and retrieval metrics.
			callLog := LoadCallLog(callLogPath)

			// Grade the result (skipped when the transcript failed to parse — an
			// empty answer is a harness problem, not a scored task failure).
			if !result.ParseFailed {
				grading := Grade(gt, GradeInput{
					FinalAnswer: result.FinalAnswer,
					ToolCalls:   callLog,
					CulpritHash: culpritHash,
					OutputDir:   filepath.Join(runDir, "workspace"),
				})
				result.Grading = grading
				result.Success = grading.Overall
				if err := WriteGradingResult(filepath.Join(runDir, "grading.json"), grading); err != nil {
					fmt.Fprintf(os.Stderr, "mode=%s %d/%d: write grading: %v\n", mode, i, o.NumRuns, err)
				}
			}

			// Per-run metrics: measured from usage events, else estimated.
			m := ComputeMetrics(result, surfaceTokens, est, RetrievalInput{
				Calls:              callLog,
				RequiredTools:      gt.RequiredTools,
				FunctionalToolsets: o.Scenario.Toolsets,
			})
			if err := WriteMetrics(filepath.Join(runDir, "metrics.json"), m); err != nil {
				fmt.Fprintf(os.Stderr, "mode=%s %d/%d: write metrics: %v\n", mode, i, o.NumRuns, err)
			}
			metrics = append(metrics, m)

			// Write final answer.
			if err := os.WriteFile(filepath.Join(runDir, "final-answer.md"), []byte(result.FinalAnswer), 0o644); err != nil {
				fmt.Fprintf(os.Stderr, "mode=%s %d/%d: write final answer: %v\n", mode, i, o.NumRuns, err)
			}

			// Write tool calls.
			toolCallsPath := filepath.Join(runDir, "tool-calls.jsonl")
			tf, err := os.Create(toolCallsPath)
			if err == nil {
				enc := json.NewEncoder(tf)
				for _, tc := range result.ToolCalls {
					_ = enc.Encode(tc)
				}
				_ = tf.Close()
			}

			fmt.Fprintf(os.Stderr, "mode=%s %d/%d: done (%.1fs, pass=%v, timed_out=%v, parse_failed=%v)\n",
				mode, i, o.NumRuns, result.DurationSec, result.Success, result.TimedOut, result.ParseFailed)
		}

		agg := ComputeAggregate(mode, metrics)
		if err := WriteAggregate(filepath.Join(modeDir, "aggregate.json"), agg); err != nil {
			fmt.Fprintf(os.Stderr, "mode=%s: write aggregate: %v\n", mode, err)
		}
		aggregates[mode] = agg
	}

	// --- Provenance + cross-mode comparison. ---
	prov, err := BuildProvenance(o.Scenario, modes, o.NumRuns, est.Name(), usageSourceOf(aggregates))
	if err != nil {
		return fmt.Errorf("build provenance: %w", err)
	}
	if err := WriteProvenance(filepath.Join(o.RunDir, "environment.json"), prov); err != nil {
		return fmt.Errorf("write provenance: %w", err)
	}
	return WriteComparison(o.RunDir, surface, aggregates, prov)
}

// remoteSystemInstruction is written as AGENTS.md into the agent's (empty)
// workspace. It tells the agent the target system isn't local, so it reaches
// for the provided tools instead of burning turns probing an empty directory.
const remoteSystemInstruction = `# Investigation environment

You are investigating a REMOTE system. Its source code, databases, and git
history are NOT in this working directory — there are no local files to read,
grep, or shell out to.

Everything you need is exposed through the tools available to you. Use them to
search the code, read files, query data stores, and inspect version history.
Begin by discovering which tools are available and what each one does.
`

// writeOpenCodeConfig writes opencode.json into dir: the built-in model ID, the
// mode's MCP servers, and the AGENTS.md instruction. No provider block and no
// auth file — the live tier drives OpenCode's built-in models (D3). MCP belongs
// under the top-level "mcp" key; a separate .opencode/mcp.json is not read.
//
//nolint:gosec // G306: 0644 permissions are intentional for bench config files.
func writeOpenCodeConfig(dir, mode string, servers []benchServer) error {
	cfg := map[string]any{
		"$schema":      "https://opencode.ai/config.json",
		"model":        benchModel(),
		"mcp":          mcpServersFor(mode, servers),
		"instructions": []string{"AGENTS.md"},
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create opencode config dir: %w", err)
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal opencode config: %w", err)
	}
	return os.WriteFile(filepath.Join(dir, "opencode.json"), data, 0o644)
}

// setupOzy makes ozy mode self-contained instead of a manual pre-step: it
// writes an ozy config listing the same fixture MCP servers direct mode uses,
// then runs `ozy index` to populate a per-run catalog. It sets OZY_CONFIG and
// OZY_CATALOG (absolute) so both the index here and the `ozy mcp` broker that
// OpenCode launches read the same catalog. Without it, `ozy mcp` would serve
// the user's default, unindexed config and advertise nothing.
//
//nolint:gosec // G204,G306: subprocess and permissions are intentional in bench harness.
func setupOzy(ctx context.Context, fixtureDir string, servers []benchServer, dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create ozy dir: %w", err)
	}
	cfgPath, err := filepath.Abs(filepath.Join(dir, "ozy.jsonc"))
	if err != nil {
		return fmt.Errorf("resolve ozy config path: %w", err)
	}
	catalogPath, err := filepath.Abs(filepath.Join(dir, "ozy-catalog.json"))
	if err != nil {
		return fmt.Errorf("resolve ozy catalog path: %w", err)
	}

	// The downstream servers are exactly direct mode's fixture servers, nested
	// under the "mcp" key ozy's config loader expects. Semantic + the embedding
	// model/backend are pinned explicitly so the runtime sidecar marker matches
	// the venv baked into the image (see bench/Dockerfile): a match means `ozy
	// index` reuses the baked venv+model offline instead of attempting a fetch.
	// Semantic is already default-on in ozy config; pinning is cheap insurance.
	cfg := map[string]any{
		"mcp":    mcpServersFor("direct", servers),
		"search": map[string]any{"semantic": map[string]any{"enabled": true}},
		"embedding": map[string]any{
			"model":         benchEmbeddingModel,
			"vectorBackend": benchVectorBackend,
		},
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal ozy config: %w", err)
	}
	if err := os.WriteFile(cfgPath, data, 0o644); err != nil {
		return fmt.Errorf("write ozy config: %w", err)
	}

	_ = os.Setenv("OZY_CONFIG", cfgPath)
	_ = os.Setenv("OZY_CATALOG", catalogPath)

	ozyBin := os.Getenv("OZY_BIN")
	if ozyBin == "" {
		ozyBin = "ozy"
	}
	cmd := exec.CommandContext(ctx, ozyBin, "index")
	cmd.Dir = fixtureDir
	cmd.Stdout, cmd.Stderr = os.Stderr, os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("ozy index: %w", err)
	}
	return nil
}

// benchServer is one MCP server in a scenario's estate: a name, the
// `ozy-bench mcp` args that serve it, and whether it is a corpus stub (vs a
// functional toolset). One resolver feeds direct-mode config, ozy's downstream
// config, and the static surface enumeration, so they cannot diverge.
type benchServer struct {
	Name   string
	Args   []string
	Corpus bool
}

// benchBin returns the fixture binary path (OZY_BENCH_BIN or a resolved
// ozy-bench).
func benchBin() string {
	if b := os.Getenv("OZY_BENCH_BIN"); b != "" {
		return b
	}
	b, _ := filepath.Abs("ozy-bench")
	return b
}

// ozyBin returns the ozy broker binary path.
func ozyBin() string {
	if b := os.Getenv("OZY_BIN"); b != "" {
		return b
	}
	return "ozy"
}

// toolsetNeedsFixture reports whether a functional toolset reads the scenario
// fixture dir. pdf-toolkit writes to the agent workspace and reads no baked
// fixture data, so it needs none; the search/lookup toolsets do.
func toolsetNeedsFixture(ts string) bool {
	return ts != "pdf-toolkit"
}

// scenarioServers resolves the ordered MCP server set for a scenario: its
// functional toolsets first, then every corpus server when the corpus attaches.
// fixtureDir/corpusDir are substituted into the served commands (callers pass
// real paths for the runner, or {env:...} placeholders for the checked-in
// templates), so the wired set is identical across consumers.
func scenarioServers(toolsets []string, corpus bool, fixtureDir, corpusDir string) ([]benchServer, error) {
	var servers []benchServer
	for _, ts := range toolsets {
		args := []string{"--toolset", ts}
		if toolsetNeedsFixture(ts) {
			args = append(args, "--fixture-dir", fixtureDir)
		}
		servers = append(servers, benchServer{Name: ts, Args: args})
	}
	if corpus {
		names, err := CorpusServerNames(corpusDir)
		if err != nil {
			return nil, fmt.Errorf("enumerate corpus servers: %w", err)
		}
		for _, n := range names {
			args := []string{"--server", n, "--corpus-dir", corpusDir}
			// A corpus server hosting a functional:search tool reads the baked
			// fixture corpus, so it needs the fixture dir like the toolsets do (D3).
			if CorpusServerNeedsFixture(corpusDir, n) {
				args = append(args, "--fixture-dir", fixtureDir)
			}
			servers = append(servers, benchServer{
				Name:   n,
				Args:   args,
				Corpus: true,
			})
		}
	}
	return servers, nil
}

// mcpServersFor returns the OpenCode MCP server map for the given mode: direct
// exposes every scenario server; direct-lean exposes only the functional
// toolsets (the corpus is dropped — the "installed only the MCPs I need"
// baseline); ozy exposes only the broker (which reaches the full set downstream
// after `ozy index`).
func mcpServersFor(mode string, servers []benchServer) map[string]any {
	if mode == "ozy" {
		// Forward the per-run config + catalog (written by setupOzy) so the broker
		// serves the indexed fixture catalog rather than the user's default config.
		env := map[string]string{}
		if v := os.Getenv("OZY_CONFIG"); v != "" {
			env["OZY_CONFIG"] = v
		}
		if v := os.Getenv("OZY_CATALOG"); v != "" {
			env["OZY_CATALOG"] = v
		}
		fixtureDir := os.Getenv("OZY_BENCH_FIXTURE_DIR")
		if fixtureDir == "" {
			fixtureDir = "/tmp/ozy-bench-fixture"
		}
		return map[string]any{
			"ozy": map[string]any{
				"type":        "local",
				"command":     []string{ozyBin(), "mcp"},
				"cwd":         fixtureDir,
				"environment": env,
				"enabled":     true,
			},
		}
	}

	bin := benchBin()
	out := make(map[string]any, len(servers))
	for _, s := range servers {
		// direct-lean wires the functional toolsets whole (all their tools,
		// task-critical and sibling alike) but never the corpus stubs.
		if mode == "direct-lean" && s.Corpus {
			continue
		}
		out[s.Name] = map[string]any{
			"type":    "local",
			"command": append([]string{bin, "mcp"}, s.Args...),
			"enabled": true,
		}
	}
	return out
}

// DefaultBenchModel is the pinned free model the zero-config bench drives.
// Confirmed resolvable by the pinned OpenCode (1.17.7) via `opencode models`;
// its free tier lists big-pickle, deepseek-v4-flash-free, nemotron-3-ultra-free,
// hy3-free, mimo-v2.5-free, and north-mini-code-free. resolveModel fails fast if
// this ever stops resolving. big-pickle is the default over deepseek-v4-flash-free
// because the latter stalls mid-stream (silent gateway hang) often enough to wedge
// runs; big-pickle completes a full three-mode pass reliably.
const DefaultBenchModel = "opencode/big-pickle"

// benchEmbeddingModel and benchVectorBackend pin ozy's semantic stack to exactly
// what bench/Dockerfile bakes into the image. setupOzy writes these into the
// per-run config so the runtime sidecar-provision marker matches the baked venv
// (no runtime reprovision or model fetch). They mirror config.DefaultEmbeddingModel
// and config.DefaultVectorBackend; kept as local literals to avoid a config import.
const (
	benchEmbeddingModel = "BAAI/bge-small-en-v1.5"
	benchVectorBackend  = "turbovec"
)

// benchModel returns the effective model ID: BENCH_MODEL env override, else the
// pinned free default.
func benchModel() string {
	if m := os.Getenv("BENCH_MODEL"); m != "" {
		return m
	}
	return DefaultBenchModel
}

// resolveModel fails fast when the selected model ID cannot be resolved by the
// pinned OpenCode, before any live run is attempted. It checks membership in
// `opencode models`. If that command is unavailable or errors for an unrelated
// reason, it does not block — the first run then surfaces a real model error.
func resolveModel(ctx context.Context, openCode, model string) error {
	//nolint:gosec // G204: opencode is the pinned agent binary in the bench image.
	out, err := exec.CommandContext(ctx, openCode, "models").Output()
	if err != nil {
		return nil // can't enumerate; let the run surface a bad model
	}
	for _, line := range strings.Split(string(out), "\n") {
		if strings.TrimSpace(line) == model {
			return nil
		}
	}
	return fmt.Errorf("model %q is not resolvable by the pinned OpenCode (not listed by `opencode models`); set BENCH_MODEL to a resolvable model", model)
}
