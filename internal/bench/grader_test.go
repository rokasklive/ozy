package bench

import (
	"os"
	"path/filepath"
	"testing"
)

// acmeGroundTruth is the declarative migration of the acme-billing scenario's
// former hardcoded criteria — used to prove 1:1 grading outcomes.
func acmeGroundTruth() *GroundTruth {
	return &GroundTruth{
		AnswerMustContain: []string{
			"src/main/java/com/acme/billing/StatusMapper.java",
			"fromString",
			"suspendedAccountsNotInvoiced",
		},
		CommitCheck: &CommitCheck{Subject: "Normalize account status mapping", HashFrom: "fixture-meta"},
		RequiredTools: []RequiredTool{
			{Server: "code-search", Tool: "search_text"},
		},
		ForbiddenToolPatterns: []string{
			"current_time", "convert_timezone", "search_memory", "store_memory",
			"create_plan", "append_note",
			"web_", "http_", "browser_", "url_", "fetch", "brave_",
		},
		ForbiddenAnswerPatterns: []string{
			"architecture redesign", "rewrite the system", "system-wide refactor", "restructure the codebase",
		},
	}
}

func TestGradeCorrectAnswerPasses(t *testing.T) {
	t.Parallel()

	finalAnswer := `
Root cause: The StatusMapper.fromString method incorrectly maps SUSPENDED to ACTIVE.

Source file: src/main/java/com/acme/billing/StatusMapper.java
Function: fromString
Culprit commit: Normalize account status mapping
Regression test: suspendedAccountsNotInvoiced
`
	calls := []ToolCallLog{
		{Tool: "search_text", Server: "code-search"},
		{Tool: "read_file", Server: "code-search"},
		{Tool: "git_show", Server: "git"},
	}

	result := Grade(acmeGroundTruth(), GradeInput{FinalAnswer: finalAnswer, ToolCalls: calls, CulpritHash: "abc123def456"})

	if !result.Overall {
		t.Errorf("correct answer should pass, got overall=%v", result.Overall)
		for _, c := range append(result.Criteria, result.Informational...) {
			if !c.Pass {
				t.Errorf("  %s: %s", c.Name, c.Detail)
			}
		}
	}
}

// Under outcome grading, a run that satisfies the result criteria PASSES even
// when it also calls a distractor / "forbidden" tool — tool attribution is
// informational, not gating (D1).
func TestGradeDistractorCallStillPasses(t *testing.T) {
	t.Parallel()

	finalAnswer := "src/main/java/com/acme/billing/StatusMapper.java fromString Normalize account status mapping suspendedAccountsNotInvoiced"
	calls := []ToolCallLog{
		{Tool: "current_time", Server: "time"},
		{Tool: "store_memory", Server: "memory"},
		{Tool: "search_text", Server: "code-search"},
	}

	result := Grade(acmeGroundTruth(), GradeInput{FinalAnswer: finalAnswer, ToolCalls: calls, CulpritHash: "abc123"})
	if !result.Overall {
		t.Error("result criteria satisfied — run should pass despite distractor calls")
	}
	// The forbidden-tool hit is still recorded as informational.
	failedInfo := false
	for _, c := range result.Informational {
		if !c.Pass {
			failedInfo = true
		}
	}
	if !failedInfo {
		t.Error("distractor calls should be recorded as a failed informational check")
	}
}

// A non-canonical same-capability tool (a different search engine) that still
// produces the right result passes — which tool was called does not matter (D1).
func TestGradeAlternativeToolStillPasses(t *testing.T) {
	t.Parallel()

	finalAnswer := "src/main/java/com/acme/billing/StatusMapper.java fromString Normalize account status mapping suspendedAccountsNotInvoiced"
	calls := []ToolCallLog{
		{Tool: "web_search", Server: "web"},
		{Tool: "search_text", Server: "code-search"},
	}

	result := Grade(acmeGroundTruth(), GradeInput{FinalAnswer: finalAnswer, ToolCalls: calls, CulpritHash: "abc123"})
	if !result.Overall {
		t.Error("run completing the task via an alternative tool should pass")
	}
}

func TestGradeCommitByHash(t *testing.T) {
	t.Parallel()

	finalAnswer := "commit abc12345 fixed src/main/java/com/acme/billing/StatusMapper.java fromString. Test: suspendedAccountsNotInvoiced"
	calls := []ToolCallLog{{Tool: "search_text", Server: "code-search"}}

	result := Grade(acmeGroundTruth(), GradeInput{FinalAnswer: finalAnswer, ToolCalls: calls, CulpritHash: "abc1234567890abcdef"})
	if !result.Overall {
		t.Error("commit matched by hash prefix should pass")
	}
}

// A run missing a result fact FAILS even when every canonical tool was called —
// the outcome, not tool attribution, gates success (D1).
func TestGradeMissingAnswerFactFails(t *testing.T) {
	t.Parallel()

	// The canonical tool ran, but the answer omits the regression-test name.
	finalAnswer := "src/main/java/com/acme/billing/StatusMapper.java fromString Normalize account status mapping"
	calls := []ToolCallLog{{Server: "code-search", Tool: "search_text"}}
	result := Grade(acmeGroundTruth(), GradeInput{FinalAnswer: finalAnswer, ToolCalls: calls, CulpritHash: "abc123"})
	if result.Overall {
		t.Error("answer missing a required result fact should fail")
	}
}

// Conversely, an answer that satisfies every result fact PASSES even when no
// tool call was logged at all — required_tools is informational (D1).
func TestGradeResultCriteriaAloneGate(t *testing.T) {
	t.Parallel()

	finalAnswer := "src/main/java/com/acme/billing/StatusMapper.java fromString Normalize account status mapping suspendedAccountsNotInvoiced"
	result := Grade(acmeGroundTruth(), GradeInput{FinalAnswer: finalAnswer, ToolCalls: nil, CulpritHash: "abc123"})
	if !result.Overall {
		t.Error("answer satisfying all result criteria should pass regardless of tool calls")
	}
}

func TestGradeArtifactPDF(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	pdfPath := filepath.Join(dir, "output", "report.pdf")
	if err := os.MkdirAll(filepath.Dir(pdfPath), 0o755); err != nil {
		t.Fatal(err)
	}
	pdf := BuildPDF(PDFMeta{Title: "Report"}, "The max temperature was 24.3 C in Vilnius")
	if err := os.WriteFile(pdfPath, pdf, 0o644); err != nil {
		t.Fatal(err)
	}

	gt := &GroundTruth{
		Artifacts: []ArtifactCheck{
			{Path: "output/report.pdf", Type: "pdf", MustContain: []string{"24.3 C", "Vilnius"}},
		},
	}
	if !Grade(gt, GradeInput{OutputDir: dir}).Overall {
		t.Error("PDF artifact containing the facts should pass")
	}

	// Missing fact fails.
	gt.Artifacts[0].MustContain = []string{"snowstorm"}
	if Grade(gt, GradeInput{OutputDir: dir}).Overall {
		t.Error("PDF artifact missing a required fact should fail")
	}

	// Missing file fails.
	gt2 := &GroundTruth{Artifacts: []ArtifactCheck{{Path: "output/missing.pdf", Type: "pdf"}}}
	if Grade(gt2, GradeInput{OutputDir: dir}).Overall {
		t.Error("missing artifact should fail")
	}
}

func TestLoadGroundTruthV2(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	gtPath := filepath.Join(dir, "ground_truth.json")
	content := `{
		"answer_must_contain": ["StatusMapper.java", "fromString"],
		"commit_check": {"subject": "Normalize account status mapping", "hashFrom": "fixture-meta"},
		"required_tools": [{"server": "code-search", "tool": "search_text"}],
		"forbidden_tool_patterns": ["web_", "fetch"],
		"forbidden_answer_patterns": ["architecture redesign"]
	}`
	if err := os.WriteFile(gtPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	gt, err := LoadGroundTruth(gtPath)
	if err != nil {
		t.Fatalf("LoadGroundTruth: %v", err)
	}
	if len(gt.AnswerMustContain) != 2 || gt.CommitCheck == nil || len(gt.RequiredTools) != 1 {
		t.Fatalf("ground truth did not parse: %+v", gt)
	}

	finalAnswer := "StatusMapper.java fromString Normalize account status mapping"
	calls := []ToolCallLog{{Server: "code-search", Tool: "search_text"}}
	if !Grade(gt, GradeInput{FinalAnswer: finalAnswer, ToolCalls: calls}).Overall {
		t.Error("correct answer from loaded ground truth should pass")
	}
}

func TestLoadCallLog(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	logPath := filepath.Join(dir, "calls.jsonl")
	lines := `{"server":"weather","tool":"get_historical_weather","argsDigest":"aa","ts":"t"}
{"server":"duckduckgo","tool":"search","argsDigest":"bb","ts":"t"}
`
	if err := os.WriteFile(logPath, []byte(lines), 0o644); err != nil {
		t.Fatal(err)
	}
	calls := LoadCallLog(logPath)
	if len(calls) != 2 || calls[0].Server != "weather" || calls[1].Tool != "search" {
		t.Fatalf("call log parse failed: %+v", calls)
	}
	// Missing log is empty, not an error.
	if LoadCallLog(filepath.Join(dir, "nope.jsonl")) != nil {
		t.Error("missing call log should be nil")
	}
}
