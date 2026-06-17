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
	"time"
)

// RunResult captures the output of a single agent run.
type RunResult struct {
	RunID       string         `json:"runId"`
	Mode        string         `json:"mode"`
	Success     bool           `json:"success"`
	TimedOut    bool           `json:"timedOut"`
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
	WorkDir      string // volume-mounted dir for artifacts
	FixtureDir   string // OpenCode's working directory
	Timeout      time.Duration
	Spy          *ContextSpy // per-run ContextSpy session control (best-effort)
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

// Run launches OpenCode in non-interactive mode with the task prompt.
// It creates a project-level opencode.json and .opencode/mcp.json in the work
// directory, derived from environment variables — never touching user config.
func (r *Runner) Run(ctx context.Context, mode, runID, taskPrompt string) (*RunResult, error) {
	ctx, cancel := context.WithTimeout(ctx, r.Timeout)
	defer cancel()

	start := time.Now()

	openCode := r.OpenCodePath
	if openCode == "" {
		openCode = "opencode"
	}

	workDir := r.WorkDir + "/" + mode + "-" + runID
	absWork, err := filepath.Abs(workDir)
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
	if err := os.WriteFile(filepath.Join(workspaceDir, "AGENTS.md"), []byte(remoteSystemInstruction), 0o644); err != nil {
		return nil, fmt.Errorf("write agent instructions: %w", err)
	}

	// OpenCode HOME/config/data live in an absolute per-run dir. A *relative*
	// HOME is resolved against OpenCode's own cwd and nests state under
	// <cwd>/<HOME>, so the auth file lands where OpenCode never looks.
	stateDir := filepath.Join(absWork, "agent-home")
	dataDir := filepath.Join(stateDir, ".local", "share", "opencode")
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, fmt.Errorf("create agent state dir: %w", err)
	}

	// Provider + MCP + instructions go in the workspace as project config
	// (loaded via --dir); auth goes in the agent data dir.
	if err := writeOpenCodeConfig(workspaceDir, mode); err != nil {
		return nil, fmt.Errorf("write opencode config: %w", err)
	}
	if err := writeAuthConfig(dataDir); err != nil {
		return nil, fmt.Errorf("write auth config: %w", err)
	}

	// Write the task prompt for reference (in the artifact dir).
	_ = os.WriteFile(filepath.Join(absWork, "task.md"), []byte(taskPrompt), 0o644)

	cmd := exec.CommandContext(ctx,
		openCode,
		"run",
		"--format", "json",
		"--dangerously-skip-permissions",
		"--dir", workspaceDir,
		"--model", modelName(),
		taskPrompt,
	)

	cmd.Dir = workspaceDir
	cmd.Env = append(os.Environ(),
		"HOME="+stateDir,
		"XDG_CONFIG_HOME="+filepath.Join(stateDir, ".config"),
		"XDG_DATA_HOME="+filepath.Join(stateDir, ".local", "share"),
	)

	// Tee OpenCode's combined output to both the transcript file (for later
	// analysis) and stderr (for real-time visibility in docker logs).
	transcriptPath := filepath.Join(absWork, "transcript.jsonl")
	tf, err := os.Create(transcriptPath)
	if err != nil {
		return nil, fmt.Errorf("create transcript file: %w", err)
	}
	defer tf.Close()

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

	// Open a ContextSpy session so every model request this run makes is tagged
	// with it — that's what gives a per-run token breakdown. session is unique
	// per run; Breakdown reads the most recent session of that name.
	session := mode + "-" + runID
	r.Spy.StartSession(ctx, session)

	if err := cmd.Start(); err != nil {
		r.Spy.EndSession(context.Background())
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
	pw.Close()
	<-teeDone
	close(done)

	// Close the session and pull its breakdown. Use a fresh context — the run
	// ctx may already be cancelled/timed out, but the capture must still flush.
	r.Spy.EndSession(context.Background())
	if bd, err := r.Spy.Breakdown(context.Background(), session); err != nil {
		fmt.Fprintf(os.Stderr, "[%s %s] contextspy breakdown: %v\n", mode, runID, err)
	} else if bd != nil {
		if err := WriteBreakdown(filepath.Join(absWork, "context-breakdown.json"), bd); err != nil {
			fmt.Fprintf(os.Stderr, "[%s %s] write breakdown: %v\n", mode, runID, err)
		} else {
			fmt.Fprintf(os.Stderr, "[%s %s] contextspy: %d requests captured, %d tool-def tokens (total input %d)\n",
				mode, runID, len(bd.Requests), bd.Totals.ToolDefinitions, bd.Totals.TotalInput)
		}
	}

	duration := time.Since(start).Seconds()
	timedOut := ctx.Err() != nil

	// Re-read transcript for final answer and tool calls.
	finalAnswer, toolCalls := parseTranscript(transcriptPath)

	result := &RunResult{
		RunID:       runID,
		Mode:        mode,
		TimedOut:    timedOut,
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

// parseTranscript reads the transcript file and extracts the final answer
// (all JSON text events concatenated) and tool call names.
func parseTranscript(path string) (string, []ToolCallLog) {
	f, err := os.Open(path)
	if err != nil {
		return "", nil
	}
	defer f.Close()

	var answer strings.Builder
	var calls []ToolCallLog
	scanner := bufio.NewScanner(f)

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
			}
		}
	}
	return strings.TrimSpace(answer.String()), calls
}
func parseToolCalls(workDir string) []ToolCallLog {
	var calls []ToolCallLog

	path := filepath.Join(workDir, "tool-calls.jsonl")
	f, err := os.Open(path)
	if err != nil {
		return calls
	}
	defer f.Close()

	dec := json.NewDecoder(f)
	var call struct {
		Tool   string `json:"tool"`
		Server string `json:"server,omitempty"`
	}
	for {
		if err := dec.Decode(&call); err == io.EOF {
			break
		} else if err != nil {
			break
		}
		calls = append(calls, ToolCallLog{
			Tool:   call.Tool,
			Server: call.Server,
		})
	}
	return calls
}

// Orchestrator manages the full benchmark lifecycle.
type Orchestrator struct {
	Scenario   *ScenarioConfig
	FixtureDir string
	RunDir     string
	NumRuns    int
	Mode       string // "direct", "ozy", or "both"
}

// Run executes the full benchmark.
func (o *Orchestrator) Run(ctx context.Context) error {
	modes := []string{o.Mode}
	if o.Mode == "both" {
		modes = []string{"direct", "ozy"}
	}

	if err := os.MkdirAll(o.RunDir, 0o755); err != nil {
		return fmt.Errorf("create run dir: %w", err)
	}

	spy := NewContextSpy()

	// Write provenance.
	provenance, err := BuildProvenance(o.Scenario, modes, o.NumRuns)
	if err != nil {
		return fmt.Errorf("build provenance: %w", err)
	}
	if err := WriteProvenance(filepath.Join(o.RunDir, "environment.json"), provenance); err != nil {
		return fmt.Errorf("write provenance: %w", err)
	}

	for _, mode := range modes {
		modeDir := filepath.Join(o.RunDir, mode)
		if err := os.MkdirAll(modeDir, 0o755); err != nil {
			return fmt.Errorf("create mode dir: %w", err)
		}

		// ozy mode is self-setup: write its config + index the fixture catalog
		// once per mode (the fixture is identical across this mode's runs).
		if mode == "ozy" {
			if err := setupOzy(ctx, o.FixtureDir, modeDir); err != nil {
				fmt.Fprintf(os.Stderr, "mode=ozy: setup failed, skipping mode: %v\n", err)
				continue
			}
		}

		for i := 1; i <= o.NumRuns; i++ {
			runID := fmt.Sprintf("run-%d", i)
			runDir := filepath.Join(modeDir, runID)
			if err := os.MkdirAll(runDir, 0o755); err != nil {
				return fmt.Errorf("create run dir: %w", err)
			}

			fmt.Fprintf(os.Stderr, "mode=%s %d/%d: starting...\n", mode, i, o.NumRuns)

			timeout := time.Duration(o.Scenario.Limits.TimeoutSeconds) * time.Second
			if s := os.Getenv("BENCH_TIMEOUT"); s != "" {
				if n, err := strconv.Atoi(s); err == nil && n > 0 {
					timeout = time.Duration(n) * time.Second
				}
			}

			configPath := "bench/configs/opencode." + mode + ".jsonc"
			runner := NewRunner(configPath, runDir, o.FixtureDir, timeout)
			runner.Spy = spy

			taskData, err := os.ReadFile(o.Scenario.ResolvePath(o.Scenario.TaskFile))
			if err != nil {
				return fmt.Errorf("read task file: %w", err)
			}

			result, err := runner.Run(ctx, mode, runID, string(taskData))
			if err != nil {
				fmt.Fprintf(os.Stderr, "mode=%s %d/%d: error: %v\n", mode, i, o.NumRuns, err)
				continue
			}

			// Grade the result.
			gtPath := o.Scenario.ResolvePath(o.Scenario.GroundTruth)
			gt, err := LoadGroundTruth(gtPath)
			if err != nil {
				fmt.Fprintf(os.Stderr, "mode=%s %d/%d: load ground truth: %v\n", mode, i, o.NumRuns, err)
				continue
			}

			// Get culprit hash from fixture meta.
			culpritHash := ""
			grading := Grade(gt, result.FinalAnswer, result.ToolCalls, culpritHash)
			result.Grading = grading
			result.Success = grading.Overall

			if err := WriteGradingResult(filepath.Join(runDir, "grading.json"), grading); err != nil {
				fmt.Fprintf(os.Stderr, "mode=%s %d/%d: write grading: %v\n", mode, i, o.NumRuns, err)
			}

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
					enc.Encode(tc)
				}
				tf.Close()
			}

			fmt.Fprintf(os.Stderr, "mode=%s %d/%d: done (%.1fs, pass=%v, timed_out=%v)\n",
				mode, i, o.NumRuns, result.DurationSec, result.Success, result.TimedOut)
		}
	}

	return nil
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

// writeOpenCodeConfig writes opencode.json into dir with a single provider
// derived from MODEL_BASE_URL / MODEL_API_KEY / MODEL_NAME and the mode's MCP
// servers. MCP belongs under the top-level "mcp" key in opencode.json — a
// separate .opencode/mcp.json is not read by OpenCode.
func writeOpenCodeConfig(dir, mode string) error {
	baseURL := os.Getenv("MODEL_BASE_URL")
	if baseURL == "" {
		baseURL = "http://localhost:8888/v1"
	}
	apiKey := os.Getenv("MODEL_API_KEY")
	name := modelName()
	parts := strings.SplitN(name, "/", 2)
	providerID := parts[0]
	modelID := name
	if len(parts) == 2 {
		modelID = parts[1]
	}

	cfg := map[string]any{
		"$schema": "https://opencode.ai/config.json",
		"provider": map[string]any{
			providerID: map[string]any{
				"npm":  "@ai-sdk/openai-compatible",
				"name": "Bench provider",
				"options": map[string]any{
					"baseURL": baseURL,
					"apiKey":  apiKey,
				},
				"models": map[string]any{
					modelID: map[string]any{
						"name": modelID,
						"limit": map[string]any{
							// Defaults suit a local 32K model; override per model via
							// env (e.g. DeepSeek's large window) to avoid OpenCode
							// auto-compacting far below the model's real limit.
							"context": envInt("MODEL_CONTEXT", 32768),
							"output":  envInt("MODEL_MAX_TOKENS", 8192),
						},
					},
				},
			},
		},
		"mcp":          mcpServers(mode),
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

// writeAuthConfig writes auth.json into dataDir (OpenCode's data dir,
// $XDG_DATA_HOME/opencode) with a credential for the bench provider.
func writeAuthConfig(dataDir string) error {
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return fmt.Errorf("mkdir auth dir: %w", err)
	}

	apiKey := os.Getenv("MODEL_API_KEY")
	if apiKey == "" {
		apiKey = "not-needed"
	}
	parts := strings.SplitN(modelName(), "/", 2)
	providerID := parts[0]

	auth := map[string]any{
		providerID: map[string]any{
			"type": "api",
			"key":  apiKey,
		},
	}
	data, err := json.MarshalIndent(auth, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal auth: %w", err)
	}
	return os.WriteFile(filepath.Join(dataDir, "auth.json"), data, 0o644)
}

// setupOzy makes ozy mode self-contained instead of a manual pre-step: it
// writes an ozy config listing the same fixture MCP servers direct mode uses,
// then runs `ozy index` to populate a per-run catalog. It sets OZY_CONFIG and
// OZY_CATALOG (absolute) so both the index here and the `ozy mcp` broker that
// OpenCode launches read the same catalog. Without it, `ozy mcp` would serve
// the user's default, unindexed config and advertise nothing.
func setupOzy(ctx context.Context, fixtureDir, dir string) error {
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
	// under the "mcp" key ozy's config loader expects.
	cfg := map[string]any{"mcp": mcpServers("direct")}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal ozy config: %w", err)
	}
	if err := os.WriteFile(cfgPath, data, 0o644); err != nil {
		return fmt.Errorf("write ozy config: %w", err)
	}

	os.Setenv("OZY_CONFIG", cfgPath)
	os.Setenv("OZY_CATALOG", catalogPath)

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

// mcpServers returns the MCP server map for the given mode: direct exposes all
// seven fixture servers, ozy exposes only the ozy broker. Each fixture server
// is a stdio child process (ozy-bench mcp --toolset X) over the fixture.
func mcpServers(mode string) map[string]any {
	benchBin := os.Getenv("OZY_BENCH_BIN")
	if benchBin == "" {
		benchBin, _ = filepath.Abs("ozy-bench")
	}
	fixtureDir := os.Getenv("OZY_BENCH_FIXTURE_DIR")
	if fixtureDir == "" {
		fixtureDir = "/tmp/ozy-bench-fixture"
	}

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
		ozyBin := os.Getenv("OZY_BIN")
		if ozyBin == "" {
			ozyBin = "ozy"
		}
		return map[string]any{
			"ozy": map[string]any{
				"type":        "local",
				"command":     []string{ozyBin, "mcp"},
				"cwd":         fixtureDir,
				"environment": env,
				"enabled":     true,
			},
		}
	}

	server := func(args ...string) map[string]any {
		return map[string]any{"type": "local", "command": append([]string{benchBin, "mcp"}, args...), "enabled": true}
	}
	return map[string]any{
		"code-search": server("--toolset", "code-search", "--fixture-dir", fixtureDir),
		"git":         server("--toolset", "git", "--fixture-dir", fixtureDir),
		"incident-db": server("--toolset", "incident-db", "--fixture-dir", fixtureDir),
		"filesystem":  server("--toolset", "filesystem", "--fixture-dir", fixtureDir),
		"time":        server("--toolset", "time"),
		"memory":      server("--toolset", "memory"),
		"notes":       server("--toolset", "notes"),
	}
}

// modelName returns the model identifier from MODEL_NAME env or a default.
func modelName() string {
	if n := os.Getenv("MODEL_NAME"); n != "" {
		return n
	}
	return "unsloth/gemma-4-E2B-it-GGUF"
}

// envInt reads an int from env var name, falling back to def when unset or unparseable.
func envInt(name string, def int) int {
	if v := os.Getenv(name); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}
