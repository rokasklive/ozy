package bench

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// GroundTruth defines the expected answer and forbidden behaviors for a scenario.
type GroundTruth struct {
	RootCauseFile        string   `json:"root_cause_file"`
	RootCauseFunction    string   `json:"root_cause_function"`
	CulpritCommitSubject string   `json:"culprit_commit_subject"`
	ExpectedTest         string   `json:"expected_test"`
	ExpectedPatchFile    string   `json:"expected_patch_file"`
	ForbiddenBehaviors   []string `json:"forbidden_behaviors"`
}

// GradingResult captures per-criterion pass/fail and overall verdict.
type GradingResult struct {
	Overall         bool              `json:"overall"`
	Criteria        []CriterionResult `json:"criteria"`
	ForbiddenChecks []CriterionResult `json:"forbidden_checks"`
}

// CriterionResult is a single pass/fail check with a description.
type CriterionResult struct {
	Name   string `json:"name"`
	Pass   bool   `json:"pass"`
	Detail string `json:"detail"`
}

// ToolCallLog records a tool invocation during the agent run.
type ToolCallLog struct {
	Tool   string `json:"tool"`
	Server string `json:"server,omitempty"`
}

// LoadGroundTruth reads ground_truth.json from path.
//
//nolint:gosec // G304: path comes from a trusted scenario config, not user input.
func LoadGroundTruth(path string) (*GroundTruth, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read ground truth: %w", err)
	}
	var gt GroundTruth
	if err := json.Unmarshal(data, &gt); err != nil {
		return nil, fmt.Errorf("unmarshal ground truth: %w", err)
	}
	return &gt, nil
}

// Grade scores the final answer and tool-call log against ground truth.
// It returns a deterministic pass/fail — no model is used.
func Grade(gt *GroundTruth, finalAnswer string, toolCalls []ToolCallLog, culpritHash string) *GradingResult {
	result := &GradingResult{Overall: true}

	// Core criteria: check the final answer contains expected values.
	core := []CriterionResult{
		checkContains("root_cause_file", finalAnswer, gt.RootCauseFile, "final answer must name the correct source file"),
		checkContains("root_cause_function", finalAnswer, gt.RootCauseFunction, "final answer must name the correct function"),
		checkCommitMatch("culprit_commit", finalAnswer, gt.CulpritCommitSubject, culpritHash, "final answer must identify the correct git commit"),
		checkContains("expected_test", finalAnswer, gt.ExpectedTest, "final answer must name the expected regression test"),
		checkContains("expected_patch_file", finalAnswer, gt.ExpectedPatchFile, "final answer must identify the correct patch target file"),
	}
	result.Criteria = core

	// Forbidden checks.
	var forbidden []CriterionResult
	forbidden = append(forbidden, checkNoDistractorTools("no_distractor_tools", toolCalls))
	forbidden = append(forbidden, checkNoWebTools("no_web_tools", toolCalls))
	forbidden = append(forbidden, checkNoBroadRefactor("no_broad_refactor", finalAnswer))
	result.ForbiddenChecks = forbidden

	// Overall: all criteria + all forbidden checks must pass.
	for _, c := range core {
		if !c.Pass {
			result.Overall = false
		}
	}
	for _, c := range forbidden {
		if !c.Pass {
			result.Overall = false
		}
	}

	return result
}

// WriteGradingResult writes the grading result as JSON to path.
//
//nolint:gosec // G304: path is a controlled artifact path in bench output.
func WriteGradingResult(path string, result *GradingResult) error {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("write grading result: %w", err)
	}
	defer func() { _ = f.Close() }()

	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(result); err != nil {
		return fmt.Errorf("encode grading result: %w", err)
	}
	return nil
}

// checkContains checks if needle appears (case-insensitive) in haystack.
func checkContains(name, haystack, needle, detail string) CriterionResult {
	lower := strings.ToLower(haystack)
	if strings.Contains(lower, strings.ToLower(needle)) {
		return CriterionResult{Name: name, Pass: true, Detail: detail}
	}
	return CriterionResult{Name: name, Pass: false, Detail: fmt.Sprintf("%s (expected %q not found)", detail, needle)}
}

// checkCommitMatch checks if the answer contains the commit subject or hash.
func checkCommitMatch(name, haystack, subject, hash, detail string) CriterionResult {
	lower := strings.ToLower(haystack)
	if strings.Contains(lower, strings.ToLower(subject)) {
		return CriterionResult{Name: name, Pass: true, Detail: detail}
	}
	if len(hash) >= 8 && strings.Contains(lower, strings.ToLower(hash[:8])) {
		return CriterionResult{Name: name, Pass: true, Detail: detail + " (matched by hash prefix)"}
	}
	return CriterionResult{Name: name, Pass: false, Detail: fmt.Sprintf("%s (expected subject %q or hash %q not found)", detail, subject, hash)}
}

// checkNoDistractorTools checks if any distractor tool was called.
func checkNoDistractorTools(name string, calls []ToolCallLog) CriterionResult {
	distractorTools := map[string]bool{
		"current_time":     true,
		"convert_timezone": true,
		"search_memory":    true,
		"store_memory":     true,
		"create_plan":      true,
		"append_note":      true,
	}
	for _, call := range calls {
		// Tool names are namespaced by MCP server (e.g. "time_current_time"),
		// so match the bare name or any "<server>_<name>" suffix.
		for d := range distractorTools {
			if call.Tool == d || strings.HasSuffix(call.Tool, "_"+d) {
				return CriterionResult{Name: name, Pass: false, Detail: fmt.Sprintf("distractor tool called: %s", call.Tool)}
			}
		}
	}
	return CriterionResult{Name: name, Pass: true, Detail: "no distractor tools were called"}
}

// checkNoWebTools checks if any web/internet tool was called.
func checkNoWebTools(name string, calls []ToolCallLog) CriterionResult {
	webPatterns := []string{"web_", "http_", "browser_", "url_", "fetch", "brave_"}
	for _, call := range calls {
		lower := strings.ToLower(call.Tool)
		for _, pattern := range webPatterns {
			if strings.HasPrefix(lower, pattern) || strings.Contains(lower, pattern) {
				return CriterionResult{Name: name, Pass: false, Detail: fmt.Sprintf("web-class tool called: %s", call.Tool)}
			}
		}
	}
	return CriterionResult{Name: name, Pass: true, Detail: "no web-class tools were called"}
}

// checkNoBroadRefactor checks if the answer suggests a broad/system-wide refactor.
func checkNoBroadRefactor(name, answer string) CriterionResult {
	lower := strings.ToLower(answer)
	broadTerms := []string{"architecture redesign", "rewrite the system", "system-wide refactor", "restructure the codebase"}
	for _, term := range broadTerms {
		if strings.Contains(lower, term) {
			return CriterionResult{Name: name, Pass: false, Detail: fmt.Sprintf("broad refactor detected: %q", term)}
		}
	}
	return CriterionResult{Name: name, Pass: true, Detail: "no broad refactor suggested"}
}
