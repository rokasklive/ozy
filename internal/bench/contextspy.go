package bench

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"
)

// ContextSpy drives a ContextSpy proxy over its HTTP API: it opens a named
// session per run (so the proxy tags that run's model requests with it) and
// reads the per-run token breakdown back from the API. It is best-effort — when
// CONTEXTSPY_API is unset, New returns a disabled spy whose methods no-op so the
// bench still runs; it just won't emit context-breakdown.json.
//
// This is the containerized path: the bench runs hermetically in Docker and
// reaches a host-side ContextSpy over HTTP (e.g. host.docker.internal:5173),
// never touching the host filesystem.
type ContextSpy struct {
	api     string
	client  *http.Client
	session string // id of the active session, set by StartSession
}

// NewContextSpy reads CONTEXTSPY_API (e.g. "http://host.docker.internal:5173").
// The spy is disabled (a no-op) when it is unset.
func NewContextSpy() *ContextSpy {
	api := strings.TrimRight(os.Getenv("CONTEXTSPY_API"), "/")
	if api == "" {
		return &ContextSpy{} // disabled: no proxy provisioned
	}
	return &ContextSpy{api: api, client: &http.Client{Timeout: 15 * time.Second}}
}

func (c *ContextSpy) enabled() bool { return c != nil && c.api != "" }

// StartSession opens a named session and remembers its id, so requests during
// the run are tagged with it. Best-effort: on failure the run proceeds with no
// active session (Breakdown then no-ops). Runs are sequential, so a single
// active session id on the struct is sufficient.
func (c *ContextSpy) StartSession(ctx context.Context, name string) {
	c.session = ""
	if !c.enabled() {
		return
	}
	var resp struct {
		Session struct {
			ID string `json:"id"`
		} `json:"session"`
	}
	if err := c.do(ctx, http.MethodPost, "/api/sessions", map[string]string{"name": name}, &resp); err != nil {
		fmt.Fprintf(os.Stderr, "contextspy start session %q: %v\n", name, err)
		return
	}
	c.session = resp.Session.ID
}

// EndSession closes the active session. Best-effort.
func (c *ContextSpy) EndSession(ctx context.Context) {
	if !c.enabled() || c.session == "" {
		return
	}
	if err := c.do(ctx, http.MethodPost, "/api/sessions/"+url.PathEscape(c.session)+"/end", nil, nil); err != nil {
		fmt.Fprintf(os.Stderr, "contextspy end session: %v\n", err)
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
// over every request that advertised or called the tool. Occurrences is not
// exposed by the HTTP API (it was a SQL COUNT) and stays zero.
type ToolTokens struct {
	Tool             string `json:"tool"`
	DefinitionTokens int    `json:"definitionTokens"`
	ResultTokens     int    `json:"resultTokens"`
	Occurrences      int    `json:"occurrences,omitempty"`
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

// apiRequest mirrors one /api/requests row (ContextSpy's snake_case schema).
type apiRequest struct {
	Timestamp           string `json:"timestamp"`
	SystemPrompt        int    `json:"tokens_system_prompt"`
	ToolDefinitions     int    `json:"tokens_tool_definitions"`
	ToolResults         int    `json:"tokens_tool_results"`
	FileContents        int    `json:"tokens_file_contents"`
	ConversationHistory int    `json:"tokens_conversation_history"`
	CurrentUserMessage  int    `json:"tokens_current_user_message"`
	AssistantPrefill    int    `json:"tokens_assistant_prefill"`
	Uncategorized       int    `json:"tokens_uncategorized"`
	TotalInput          int    `json:"tokens_total_input"`
	TotalOutput         int    `json:"tokens_total_output"`
}

func (a apiRequest) categories() CategoryTokens {
	return CategoryTokens{
		SystemPrompt:        a.SystemPrompt,
		ToolDefinitions:     a.ToolDefinitions,
		ToolResults:         a.ToolResults,
		FileContents:        a.FileContents,
		ConversationHistory: a.ConversationHistory,
		CurrentUserMessage:  a.CurrentUserMessage,
		AssistantPrefill:    a.AssistantPrefill,
		Uncategorized:       a.Uncategorized,
		TotalInput:          a.TotalInput,
		TotalOutput:         a.TotalOutput,
	}
}

// Breakdown fetches the active session's per-request breakdown and per-tool
// token totals from the ContextSpy API. Returns nil (no error) when the spy is
// disabled, no session is active, or the session captured no requests.
func (c *ContextSpy) Breakdown(ctx context.Context, name string) (*ContextBreakdown, error) {
	if !c.enabled() || c.session == "" {
		return nil, nil
	}

	var reqResp struct {
		Requests []apiRequest `json:"requests"`
	}
	if err := c.do(ctx, http.MethodGet,
		"/api/requests?session_id="+url.QueryEscape(c.session)+"&limit=500", nil, &reqResp); err != nil {
		return nil, fmt.Errorf("contextspy requests: %w", err)
	}
	if len(reqResp.Requests) == 0 {
		return nil, nil
	}
	// Order by timestamp so request indices match send order.
	sort.SliceStable(reqResp.Requests, func(i, j int) bool {
		return reqResp.Requests[i].Timestamp < reqResp.Requests[j].Timestamp
	})

	bd := &ContextBreakdown{Session: name}
	for _, rr := range reqResp.Requests {
		t := rr.categories()
		bd.Requests = append(bd.Requests, RequestTokens{Index: len(bd.Requests), CategoryTokens: t})
		addTokens(&bd.Totals, t)
	}

	var toolResp struct {
		Tools []struct {
			Tool             string `json:"tool_name"`
			DefinitionTokens int    `json:"definition_tokens"`
			ResultTokens     int    `json:"result_tokens"`
		} `json:"tools"`
	}
	if err := c.do(ctx, http.MethodGet,
		"/api/stats/tools?session_id="+url.QueryEscape(c.session), nil, &toolResp); err != nil {
		return nil, fmt.Errorf("contextspy tool stats: %w", err)
	}
	for _, tt := range toolResp.Tools {
		bd.Tools = append(bd.Tools, ToolTokens{
			Tool:             tt.Tool,
			DefinitionTokens: tt.DefinitionTokens,
			ResultTokens:     tt.ResultTokens,
		})
	}
	return bd, nil
}

// do performs an HTTP request against the ContextSpy API, JSON-encoding body
// (when non-nil) and decoding a 2xx response into out (when non-nil).
func (c *ContextSpy) do(ctx context.Context, method, path string, body, out any) error {
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.api+path, rdr)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("%s %s: %s: %s", method, path, resp.Status, strings.TrimSpace(string(b)))
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
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
