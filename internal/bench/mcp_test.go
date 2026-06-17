package bench

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// connectTestServer creates an MCP server for the given toolset, wires it
// through in-memory transports, and returns a connected client session.
func connectTestServer(t *testing.T, toolset string, fixtureDir string) *mcpsdk.ClientSession {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	srv, err := newMCPServer(toolset, fixtureDir)
	if err != nil {
		t.Fatalf("newMCPServer(%q): %v", toolset, err)
	}

	serverT, clientT := mcpsdk.NewInMemoryTransports()
	go func() { _ = srv.Run(ctx, serverT) }()

	client := mcpsdk.NewClient(&mcpsdk.Implementation{Name: "test", Version: "0"}, nil)
	cs, err := client.Connect(ctx, clientT, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { _ = cs.Close() })
	return cs
}

// textContent returns the text from the first content block of a tool
// result, or fails the test.
func textContent(t *testing.T, res *mcpsdk.CallToolResult) string {
	t.Helper()
	if len(res.Content) == 0 {
		t.Fatal("tool result had no content")
	}
	tc, ok := res.Content[0].(*mcpsdk.TextContent)
	if !ok {
		t.Fatalf("content[0] is %T, want *TextContent", res.Content[0])
	}
	return tc.Text
}

// jsonPayload parses the text content of a tool result as a JSON object.
func jsonPayload(t *testing.T, res *mcpsdk.CallToolResult) map[string]any {
	t.Helper()
	text := textContent(t, res)
	var payload map[string]any
	if err := json.Unmarshal([]byte(text), &payload); err != nil {
		t.Fatalf("content is not valid JSON: %v\n%s", err, text)
	}
	return payload
}

// ---------------------------------------------------------------------------
// TestMCPServerCodeSearch
// ---------------------------------------------------------------------------

func TestMCPServerCodeSearch(t *testing.T) {
	t.Parallel()

	if _, err := exec.LookPath("rg"); err != nil {
		t.Skip("rg not available")
	}

	fixtureDir := t.TempDir()
	_, err := GenerateFixture(fixtureDir)
	if err != nil {
		t.Fatalf("GenerateFixture: %v", err)
	}

	cs := connectTestServer(t, "code-search", fixtureDir)

	res, err := cs.CallTool(context.Background(), &mcpsdk.CallToolParams{
		Name:      "search_text",
		Arguments: map[string]any{"query": "SUSPENDED"},
	})
	if err != nil {
		t.Fatalf("CallTool(search_text): %v", err)
	}
	if res.IsError {
		t.Fatalf("search_text returned IsError=true; content=%+v", res.Content)
	}

	payload := jsonPayload(t, res)
	results, ok := payload["results"].(string)
	if !ok {
		t.Fatalf("results field is not a string: %T", payload["results"])
	}
	if !strings.Contains(results, "StatusMapper.java") {
		t.Errorf("search_text results should contain StatusMapper.java, got: %s", results)
	}
}

// ---------------------------------------------------------------------------
// TestMCPServerGit
// ---------------------------------------------------------------------------

func TestMCPServerGit(t *testing.T) {
	t.Parallel()

	gitDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(gitDir, "README.md"), []byte("# test\n"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	runGit := func(extraEnv []string, args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = gitDir
		cmd.Env = append(os.Environ(), extraEnv...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, string(out))
		}
	}

	runGit(nil, "init")
	runGit(nil, "checkout", "-b", "main")

	env := []string{
		"GIT_AUTHOR_NAME=Test",
		"GIT_AUTHOR_EMAIL=test@example.com",
		"GIT_COMMITTER_NAME=Test",
		"GIT_COMMITTER_EMAIL=test@example.com",
	}
	runGit(env, "add", "-A")
	runGit(env, "commit", "-m", "initial commit")

	cs := connectTestServer(t, "git", gitDir)

	res, err := cs.CallTool(context.Background(), &mcpsdk.CallToolParams{
		Name:      "git_log",
		Arguments: map[string]any{},
	})
	if err != nil {
		t.Fatalf("CallTool(git_log): %v", err)
	}

	payload := jsonPayload(t, res)
	commits, ok := payload["commits"].(string)
	if !ok {
		t.Fatalf("commits field is not a string: %T", payload["commits"])
	}
	if commits == "" {
		t.Error("git_log returned empty commits string")
	}
	if !strings.Contains(commits, "initial commit") {
		t.Errorf("git_log should contain 'initial commit', got: %s", commits)
	}
}

// ---------------------------------------------------------------------------
// TestMCPServerIncidentDB
// ---------------------------------------------------------------------------

func TestMCPServerIncidentDB(t *testing.T) {
	t.Parallel()

	fixtureDir := t.TempDir()
	_, err := GenerateFixture(fixtureDir)
	if err != nil {
		t.Fatalf("GenerateFixture: %v", err)
	}

	cs := connectTestServer(t, "incident-db", fixtureDir)

	// Call list_tables.
	res, err := cs.CallTool(context.Background(), &mcpsdk.CallToolParams{
		Name:      "list_tables",
		Arguments: map[string]any{},
	})
	if err != nil {
		t.Fatalf("CallTool(list_tables): %v", err)
	}
	payload := jsonPayload(t, res)
	tables, ok := payload["tables"].([]any)
	if !ok {
		t.Fatalf("tables field is not an array: %T", payload["tables"])
	}
	if len(tables) == 0 {
		t.Error("list_tables returned empty tables array")
	}
	foundInvoices := false
	for _, tbl := range tables {
		if name, _ := tbl.(string); name == "invoices" {
			foundInvoices = true
			break
		}
	}
	if !foundInvoices {
		t.Errorf("list_tables should include 'invoices', got: %v", tables)
	}

	// Call query_readonly with a valid SELECT.
	res, err = cs.CallTool(context.Background(), &mcpsdk.CallToolParams{
		Name:      "query_readonly",
		Arguments: map[string]any{"query": "SELECT count(*) FROM invoices"},
	})
	if err != nil {
		t.Fatalf("CallTool(query_readonly SELECT): %v", err)
	}
	if res.IsError {
		t.Fatalf("SELECT query returned IsError=true; content=%+v", res.Content)
	}

	// Call query_readonly with INSERT (must be rejected).
	res, err = cs.CallTool(context.Background(), &mcpsdk.CallToolParams{
		Name:      "query_readonly",
		Arguments: map[string]any{"query": "INSERT INTO invoices VALUES (99, 'X', 'X', 'X', 0, '2026', 'X')"},
	})
	if err != nil {
		t.Fatalf("CallTool(query_readonly INSERT): %v", err)
	}
	payload = jsonPayload(t, res)
	if payload["error"] != "write statements are not allowed" {
		t.Errorf("INSERT should be rejected with 'write statements are not allowed', got: %v", payload["error"])
	}
}

// ---------------------------------------------------------------------------
// TestMCPServerReadOnlyEnforcement
// ---------------------------------------------------------------------------

func TestMCPServerReadOnlyEnforcement(t *testing.T) {
	t.Parallel()

	fixtureDir := t.TempDir()
	_, err := GenerateFixture(fixtureDir)
	if err != nil {
		t.Fatalf("GenerateFixture: %v", err)
	}

	cs := connectTestServer(t, "incident-db", fixtureDir)

	forbiddenQueries := []string{
		"INSERT INTO invoices VALUES (1, 'X', 'X', 'X', 0, '2026', 'X')",
		"UPDATE invoices SET status='X' WHERE id=1",
		"DELETE FROM invoices WHERE id=1",
		"DROP TABLE invoices",
		"ALTER TABLE invoices ADD COLUMN foo TEXT",
	}

	for _, q := range forbiddenQueries {
		res, err := cs.CallTool(context.Background(), &mcpsdk.CallToolParams{
			Name:      "query_readonly",
			Arguments: map[string]any{"query": q},
		})
		if err != nil {
			t.Fatalf("CallTool(query_readonly %s): %v", q, err)
		}
		payload := jsonPayload(t, res)
		if payload["error"] != "write statements are not allowed" {
			t.Errorf("query %q should be rejected, got error: %v", q, payload["error"])
		}
		if payload["detail"] == nil {
			t.Errorf("rejection for %q should include detail", q)
		}
	}
}

// ---------------------------------------------------------------------------
// TestMCPServerDistractors
// ---------------------------------------------------------------------------

func TestMCPServerDistractors(t *testing.T) {
	t.Parallel()

	cs := connectTestServer(t, "time", "")

	res, err := cs.CallTool(context.Background(), &mcpsdk.CallToolParams{
		Name:      "current_time",
		Arguments: map[string]any{},
	})
	if err != nil {
		t.Fatalf("CallTool(current_time): %v", err)
	}
	if res.IsError {
		t.Fatalf("current_time returned IsError=true; content=%+v", res.Content)
	}

	payload := jsonPayload(t, res)
	timeStr, ok := payload["time"].(string)
	if !ok || timeStr == "" {
		t.Fatalf("current_time should return a 'time' string, got: %v", payload)
	}
	tz, ok := payload["timezone"].(string)
	if !ok || tz != "UTC" {
		t.Errorf("timezone = %v, want UTC", payload["timezone"])
	}

	if !strings.Contains(timeStr, "T") {
		t.Errorf("time string %q does not look like ISO 8601", timeStr)
	}
}

// ---------------------------------------------------------------------------
// TestMCPServerToolsetIsolation
// ---------------------------------------------------------------------------

func TestMCPServerToolsetIsolation(t *testing.T) {
	t.Parallel()

	fixtureDir := t.TempDir()
	_, err := GenerateFixture(fixtureDir)
	if err != nil {
		t.Fatalf("GenerateFixture: %v", err)
	}

	cs := connectTestServer(t, "code-search", fixtureDir)

	list, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}

	got := make(map[string]bool, len(list.Tools))
	for _, tool := range list.Tools {
		got[tool.Name] = true
	}

	// Expected code-search tools.
	expectedTools := []string{"search_text", "search_symbol", "read_file", "find_references"}
	for _, name := range expectedTools {
		if !got[name] {
			t.Errorf("missing expected code-search tool %q", name)
		}
	}

	// These must NOT be present.
	forbiddenTools := []string{
		"git_log", "git_show", "git_blame", "git_diff",
		"list_tables", "describe_table", "query_readonly",
		"list_dir",
		"current_time", "convert_timezone",
		"search_memory", "store_memory",
		"create_plan", "append_note",
	}
	for _, name := range forbiddenTools {
		if got[name] {
			t.Errorf("toolset isolation violated: %q leaked into code-search toolset", name)
		}
	}

	// Ensure exact count.
	if len(list.Tools) != len(expectedTools) {
		t.Errorf("advertised %d tools, want %d: %v", len(list.Tools), len(expectedTools), got)
	}
}
