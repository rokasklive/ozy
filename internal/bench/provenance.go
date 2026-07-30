package bench

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// EnvironmentRecord captures the model, runtime, and tooling provenance for a
// benchmark run. It holds no credential of any kind: the live tier drives
// OpenCode's built-in models, so there is no key, token, or endpoint to record.
type EnvironmentRecord struct {
	ModelID         string   `json:"modelId"`
	Timestamp       string   `json:"timestamp"`
	OzyGitDescribe  string   `json:"ozyGitDescribe"`
	OzyGitSHA       string   `json:"ozyGitSHA"`
	OpenCodeVersion string   `json:"openCodeVersion"`
	TokenEstimator  string   `json:"tokenEstimator"`
	UsageSource     string   `json:"usageSource"` // measured | estimated | mixed | skipped
	Retrieval       string   `json:"retrieval"`   // semantic | lexical (what ozy actually exercised)
	ScenarioHash    string   `json:"scenarioHash"`
	Modes           []string `json:"modes"`
	RunCount        int      `json:"runCount"`
}

// BuildProvenance collects the model, runtime, and tooling metadata for the run.
// estimator names the token estimator; usageSource records whether token
// accounting was agent-reported (measured), estimated, mixed, or skipped.
func BuildProvenance(cfg *ScenarioConfig, modes []string, runCount int, estimator, usageSource string) (*EnvironmentRecord, error) {
	record := &EnvironmentRecord{
		ModelID:         benchModel(),
		Timestamp:       time.Now().UTC().Format(time.RFC3339),
		OzyGitDescribe:  resolveGitDescribe(),
		OzyGitSHA:       resolveGitSHA(),
		OpenCodeVersion: resolveOpenCodeVersion(),
		TokenEstimator:  estimator,
		UsageSource:     usageSource,
		Retrieval:       resolveRetrievalStack(),
		Modes:           modes,
		RunCount:        runCount,
	}

	var err error
	record.ScenarioHash, err = cfg.ScenarioHash()
	if err != nil {
		return nil, fmt.Errorf("compute scenario hash: %w", err)
	}

	return record, nil
}

// resolveRetrievalStack reports which retrieval stack ozy exercised. The value
// is pinned at image build time (task 1.2 verifies it in the hermetic image and
// sets OZY_BENCH_RETRIEVAL); it is never assumed. "unknown" until verified.
func resolveRetrievalStack() string {
	if v := strings.TrimSpace(os.Getenv("OZY_BENCH_RETRIEVAL")); v != "" {
		return v
	}
	return "unknown"
}

// resolveGitDescribe returns a human-readable ozy version (tag+distance+sha),
// falling back to the short SHA and then "unknown".
func resolveGitDescribe() string {
	out, err := exec.Command("git", "describe", "--tags", "--always", "--dirty").Output()
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(out))
}

// resolveOpenCodeVersion reads the pinned OpenCode version, preferring the
// build-time OPENCODE_VERSION baked into the image, then `opencode --version`.
func resolveOpenCodeVersion() string {
	if v := os.Getenv("OPENCODE_VERSION"); v != "" {
		return strings.TrimSpace(v)
	}
	openCode := os.Getenv("OPENCODE_PATH")
	if openCode == "" {
		openCode = "opencode"
	}
	//nolint:gosec // G204: opencode is the pinned agent binary in the bench image.
	out, err := exec.Command(openCode, "--version").Output()
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(out))
}

// WriteProvenance writes the environment record as JSON to the given path.
//
//nolint:gosec // G304: path is a controlled artifact path in bench output.
func WriteProvenance(path string, record *EnvironmentRecord) error {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("write provenance: %w", err)
	}
	defer func() { _ = f.Close() }()

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
//
//nolint:gosec // G304: dir is a controlled fixture path, not user input.
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
