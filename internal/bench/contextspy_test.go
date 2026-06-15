package bench

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
)

// seedContextSpyDB builds a minimal ContextSpy schema with one session whose
// two requests re-send the same tool surface, plus per-tool stats.
func seedContextSpyDB(t *testing.T) string {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "contextspy.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()

	stmts := []string{
		`CREATE TABLE sessions (id VARCHAR, name VARCHAR, started_at DATETIME, ended_at DATETIME, is_active INTEGER)`,
		`CREATE TABLE requests (id VARCHAR, session_id VARCHAR, timestamp DATETIME,
			tokens_system_prompt INTEGER, tokens_tool_definitions INTEGER, tokens_tool_results INTEGER,
			tokens_file_contents INTEGER, tokens_conversation_history INTEGER, tokens_current_user_message INTEGER,
			tokens_assistant_prefill INTEGER, tokens_uncategorized INTEGER, tokens_total_input INTEGER, tokens_total_output INTEGER)`,
		`CREATE TABLE tool_stats (id INTEGER, request_id VARCHAR, tool_name VARCHAR, definition_tokens INTEGER, result_tokens INTEGER)`,
		// An older session with the same name must be ignored (Breakdown takes the latest).
		`INSERT INTO sessions VALUES ('old', 'direct-run-1', '2026-01-01', '2026-01-01', 0)`,
		`INSERT INTO sessions VALUES ('s1', 'direct-run-1', '2026-06-15', '2026-06-15', 0)`,
		`INSERT INTO requests VALUES ('r1','s1','2026-06-15T00:00:01', 100, 500, 10, 0, 0, 20, 0, 0, 630, 40)`,
		`INSERT INTO requests VALUES ('r2','s1','2026-06-15T00:00:02', 100, 500, 80, 0, 50, 5, 0, 0, 735, 30)`,
		// A stray request on the old session must not leak in.
		`INSERT INTO requests VALUES ('rOld','old','2026-01-01T00:00:01', 1, 1, 1, 0, 0, 1, 0, 0, 4, 1)`,
		`INSERT INTO tool_stats VALUES (1,'r1','git_log', 300, 0)`,
		`INSERT INTO tool_stats VALUES (2,'r1','read_file', 200, 0)`,
		`INSERT INTO tool_stats VALUES (3,'r2','git_log', 300, 80)`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			t.Fatalf("seed %q: %v", s, err)
		}
	}
	return dbPath
}

func TestContextSpyBreakdown(t *testing.T) {
	spy := &ContextSpy{bin: []string{"true"}, dbPath: seedContextSpyDB(t)}

	bd, err := spy.Breakdown(context.Background(), "direct-run-1")
	if err != nil {
		t.Fatalf("Breakdown: %v", err)
	}
	if bd == nil {
		t.Fatal("expected a breakdown, got nil")
	}

	if len(bd.Requests) != 2 {
		t.Fatalf("requests = %d, want 2 (latest session only)", len(bd.Requests))
	}
	// Totals sum across the two requests; the old session's row must be excluded.
	if bd.Totals.ToolDefinitions != 1000 {
		t.Errorf("toolDefinitions total = %d, want 1000", bd.Totals.ToolDefinitions)
	}
	if bd.Totals.TotalInput != 1365 {
		t.Errorf("totalInput = %d, want 1365", bd.Totals.TotalInput)
	}
	if bd.Totals.ToolResults != 90 {
		t.Errorf("toolResults = %d, want 90", bd.Totals.ToolResults)
	}

	// Tools are grouped and ordered by definition tokens desc; git_log spans
	// both requests (300+300), read_file only the first.
	if len(bd.Tools) != 2 || bd.Tools[0].Tool != "git_log" {
		t.Fatalf("tools = %+v, want git_log first", bd.Tools)
	}
	if bd.Tools[0].DefinitionTokens != 600 || bd.Tools[0].Occurrences != 2 {
		t.Errorf("git_log = %+v, want 600 def tokens / 2 occurrences", bd.Tools[0])
	}
}

func TestContextSpyDisabled(t *testing.T) {
	// No bin → disabled; every method is a safe no-op.
	var spy ContextSpy
	spy.StartSession(context.Background(), "x")
	spy.EndSession(context.Background())
	bd, err := spy.Breakdown(context.Background(), "x")
	if err != nil || bd != nil {
		t.Fatalf("disabled spy: bd=%v err=%v, want nil/nil", bd, err)
	}
}
