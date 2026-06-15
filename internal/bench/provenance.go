package bench

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

// EnvironmentRecord captures the model and runtime provenance for a benchmark run.
type EnvironmentRecord struct {
	ModelName      string `json:"modelName"`
	ModelBaseURL   string `json:"modelBaseURL"`
	Temperature    string `json:"temperature"`
	MaxTokens      int    `json:"maxTokens"`
	ContextWindow  int    `json:"contextWindow"`
	Timestamp      string `json:"timestamp"`
	OzyGitSHA      string `json:"ozyGitSHA"`
	ScenarioHash   string `json:"scenarioHash"`
	Modes          []string `json:"modes"`
	RunCount       int      `json:"runCount"`
}

// SanitizeBaseURL strips path and query components, keeping only scheme://host[:port].
func SanitizeBaseURL(raw string) string {
	// Remove everything after the host:port
	re := regexp.MustCompile(`^(https?://[^/]+)`)
	m := re.FindStringSubmatch(raw)
	if len(m) > 1 {
		return m[1]
	}
	// Fallback: mask everything if we can't parse it.
	return "****"
}

// BuildProvenance collects the model and runtime metadata for the run.
func BuildProvenance(cfg *ScenarioConfig, modes []string, runCount int) (*EnvironmentRecord, error) {
	baseURL := os.Getenv(cfg.Model.BaseURLEnv)
	record := &EnvironmentRecord{
		ModelName:     os.Getenv(cfg.Model.NameEnv),
		ModelBaseURL:  SanitizeBaseURL(baseURL),
		Temperature:   cfg.Model.Temperature,
		MaxTokens:     cfg.Model.MaxTokens,
		ContextWindow: cfg.Model.ContextWindow,
		Timestamp:     time.Now().UTC().Format(time.RFC3339),
		OzyGitSHA:     resolveGitSHA(),
		Modes:         modes,
		RunCount:      runCount,
	}

	var err error
	record.ScenarioHash, err = cfg.ScenarioHash()
	if err != nil {
		return nil, fmt.Errorf("compute scenario hash: %w", err)
	}

	return record, nil
}

// WriteProvenance writes the environment record as JSON to the given path.
func WriteProvenance(path string, record *EnvironmentRecord) error {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("write provenance: %w", err)
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(record); err != nil {
		return fmt.Errorf("encode provenance: %w", err)
	}
	return nil
}

// resolveGitSHA returns the current git commit SHA for the ozy repository.
func resolveGitSHA() string {
	cmd := exec.Command("git", "rev-parse", "HEAD")
	out, err := cmd.Output()
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(out))
}

// ComputeFixtureHash returns a SHA-256 hash of the fixture directory contents,
// used to detect fixture changes across runs.
func ComputeFixtureHash(dir string) (string, error) {
	// Walk the fixture directory and hash file contents.
	h := sha256.New()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", fmt.Errorf("read fixture dir: %w", err)
	}

	var walk func(path string) error
	walk = func(path string) error {
		entries, err := os.ReadDir(path)
		if err != nil {
			return err
		}
		for _, e := range entries {
			fullPath := path + "/" + e.Name()
			if e.IsDir() {
				if e.Name() == ".git" {
					continue
				}
				if err := walk(fullPath); err != nil {
					return err
				}
				continue
			}
			rel, _ := strings.CutPrefix(fullPath, dir+"/")
			fmt.Fprintf(h, "F:%s\n", rel)
			content, err := os.ReadFile(fullPath)
			if err != nil {
				return fmt.Errorf("read %s: %w", rel, err)
			}
			h.Write(content)
		}
		return nil
	}

	for _, e := range entries {
		if e.Name() == ".git" {
			continue
		}
		if e.IsDir() {
			if err := walk(dir + "/" + e.Name()); err != nil {
				return "", err
			}
		}
	}

	return fmt.Sprintf("fixture:%x", h.Sum(nil)), nil
}
