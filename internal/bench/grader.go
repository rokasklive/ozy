package bench

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// GroundTruth v2 is a declarative check specification the generic grader
// executes — no scenario-specific logic lives in Go. Each scenario expresses all
// of its criteria as this data.
type GroundTruth struct {
	// AnswerMustContain are substrings the final answer must include (case-insensitive).
	AnswerMustContain []string `json:"answer_must_contain"`
	// CommitCheck matches a culprit commit by subject or fixture-recorded hash.
	CommitCheck *CommitCheck `json:"commit_check,omitempty"`
	// RequiredTools are {server, tool} pairs recorded as informational telemetry
	// (which canonical tools the run engaged); they do NOT gate success (D1).
	RequiredTools []RequiredTool `json:"required_tools"`
	// ForbiddenToolPatterns are substrings recorded as informational telemetry
	// (which distractor tools the run touched); they do NOT gate success (D1).
	ForbiddenToolPatterns []string `json:"forbidden_tool_patterns"`
	// ForbiddenAnswerPatterns are substrings the final answer may not contain
	// (e.g. broad-refactor language). The home for the old refactor ban as data.
	ForbiddenAnswerPatterns []string `json:"forbidden_answer_patterns,omitempty"`
	// Artifacts are files the agent must have produced, with content checks.
	Artifacts []ArtifactCheck `json:"artifacts"`
}

// CommitCheck is the optional culprit-commit criterion. HashFrom is informational
// provenance (e.g. "fixture-meta"); the hash itself is supplied at grade time.
type CommitCheck struct {
	Subject  string `json:"subject"`
	HashFrom string `json:"hashFrom,omitempty"`
}

// RequiredTool is a {server, tool} pair the run must have invoked.
type RequiredTool struct {
	Server string `json:"server"`
	Tool   string `json:"tool"`
}

// ArtifactCheck asserts a produced file exists, is of a type, and contains facts.
// Type is "pdf" (parsed and text-extracted) or "text"/"file" (read as bytes).
type ArtifactCheck struct {
	Path        string   `json:"path"`
	Type        string   `json:"type"`
	MustContain []string `json:"must_contain"`
}

// GradingResult captures per-criterion pass/fail and overall verdict. Overall is
// determined by result criteria only (answer/commit/artifact/forbidden-answer);
// the tool-usage checks in Informational are recorded but never flip Overall.
type GradingResult struct {
	Overall  bool              `json:"overall"`
	Criteria []CriterionResult `json:"criteria"`
	// Informational holds required-tool / forbidden-tool results: telemetry about
	// which tools the run selected, recorded and reported but not gating (D1).
	Informational []CriterionResult `json:"informational"`
}

// CriterionResult is a single pass/fail check with a description.
type CriterionResult struct {
	Name   string `json:"name"`
	Pass   bool   `json:"pass"`
	Detail string `json:"detail"`
}

// ToolCallLog records a tool invocation. When loaded from the server-side
// invocation log it carries the downstream server and bare tool name.
type ToolCallLog struct {
	Tool   string `json:"tool"`
	Server string `json:"server,omitempty"`
}

// GradeInput is everything the generic grader scores against ground truth.
type GradeInput struct {
	FinalAnswer string
	// ToolCalls is the server-side invocation log (mode-symmetric ground truth
	// for tool selection), not the transcript's broker calls.
	ToolCalls   []ToolCallLog
	CulpritHash string
	// OutputDir is where artifact paths resolve (the per-run agent workspace).
	OutputDir string
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

// LoadCallLog reads a server-side invocation log (calls.jsonl) into tool-call
// records. A missing log yields an empty slice, never an error.
//
//nolint:gosec // G304: path is a controlled per-run artifact path.
func LoadCallLog(path string) []ToolCallLog {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer func() { _ = f.Close() }()

	var calls []ToolCallLog
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		var rec callLogRecord
		if json.Unmarshal(sc.Bytes(), &rec) != nil {
			continue
		}
		calls = append(calls, ToolCallLog{Tool: rec.Tool, Server: rec.Server})
	}
	return calls
}

// Grade scores an agent run against declarative ground truth. It is a generic
// engine over the check data — deterministic, no model. Success (Overall) is
// judged on the outcome — the result criteria — only; which tool the agent
// picked is recorded as informational telemetry that never flips the verdict (D1).
func Grade(gt *GroundTruth, in GradeInput) *GradingResult {
	result := &GradingResult{Overall: true}

	// Result criteria — these gate Overall.
	for _, want := range gt.AnswerMustContain {
		result.Criteria = append(result.Criteria,
			checkContains("answer_contains", in.FinalAnswer, want, "final answer must contain "+strconv.Quote(want)))
	}
	if gt.CommitCheck != nil {
		result.Criteria = append(result.Criteria,
			checkCommitMatch("commit_check", in.FinalAnswer, gt.CommitCheck.Subject, in.CulpritHash,
				"final answer must identify the correct git commit"))
	}
	for _, a := range gt.Artifacts {
		result.Criteria = append(result.Criteria, checkArtifact(a, in.OutputDir))
	}
	for _, pat := range gt.ForbiddenAnswerPatterns {
		result.Criteria = append(result.Criteria, checkForbiddenAnswer(pat, in.FinalAnswer))
	}

	// Tool-usage checks — informational only, recorded but never gating.
	for _, rt := range gt.RequiredTools {
		result.Informational = append(result.Informational, checkRequiredTool(rt, in.ToolCalls))
	}
	for _, pat := range gt.ForbiddenToolPatterns {
		result.Informational = append(result.Informational, checkForbiddenTool(pat, in.ToolCalls))
	}

	for _, c := range result.Criteria {
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
	if strings.Contains(strings.ToLower(haystack), strings.ToLower(needle)) {
		return CriterionResult{Name: name, Pass: true, Detail: detail}
	}
	return CriterionResult{Name: name, Pass: false, Detail: fmt.Sprintf("%s (expected %q not found)", detail, needle)}
}

// checkCommitMatch checks if the answer contains the commit subject or hash.
func checkCommitMatch(name, haystack, subject, hash, detail string) CriterionResult {
	lower := strings.ToLower(haystack)
	if subject != "" && strings.Contains(lower, strings.ToLower(subject)) {
		return CriterionResult{Name: name, Pass: true, Detail: detail}
	}
	if len(hash) >= 8 && strings.Contains(lower, strings.ToLower(hash[:8])) {
		return CriterionResult{Name: name, Pass: true, Detail: detail + " (matched by hash prefix)"}
	}
	return CriterionResult{Name: name, Pass: false, Detail: fmt.Sprintf("%s (expected subject %q or hash %q not found)", detail, subject, hash)}
}

// requiredToolMatches reports whether a logged call satisfies a required-tool
// spec. An empty Server matches any server; an empty Tool matches any tool on
// the server ("this canonical server was engaged"). Shared by grading and the
// retrieval metric so the two cannot drift.
func requiredToolMatches(rt RequiredTool, c ToolCallLog) bool {
	if rt.Server != "" && c.Server != rt.Server {
		return false
	}
	return rt.Tool == "" || c.Tool == rt.Tool || strings.HasSuffix(c.Tool, "_"+rt.Tool)
}

// checkRequiredTool passes when the invocation log contains a matching call.
// Answer/artifact content checks verify the right tool actually returned data,
// so a server-level requirement still forbids satisfying the task from the wrong
// neighborhood.
func checkRequiredTool(rt RequiredTool, calls []ToolCallLog) CriterionResult {
	name := "required_tool:" + rt.Server + "/" + rt.Tool
	for _, c := range calls {
		if requiredToolMatches(rt, c) {
			return CriterionResult{Name: name, Pass: true, Detail: fmt.Sprintf("%s/%s was called", rt.Server, rt.Tool)}
		}
	}
	return CriterionResult{Name: name, Pass: false, Detail: fmt.Sprintf("required tool %s/%s was never called", rt.Server, rt.Tool)}
}

// checkForbiddenTool fails when any called tool matches the pattern (substring,
// case-insensitive) on either the bare tool name or the "server_tool" form.
func checkForbiddenTool(pattern string, calls []ToolCallLog) CriterionResult {
	name := "forbidden_tool:" + pattern
	p := strings.ToLower(pattern)
	for _, c := range calls {
		if strings.Contains(strings.ToLower(c.Tool), p) ||
			strings.Contains(strings.ToLower(c.Server+"_"+c.Tool), p) {
			return CriterionResult{Name: name, Pass: false, Detail: fmt.Sprintf("forbidden tool called: %s/%s", c.Server, c.Tool)}
		}
	}
	return CriterionResult{Name: name, Pass: true, Detail: "no tool matched " + strconv.Quote(pattern)}
}

// checkForbiddenAnswer fails when the final answer contains the pattern
// (substring, case-insensitive).
func checkForbiddenAnswer(pattern, answer string) CriterionResult {
	name := "forbidden_answer:" + pattern
	if strings.Contains(strings.ToLower(answer), strings.ToLower(pattern)) {
		return CriterionResult{Name: name, Pass: false, Detail: fmt.Sprintf("forbidden phrase in answer: %q", pattern)}
	}
	return CriterionResult{Name: name, Pass: true, Detail: "answer avoids " + strconv.Quote(pattern)}
}

// checkArtifact resolves path under outputDir and verifies the file exists, is
// of the declared type, and contains the required strings. PDFs are parsed and
// text-extracted via the pure-Go reader.
//
//nolint:gosec // G304: path resolves under the trusted per-run output dir.
func checkArtifact(a ArtifactCheck, outputDir string) CriterionResult {
	name := "artifact:" + a.Path
	full := a.Path
	if !filepath.IsAbs(full) {
		full = filepath.Join(outputDir, a.Path)
	}
	data, err := os.ReadFile(full)
	if err != nil {
		return CriterionResult{Name: name, Pass: false, Detail: fmt.Sprintf("artifact %s not found: %v", a.Path, err)}
	}

	var text string
	switch strings.ToLower(a.Type) {
	case "pdf":
		if !IsPDF(data) {
			return CriterionResult{Name: name, Pass: false, Detail: fmt.Sprintf("artifact %s is not a valid PDF", a.Path)}
		}
		text = ExtractPDFText(data)
	default:
		text = string(data)
	}

	lower := strings.ToLower(text)
	for _, want := range a.MustContain {
		if !strings.Contains(lower, strings.ToLower(want)) {
			return CriterionResult{Name: name, Pass: false, Detail: fmt.Sprintf("artifact %s missing %q", a.Path, want)}
		}
	}
	return CriterionResult{Name: name, Pass: true, Detail: fmt.Sprintf("artifact %s exists and contains required facts", a.Path)}
}
