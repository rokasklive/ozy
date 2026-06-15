package bench

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ContextSpy drives a host-side ContextSpy proxy: it opens a named session per
// run (so the proxy tags that run's model requests) and reads ContextSpy's
// SQLite DB for the per-run token breakdown. It is best-effort — when
// ContextSpy isn't provisioned (no DB dir, e.g. the containerized path with no
// host proxy) New returns a disabled spy whose methods no-op, so the bench
// still runs; it just won't emit context-breakdown.json.
type ContextSpy struct {
	bin    []string // CLI invocation, e.g. ["contextspy"] or the uvx form
	dbPath string
}

// NewContextSpy resolves the ContextSpy CLI and DB. CONTEXTSPY_BIN overrides the
// invocation (space-separated argv); CONTEXTSPY_DB overrides the DB path. The
// spy is disabled (a no-op) when the DB's directory is absent.
func NewContextSpy() *ContextSpy {
	db := os.Getenv("CONTEXTSPY_DB")
	if db == "" {
		home, _ := os.UserHomeDir()
		db = filepath.Join(home, ".contextspy", "contextspy.db")
	}
	if _, err := os.Stat(filepath.Dir(db)); err != nil {
		return &ContextSpy{} // disabled: no proxy provisioned
	}
	bin := strings.Fields(os.Getenv("CONTEXTSPY_BIN"))
	if len(bin) == 0 {
		bin = []string{"uvx", "--python", "3.11", "--from", "contextspy==0.2.0", "contextspy"}
	}
	return &ContextSpy{bin: bin, dbPath: db}
}

func (c *ContextSpy) enabled() bool { return c != nil && len(c.bin) > 0 }

// StartSession opens a named session (ending any current one), so requests
// during the run are tagged with it. Best-effort.
func (c *ContextSpy) StartSession(ctx context.Context, name string) {
	if c.enabled() {
		c.cli(ctx, "session", "start", name)
	}
}

// EndSession closes the active session. Best-effort.
func (c *ContextSpy) EndSession(ctx context.Context) {
	if c.enabled() {
		c.cli(ctx, "session", "end")
	}
}

func (c *ContextSpy) cli(ctx context.Context, args ...string) {
	full := append(append([]string(nil), c.bin...), args...)
	cmd := exec.CommandContext(ctx, full[0], full[1:]...)
	if out, err := cmd.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "contextspy %s: %v: %s\n", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
}

// CategoryTokens is ContextSpy's per-request input/output token split by role in
// the context window. Fields mirror the requests table's tokens_* columns.
type CategoryTokens struct {
	SystemPrompt        int `json:"systemPrompt"`
	ToolDefinitions     int `json:"toolDefinitions"`
	ToolResults         int `json:"toolResults"`
	FileContents        int `json:"fileContents"`
	ConversationHistory int `json:"conversationHistory"`
	CurrentUserMessage  int `json:"currentUserMessage"`
	AssistantPrefill    int `json:"assistantPrefill"`
	Uncategorized       int `json:"uncategorized"`
	TotalInput          int `json:"totalInput"`
	TotalOutput         int `json:"totalOutput"`
}

// RequestTokens is one captured request's breakdown, ordered within the run.
type RequestTokens struct {
	Index int `json:"index"`
	CategoryTokens
}

// ToolTokens is a per-tool schema/result token total across the run, summed
// over every request that advertised or called the tool.
type ToolTokens struct {
	Tool             string `json:"tool"`
	DefinitionTokens int    `json:"definitionTokens"`
	ResultTokens     int    `json:"resultTokens"`
	Occurrences      int    `json:"occurrences"`
}

// ContextBreakdown is the per-run capture: every request's breakdown (the tool
// surface is re-sent each turn, so any single request shows its share), the
// summed totals, and the per-tool schema cost.
type ContextBreakdown struct {
	Session  string          `json:"session"`
	Requests []RequestTokens `json:"requests"`
	Totals   CategoryTokens  `json:"totals"`
	Tools    []ToolTokens    `json:"tools,omitempty"`
}

// Breakdown reads the most recent session with the given name and aggregates
// its requests. Returns nil (no error) when the spy is disabled or the session
// captured no requests.
func (c *ContextSpy) Breakdown(ctx context.Context, name string) (*ContextBreakdown, error) {
	if !c.enabled() {
		return nil, nil
	}
	db, err := sql.Open("sqlite", c.dbPath+"?mode=ro")
	if err != nil {
		return nil, fmt.Errorf("open contextspy db: %w", err)
	}
	defer db.Close()

	var sessionID string
	err = db.QueryRowContext(ctx,
		`SELECT id FROM sessions WHERE name = ? ORDER BY started_at DESC LIMIT 1`, name,
	).Scan(&sessionID)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find session %q: %w", name, err)
	}

	rows, err := db.QueryContext(ctx, `
		SELECT tokens_system_prompt, tokens_tool_definitions, tokens_tool_results,
		       tokens_file_contents, tokens_conversation_history, tokens_current_user_message,
		       tokens_assistant_prefill, tokens_uncategorized, tokens_total_input, tokens_total_output
		FROM requests WHERE session_id = ? ORDER BY timestamp`, sessionID)
	if err != nil {
		return nil, fmt.Errorf("read requests: %w", err)
	}
	defer rows.Close()

	bd := &ContextBreakdown{Session: name}
	for rows.Next() {
		var t CategoryTokens
		if err := rows.Scan(&t.SystemPrompt, &t.ToolDefinitions, &t.ToolResults,
			&t.FileContents, &t.ConversationHistory, &t.CurrentUserMessage,
			&t.AssistantPrefill, &t.Uncategorized, &t.TotalInput, &t.TotalOutput); err != nil {
			return nil, fmt.Errorf("scan request: %w", err)
		}
		bd.Requests = append(bd.Requests, RequestTokens{Index: len(bd.Requests), CategoryTokens: t})
		addTokens(&bd.Totals, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate requests: %w", err)
	}
	if len(bd.Requests) == 0 {
		return nil, nil
	}

	bd.Tools, err = readToolTokens(ctx, db, sessionID)
	if err != nil {
		return nil, err
	}
	return bd, nil
}

func readToolTokens(ctx context.Context, db *sql.DB, sessionID string) ([]ToolTokens, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT ts.tool_name, COALESCE(SUM(ts.definition_tokens),0),
		       COALESCE(SUM(ts.result_tokens),0), COUNT(*)
		FROM tool_stats ts JOIN requests r ON r.id = ts.request_id
		WHERE r.session_id = ?
		GROUP BY ts.tool_name ORDER BY SUM(ts.definition_tokens) DESC`, sessionID)
	if err != nil {
		return nil, fmt.Errorf("read tool stats: %w", err)
	}
	defer rows.Close()

	var tools []ToolTokens
	for rows.Next() {
		var t ToolTokens
		if err := rows.Scan(&t.Tool, &t.DefinitionTokens, &t.ResultTokens, &t.Occurrences); err != nil {
			return nil, fmt.Errorf("scan tool stat: %w", err)
		}
		tools = append(tools, t)
	}
	return tools, rows.Err()
}

func addTokens(dst *CategoryTokens, s CategoryTokens) {
	dst.SystemPrompt += s.SystemPrompt
	dst.ToolDefinitions += s.ToolDefinitions
	dst.ToolResults += s.ToolResults
	dst.FileContents += s.FileContents
	dst.ConversationHistory += s.ConversationHistory
	dst.CurrentUserMessage += s.CurrentUserMessage
	dst.AssistantPrefill += s.AssistantPrefill
	dst.Uncategorized += s.Uncategorized
	dst.TotalInput += s.TotalInput
	dst.TotalOutput += s.TotalOutput
}

// WriteBreakdown writes a context breakdown as indented JSON.
func WriteBreakdown(path string, bd *ContextBreakdown) error {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create breakdown file: %w", err)
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	return enc.Encode(bd)
}
