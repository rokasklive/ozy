package bench

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// fakeContextSpy serves the subset of the ContextSpy HTTP API the bench uses:
// session create/end, per-request rows, and per-tool stats. Requests are
// returned out of timestamp order to exercise Breakdown's sort.
func fakeContextSpy(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/sessions", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"session":{"id":"s1"}}`))
	})
	mux.HandleFunc("POST /api/sessions/{id}/end", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"ok":true}`))
	})
	mux.HandleFunc("GET /api/requests", func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("session_id"); got != "s1" {
			t.Errorf("requests session_id = %q, want s1", got)
		}
		_, _ = w.Write([]byte(`{"requests":[
			{"timestamp":"2026-06-15T00:00:02","tokens_system_prompt":100,"tokens_tool_definitions":500,"tokens_tool_results":80,"tokens_file_contents":0,"tokens_conversation_history":50,"tokens_current_user_message":5,"tokens_assistant_prefill":0,"tokens_uncategorized":0,"tokens_total_input":735,"tokens_total_output":30},
			{"timestamp":"2026-06-15T00:00:01","tokens_system_prompt":100,"tokens_tool_definitions":500,"tokens_tool_results":10,"tokens_file_contents":0,"tokens_conversation_history":0,"tokens_current_user_message":20,"tokens_assistant_prefill":0,"tokens_uncategorized":0,"tokens_total_input":630,"tokens_total_output":40}
		]}`))
	})
	mux.HandleFunc("GET /api/stats/tools", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"tools":[
			{"tool_name":"git_log","definition_tokens":600,"result_tokens":80},
			{"tool_name":"read_file","definition_tokens":200,"result_tokens":0}
		]}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestContextSpyBreakdown(t *testing.T) {
	srv := fakeContextSpy(t)
	t.Setenv("CONTEXTSPY_API", srv.URL)

	spy := NewContextSpy()
	spy.StartSession(context.Background(), "direct-run-1")
	if spy.session != "s1" {
		t.Fatalf("session id = %q, want s1 (StartSession must stash it)", spy.session)
	}

	bd, err := spy.Breakdown(context.Background(), "direct-run-1")
	if err != nil {
		t.Fatalf("Breakdown: %v", err)
	}
	if bd == nil {
		t.Fatal("expected a breakdown, got nil")
	}

	if len(bd.Requests) != 2 {
		t.Fatalf("requests = %d, want 2", len(bd.Requests))
	}
	// Sorted by timestamp: the 00:00:01 row (systemPrompt+currentUser=120, no
	// conversation history) must come first despite being sent second.
	if bd.Requests[0].CurrentUserMessage != 20 || bd.Requests[0].ConversationHistory != 0 {
		t.Errorf("requests not ordered by timestamp: first = %+v", bd.Requests[0].CategoryTokens)
	}
	// Totals sum across the two requests.
	if bd.Totals.ToolDefinitions != 1000 {
		t.Errorf("toolDefinitions total = %d, want 1000", bd.Totals.ToolDefinitions)
	}
	if bd.Totals.TotalInput != 1365 {
		t.Errorf("totalInput = %d, want 1365", bd.Totals.TotalInput)
	}
	if bd.Totals.ToolResults != 90 {
		t.Errorf("toolResults = %d, want 90", bd.Totals.ToolResults)
	}

	// Tools come pre-aggregated from the API, ordered by definition tokens desc.
	if len(bd.Tools) != 2 || bd.Tools[0].Tool != "git_log" {
		t.Fatalf("tools = %+v, want git_log first", bd.Tools)
	}
	if bd.Tools[0].DefinitionTokens != 600 || bd.Tools[0].ResultTokens != 80 {
		t.Errorf("git_log = %+v, want 600 def / 80 result tokens", bd.Tools[0])
	}
}

func TestContextSpyDisabled(t *testing.T) {
	// No CONTEXTSPY_API → disabled; every method is a safe no-op.
	t.Setenv("CONTEXTSPY_API", "")
	spy := NewContextSpy()
	spy.StartSession(context.Background(), "x")
	spy.EndSession(context.Background())
	bd, err := spy.Breakdown(context.Background(), "x")
	if err != nil || bd != nil {
		t.Fatalf("disabled spy: bd=%v err=%v, want nil/nil", bd, err)
	}
}
