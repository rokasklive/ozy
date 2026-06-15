package bench

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGradeCorrectAnswerPasses(t *testing.T) {
	t.Parallel()

	gt := &GroundTruth{
		RootCauseFile:        "src/main/java/com/acme/billing/StatusMapper.java",
		RootCauseFunction:    "fromString",
		CulpritCommitSubject: "Normalize account status mapping",
		ExpectedTest:         "suspendedAccountsNotInvoiced",
		ExpectedPatchFile:    "src/main/java/com/acme/billing/StatusMapper.java",
		ForbiddenBehaviors:   []string{"public_web_search"},
	}

	finalAnswer := `
Root cause: The StatusMapper.fromString method incorrectly maps SUSPENDED to ACTIVE.

Source file: src/main/java/com/acme/billing/StatusMapper.java
Function: fromString
Culprit commit: Normalize account status mapping
Patch target: src/main/java/com/acme/billing/StatusMapper.java
Regression test: suspendedAccountsNotInvoiced
`
	toolCalls := []ToolCallLog{
		{Tool: "search_text", Server: "code-search"},
		{Tool: "read_file", Server: "code-search"},
		{Tool: "git_show", Server: "git"},
		{Tool: "query_readonly", Server: "incident-db"},
	}

	result := Grade(gt, finalAnswer, toolCalls, "abc123def456")

	if !result.Overall {
		t.Errorf("correct answer should pass, got overall=%v", result.Overall)
		for _, c := range result.Criteria {
			if !c.Pass {
				t.Errorf("  criterion %s: %s", c.Name, c.Detail)
			}
		}
		for _, c := range result.ForbiddenChecks {
			if !c.Pass {
				t.Errorf("  forbidden check %s: %s", c.Name, c.Detail)
			}
		}
	}
}

func TestGradeDistractorCallFails(t *testing.T) {
	t.Parallel()

	gt := &GroundTruth{
		RootCauseFile:        "StatusMapper.java",
		RootCauseFunction:    "fromString",
		CulpritCommitSubject: "Normalize account status mapping",
		ExpectedTest:         "suspendedAccountsNotInvoiced",
		ExpectedPatchFile:    "StatusMapper.java",
	}

	finalAnswer := "fixed StatusMapper.java fromString - commit: Normalize account status mapping test: suspendedAccountsNotInvoiced patch: StatusMapper.java"
	toolCalls := []ToolCallLog{
		{Tool: "current_time", Server: "time"},
		{Tool: "store_memory", Server: "memory"},
		{Tool: "search_text", Server: "code-search"},
	}

	result := Grade(gt, finalAnswer, toolCalls, "abc123")

	if result.Overall {
		t.Error("run with distractor calls should fail")
	}
}

func TestGradeWebToolCallFails(t *testing.T) {
	t.Parallel()

	gt := &GroundTruth{
		RootCauseFile:        "StatusMapper.java",
		RootCauseFunction:    "fromString",
		CulpritCommitSubject: "Normalize account status mapping",
		ExpectedTest:         "suspendedAccountsNotInvoiced",
		ExpectedPatchFile:    "StatusMapper.java",
	}

	finalAnswer := "fixed StatusMapper.java fromString commit: Normalize account status mapping test: suspendedAccountsNotInvoiced patch: StatusMapper.java"
	toolCalls := []ToolCallLog{
		{Tool: "web_search", Server: "web"},
		{Tool: "search_text", Server: "code-search"},
	}

	result := Grade(gt, finalAnswer, toolCalls, "abc123")

	if result.Overall {
		t.Error("run with web tool call should fail")
	}
}

func TestGradeCommitByHash(t *testing.T) {
	t.Parallel()

	gt := &GroundTruth{
		RootCauseFile:        "StatusMapper.java",
		RootCauseFunction:    "fromString",
		CulpritCommitSubject: "Normalize account status mapping",
		ExpectedTest:         "suspendedAccountsNotInvoiced",
		ExpectedPatchFile:    "StatusMapper.java",
	}

	finalAnswer := "commit abc12345 fixed StatusMapper.java fromString. Test: suspendedAccountsNotInvoiced. Patch: StatusMapper.java"
	toolCalls := []ToolCallLog{}

	result := Grade(gt, finalAnswer, toolCalls, "abc1234567890abcdef")

	if !result.Overall {
		t.Error("commit matched by hash prefix should pass")
	}
}

func TestLoadAndGradeRoundTrip(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	gtPath := filepath.Join(dir, "ground_truth.json")
	content := `{
		"root_cause_file": "StatusMapper.java",
		"root_cause_function": "fromString",
		"culprit_commit_subject": "Normalize account status mapping",
		"expected_test": "suspendedAccountsNotInvoiced",
		"expected_patch_file": "StatusMapper.java",
		"forbidden_behaviors": ["public_web_search", "architecture_redesign"]
	}`
	if err := os.WriteFile(gtPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write ground truth: %v", err)
	}

	gt, err := LoadGroundTruth(gtPath)
	if err != nil {
		t.Fatalf("LoadGroundTruth: %v", err)
	}

	if gt.RootCauseFunction != "fromString" {
		t.Errorf("root cause function = %q, want fromString", gt.RootCauseFunction)
	}

	finalAnswer := "StatusMapper.java fromString Normalize account status mapping suspendedAccountsNotInvoiced StatusMapper.java"
	result := Grade(gt, finalAnswer, nil, "")

	gradingPath := filepath.Join(dir, "grading.json")
	if err := WriteGradingResult(gradingPath, result); err != nil {
		t.Fatalf("WriteGradingResult: %v", err)
	}

	if !result.Overall {
		t.Error("correct answer from loaded ground truth should pass")
	}
}
