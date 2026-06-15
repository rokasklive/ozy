package bench

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadScenarioDefaults(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "scenario.jsonc")
	content := `{
		"name": "test-scenario",
		"taskFile": "task.md",
		"fixture": ".",
		"model": {
			"nameEnv": "MODEL_NAME",
			"baseURLEnv": "MODEL_BASE_URL",
			"apiKeyEnv": "MODEL_API_KEY",
			"temperature": "0.0",
			"maxTokens": 4096,
			"contextWindow": 32768
		},
		"agentConfigs": {
			"direct": "direct.jsonc",
			"ozy": "ozy.jsonc"
		},
		"limits": {},
		"groundTruth": "expected/ground_truth.json",
		"forbiddenTools": [],
		"forbiddenBehaviors": ["public_web_search", "architecture_redesign"]
	}`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := LoadScenario(path)
	if err != nil {
		t.Fatalf("LoadScenario: %v", err)
	}

	if cfg.Limits.Runs != 5 {
		t.Errorf("default runs = %d, want 5", cfg.Limits.Runs)
	}
	if cfg.Limits.TimeoutSeconds != 600 {
		t.Errorf("default timeout = %d, want 600", cfg.Limits.TimeoutSeconds)
	}
}

func TestLoadScenarioEnvSubstitution(t *testing.T) {
	// Not parallel — uses t.Setenv.

	t.Setenv("MY_NAME", "test-model")
	t.Setenv("MY_URL", "http://localhost:8080/v1")

	dir := t.TempDir()
	path := filepath.Join(dir, "scenario.jsonc")
	content := `{
		"name": "test",
		"taskFile": "task.md",
		"fixture": ".",
		"model": {
			"nameEnv": "MY_NAME",
			"baseURLEnv": "MY_URL",
			"apiKeyEnv": "MY_KEY",
			"temperature": "0.0",
			"maxTokens": 4096,
			"contextWindow": 32768
		},
		"agentConfigs": {
			"direct": "d.jsonc",
			"ozy": "o.jsonc"
		},
		"limits": { "runs": 3, "timeoutSeconds": 300 },
		"groundTruth": "gt.json",
		"forbiddenTools": [],
		"forbiddenBehaviors": []
	}`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	_, err := LoadScenario(path)
	if err != nil {
		t.Fatalf("LoadScenario: %v", err)
	}
	// LoadScenario should not fail — env var substitution happens via
	// os.LookupEnv during the live tier, not during loading.
}

func TestSanitizeBaseURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		raw  string
		want string
	}{
		{"http://localhost:8080/v1/chat", "http://localhost:8080"},
		{"https://api.openai.com/v1", "https://api.openai.com"},
		{"http://host.docker.internal:1234", "http://host.docker.internal:1234"},
		{"invalid-url", "****"},
		{"", "****"},
	}

	for _, tt := range tests {
		got := SanitizeBaseURL(tt.raw)
		if got != tt.want {
			t.Errorf("SanitizeBaseURL(%q) = %q, want %q", tt.raw, got, tt.want)
		}
	}
}

func TestResolveRunCount(t *testing.T) {
	// Not parallel — uses t.Setenv.

	cfg := &ScenarioConfig{}
	cfg.Limits.Runs = 5

	// Default from config.
	if got := ResolveRunCount(0, cfg); got != 5 {
		t.Errorf("default runs = %d, want 5", got)
	}

	// CLI override.
	if got := ResolveRunCount(10, cfg); got != 10 {
		t.Errorf("cli runs = %d, want 10", got)
	}

	// Env override.
	t.Setenv("BENCH_RUNS", "7")
	if got := ResolveRunCount(0, cfg); got != 7 {
		t.Errorf("env runs = %d, want 7", got)
	}
}
