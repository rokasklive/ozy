package bench

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/tailscale/hujson"
)

// ScenarioConfig is the typed JSONC configuration for a benchmark scenario.
type ScenarioConfig struct {
	Name     string `json:"name"`
	TaskFile string `json:"taskFile"`
	Fixture  string `json:"fixture"`

	Model struct {
		NameEnv       string `json:"nameEnv"`
		BaseURLEnv    string `json:"baseURLEnv"`
		APIKeyEnv     string `json:"apiKeyEnv"`
		Temperature   string `json:"temperature"`
		MaxTokens     int    `json:"maxTokens"`
		ContextWindow int    `json:"contextWindow"`
	} `json:"model"`

	AgentConfigs struct {
		Direct string `json:"direct"`
		Ozy    string `json:"ozy"`
	} `json:"agentConfigs"`

	Limits struct {
		Runs           int `json:"runs"`
		TimeoutSeconds int `json:"timeoutSeconds"`
	} `json:"limits"`

	GroundTruth string `json:"groundTruth"`

	ForbiddenTools     []string `json:"forbiddenTools"`
	ForbiddenBehaviors []string `json:"forbiddenBehaviors"`

	// BaseDir is the directory containing the scenario config file.
	// All relative paths (TaskFile, Fixture, GroundTruth) are resolved
	// relative to this directory.
	BaseDir string `json:"-"`
}

// ResolvePath resolves a relative path against the scenario's base directory.
func (cfg *ScenarioConfig) ResolvePath(rel string) string {
	if filepath.IsAbs(rel) || cfg.BaseDir == "" {
		return rel
	}
	return filepath.Join(cfg.BaseDir, rel)
}

// scenarioConfigJSON is the raw JSON form used to resolve env references
// during unmarshaling.
type scenarioConfigJSON struct {
	Name               string          `json:"name"`
	TaskFile           string          `json:"taskFile"`
	Fixture            string          `json:"fixture"`
	Model              json.RawMessage `json:"model"`
	AgentConfigs       json.RawMessage `json:"agentConfigs"`
	Limits             json.RawMessage `json:"limits"`
	GroundTruth        string          `json:"groundTruth"`
	ForbiddenTools     []string        `json:"forbiddenTools"`
	ForbiddenBehaviors []string        `json:"forbiddenBehaviors"`
}

// LoadScenario reads, parses, and resolves a JSONC scenario configuration
// from path. It substitutes {env:VAR} references using os.LookupEnv.
func LoadScenario(path string) (*ScenarioConfig, error) {
	//nolint:gosec // G304: path comes from a trusted CLI flag, not user input.
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read scenario config: %w", err)
	}

	// Resolve {env:VAR} references before standardizing JSONC.
	resolved := resolveEnvRefs(string(data))

	standard, err := hujson.Standardize([]byte(resolved))
	if err != nil {
		return nil, fmt.Errorf("invalid JSONC in %s: %w", path, err)
	}

	var raw scenarioConfigJSON
	if err := json.Unmarshal(standard, &raw); err != nil {
		return nil, fmt.Errorf("unmarshal scenario config: %w", err)
	}

	cfg := &ScenarioConfig{
		Name:               raw.Name,
		TaskFile:           raw.TaskFile,
		Fixture:            raw.Fixture,
		GroundTruth:        raw.GroundTruth,
		ForbiddenTools:     raw.ForbiddenTools,
		ForbiddenBehaviors: raw.ForbiddenBehaviors,
		BaseDir:            filepath.Dir(path),
	}

	applyDefaults(cfg)

	return cfg, nil
}

// applyDefaults fills in the documented defaults for omitted fields.
func applyDefaults(cfg *ScenarioConfig) {
	if cfg.Limits.Runs == 0 {
		cfg.Limits.Runs = 5
	}
	if cfg.Limits.TimeoutSeconds == 0 {
		cfg.Limits.TimeoutSeconds = 600
	}
}

// ResolveRunCount returns the effective run count: CLI flag, then BENCH_RUNS
// env, then scenario config default.
func ResolveRunCount(cliRuns int, cfg *ScenarioConfig) int {
	if cliRuns > 0 {
		return cliRuns
	}
	if s := os.Getenv("BENCH_RUNS"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			return n
		}
	}
	return cfg.Limits.Runs
}

// resolveEnvRefs substitutes {env:NAME} references in the raw JSONC text with
// their resolved environment variable values.
func resolveEnvRefs(raw string) string {
	var result strings.Builder
	i := 0
	for i < len(raw) {
		start := strings.Index(raw[i:], "{env:")
		if start == -1 {
			result.WriteString(raw[i:])
			break
		}
		start += i
		result.WriteString(raw[i:start])

		// Find the closing }
		end := strings.IndexByte(raw[start:], '}')
		if end == -1 {
			result.WriteString(raw[start:])
			break
		}
		end += start

		// Extract the variable name: {env:NAME} → NAME
		ref := raw[start+1 : end] // skip { and }
		name := ref
		if strings.HasPrefix(ref, "env:") {
			name = ref[4:]
		}

		if val, ok := os.LookupEnv(name); ok {
			result.WriteString(val)
		} else {
			// Keep unresolved reference so the caller can detect it.
			result.WriteString(raw[start : end+1])
		}
		i = end + 1
	}
	return result.String()
}

// ScenarioHash computes a deterministic SHA-256 hash of the scenario's
// definition. The hash incorporates the task prompt content so it changes
// when the scenario definition or prompt changes.
func (cfg *ScenarioConfig) ScenarioHash() (string, error) {
	h := sha256.New()
	fmt.Fprintf(h, "name=%s\n", cfg.Name)
	fmt.Fprintf(h, "taskFile=%s\n", cfg.TaskFile)
	fmt.Fprintf(h, "fixture=%s\n", cfg.Fixture)
	fmt.Fprintf(h, "groundTruth=%s\n", cfg.GroundTruth)

	// Incorporate the task prompt content.
	taskPath := cfg.ResolvePath(cfg.TaskFile)
	//nolint:gosec // G304: taskPath is resolved from a trusted scenario config.
	content, err := os.ReadFile(taskPath)
	if err != nil {
		return "", fmt.Errorf("read task file for hash: %w", err)
	}
	h.Write(content)

	return fmt.Sprintf("%x", h.Sum(nil)), nil
}
