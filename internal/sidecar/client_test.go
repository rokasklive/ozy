package sidecar

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/rokasklive/ozy/internal/search"
)

// newTestClient constructs a Client backed by a ScriptedSidecar and
// registers a cleanup that calls Close. The Client does NOT have a
// successful health probe; callers must call Health first if they
// need the client to be Available().
func newTestClient(t *testing.T, s *ScriptedSidecar) *Client {
	t.Helper()
	c, err := NewClient(Options{
		Driver: newFakeDriver(s),
		Logger: noopLogger{},
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
}

// healthOK returns a scripted response map for a successful health probe.
func healthOK(req map[string]any) map[string]any {
	return map[string]any{
		"id":          req["id"],
		"ok":          true,
		"model":       "BAAI/bge-small-en-v1.5",
		"dim":         384,
		"backend":     "turbovec",
		"vectorCount": 0,
	}
}

// healthNotOK returns a scripted response for a failed health probe.
func healthNotOK(req map[string]any) map[string]any {
	return map[string]any{
		"id":    req["id"],
		"ok":    false,
		"error": "model download failed",
	}
}

func TestClient_Health_HappyPath(t *testing.T) {
	t.Parallel()
	c := newTestClient(t, &ScriptedSidecar{
		OnRequest: func(req map[string]any) ScriptedResponse {
			return ScriptedResponse{Response: healthOK(req)}
		},
	})
	h := c.Health(context.Background())
	if !h.OK {
		t.Fatalf("Health OK = false, err = %v", h.Err)
	}
	if !h.Available {
		t.Error("Health Available = false")
	}
	if h.Model != "BAAI/bge-small-en-v1.5" {
		t.Errorf("Model = %s, want BAAI/bge-small-en-v1.5", h.Model)
	}
	if h.Dim != 384 {
		t.Errorf("Dim = %d, want 384", h.Dim)
	}
	if h.Backend != "turbovec" {
		t.Errorf("Backend = %s, want turbovec", h.Backend)
	}
	if !c.Available() {
		t.Error("Client.Available() = false after successful Health")
	}
}

func TestClient_Health_NotOK(t *testing.T) {
	t.Parallel()
	c := newTestClient(t, &ScriptedSidecar{
		OnRequest: func(req map[string]any) ScriptedResponse {
			return ScriptedResponse{Response: healthNotOK(req)}
		},
	})
	h := c.Health(context.Background())
	if h.OK {
		t.Error("Health OK = true, want false")
	}
	if h.Available {
		t.Error("Health Available = true, want false")
	}
	if h.Err == nil {
		t.Error("Health.Err = nil, want non-nil error")
	}
	if c.Available() {
		t.Error("Client.Available() = true after failed Health")
	}
}

func TestClient_Upsert_HappyPath(t *testing.T) {
	t.Parallel()
	c := newTestClient(t, &ScriptedSidecar{
		OnRequest: func(req map[string]any) ScriptedResponse {
			op, _ := req["op"].(string)
			switch op {
			case opHealth:
				return ScriptedResponse{Response: healthOK(req)}
			case opUpsert:
				return ScriptedResponse{Response: map[string]any{
					"id":       req["id"],
					"ok":       true,
					"upserted": 3,
					"skipped":  1,
					"errors":   []any{},
				}}
			default:
				return ScriptedResponse{}
			}
		},
	})
	if h := c.Health(context.Background()); !h.OK {
		t.Fatalf("Health: %v", h.Err)
	}
	result, err := c.Upsert(context.Background(), []UpsertItem{
		{ToolRef: "a.search", Text: "search tool", ContentHash: "abc", ServerID: "srv"},
		{ToolRef: "b.other", Text: "other", ContentHash: "def", ServerID: "srv"},
		{ToolRef: "c.more", Text: "more", ContentHash: "ghi", ServerID: "srv"},
		{ToolRef: "d.skip", Text: "skip", ContentHash: "same", ServerID: "srv"},
	})
	if err != nil {
		t.Fatalf("Upsert error = %v", err)
	}
	if result.Upserted != 3 {
		t.Errorf("upserted = %d, want 3", result.Upserted)
	}
	if result.Skipped != 1 {
		t.Errorf("skipped = %d, want 1", result.Skipped)
	}
}

func TestClient_Upsert_EmptyInput(t *testing.T) {
	t.Parallel()
	c := newTestClient(t, &ScriptedSidecar{
		OnRequest: func(req map[string]any) ScriptedResponse {
			return ScriptedResponse{Response: healthOK(req)}
		},
	})
	if h := c.Health(context.Background()); !h.OK {
		t.Fatalf("Health: %v", h.Err)
	}
	result, err := c.Upsert(context.Background(), nil)
	if err != nil {
		t.Fatalf("Upsert(nil) error = %v", err)
	}
	if result.Upserted != 0 || result.Skipped != 0 {
		t.Errorf("upserted=%d skipped=%d, want 0,0", result.Upserted, result.Skipped)
	}
}

func TestClient_Delete_HappyPath(t *testing.T) {
	t.Parallel()
	c := newTestClient(t, &ScriptedSidecar{
		OnRequest: func(req map[string]any) ScriptedResponse {
			op, _ := req["op"].(string)
			switch op {
			case opHealth:
				return ScriptedResponse{Response: healthOK(req)}
			case opDelete:
				return ScriptedResponse{Response: map[string]any{
					"id":      req["id"],
					"ok":      true,
					"deleted": 2,
				}}
			default:
				return ScriptedResponse{}
			}
		},
	})
	if h := c.Health(context.Background()); !h.OK {
		t.Fatalf("Health: %v", h.Err)
	}
	result, err := c.Delete(context.Background(), []string{"a.search", "b.other"})
	if err != nil {
		t.Fatalf("Delete error = %v", err)
	}
	if result.Deleted != 2 {
		t.Errorf("deleted = %d, want 2", result.Deleted)
	}
}

func TestClient_Delete_EmptyInput(t *testing.T) {
	t.Parallel()
	c := newTestClient(t, &ScriptedSidecar{
		OnRequest: func(req map[string]any) ScriptedResponse {
			return ScriptedResponse{Response: healthOK(req)}
		},
	})
	if h := c.Health(context.Background()); !h.OK {
		t.Fatalf("Health: %v", h.Err)
	}
	result, err := c.Delete(context.Background(), nil)
	if err != nil {
		t.Fatalf("Delete(nil) error = %v", err)
	}
	if result.Deleted != 0 {
		t.Errorf("deleted = %d, want 0", result.Deleted)
	}
}

func TestClient_Query_HappyPath(t *testing.T) {
	t.Parallel()
	c := newTestClient(t, &ScriptedSidecar{
		OnRequest: func(req map[string]any) ScriptedResponse {
			op, _ := req["op"].(string)
			switch op {
			case opHealth:
				return ScriptedResponse{Response: healthOK(req)}
			case opQuery:
				return ScriptedResponse{Response: map[string]any{
					"id": req["id"],
					"ok": true,
					"hits": []any{
						map[string]any{"toolRef": "a.search", "score": 0.95},
						map[string]any{"toolRef": "b.other", "score": 0.72},
					},
				}}
			default:
				return ScriptedResponse{}
			}
		},
	})
	if h := c.Health(context.Background()); !h.OK {
		t.Fatalf("Health: %v", h.Err)
	}
	result, err := c.Query(context.Background(), "search query", 5, SearchFilter{})
	if err != nil {
		t.Fatalf("Query error = %v", err)
	}
	if len(result.Hits) != 2 {
		t.Fatalf("hits = %d, want 2", len(result.Hits))
	}
	if result.Hits[0].ToolRef != "a.search" || result.Hits[0].Score != 0.95 {
		t.Errorf("hit[0] = {%s, %.2f}, want {a.search, 0.95}", result.Hits[0].ToolRef, result.Hits[0].Score)
	}
	if result.Hits[1].ToolRef != "b.other" || result.Hits[1].Score != 0.72 {
		t.Errorf("hit[1] = {%s, %.2f}, want {b.other, 0.72}", result.Hits[1].ToolRef, result.Hits[1].Score)
	}
}

func TestClient_Query_WithFilter(t *testing.T) {
	t.Parallel()
	var gotFilter map[string]any
	c := newTestClient(t, &ScriptedSidecar{
		OnRequest: func(req map[string]any) ScriptedResponse {
			op, _ := req["op"].(string)
			switch op {
			case opHealth:
				return ScriptedResponse{Response: healthOK(req)}
			case opQuery:
				gotFilter = nil
				if f, ok := req["filter"]; ok && f != nil {
					if fm, ok := f.(map[string]any); ok {
						gotFilter = fm
					}
				}
				return ScriptedResponse{Response: map[string]any{
					"id":   req["id"],
					"ok":   true,
					"hits": []any{},
				}}
			default:
				return ScriptedResponse{}
			}
		},
	})
	if h := c.Health(context.Background()); !h.OK {
		t.Fatalf("Health: %v", h.Err)
	}
	_, err := c.Query(context.Background(), "test", 3, SearchFilter{ServerID: "atlassian"})
	if err != nil {
		t.Fatalf("Query error = %v", err)
	}
	if gotFilter == nil {
		t.Fatal("filter not received by sidecar")
	}
	if sid, _ := gotFilter["serverId"].(string); sid != "atlassian" {
		t.Errorf("serverId = %q, want atlassian", sid)
	}
}

func TestClient_Query_NoFilter(t *testing.T) {
	t.Parallel()
	var gotFilter any
	c := newTestClient(t, &ScriptedSidecar{
		OnRequest: func(req map[string]any) ScriptedResponse {
			op, _ := req["op"].(string)
			switch op {
			case opHealth:
				return ScriptedResponse{Response: healthOK(req)}
			case opQuery:
				gotFilter = req["filter"]
				return ScriptedResponse{Response: map[string]any{
					"id":   req["id"],
					"ok":   true,
					"hits": []any{},
				}}
			default:
				return ScriptedResponse{}
			}
		},
	})
	if h := c.Health(context.Background()); !h.OK {
		t.Fatalf("Health: %v", h.Err)
	}
	_, err := c.Query(context.Background(), "test", 3, SearchFilter{})
	if err != nil {
		t.Fatalf("Query error = %v", err)
	}
	if gotFilter != nil {
		t.Errorf("filter should be null, got %v", gotFilter)
	}
}

func TestClient_Stats_HappyPath(t *testing.T) {
	t.Parallel()
	c := newTestClient(t, &ScriptedSidecar{
		OnRequest: func(req map[string]any) ScriptedResponse {
			op, _ := req["op"].(string)
			switch op {
			case opHealth:
				return ScriptedResponse{Response: healthOK(req)}
			case opStats:
				return ScriptedResponse{Response: map[string]any{
					"id":          req["id"],
					"ok":          true,
					"backend":     "turbovec",
					"model":       "BAAI/bge-small-en-v1.5",
					"dim":         384,
					"vectorCount": 42,
					"toolCount":   10,
				}}
			default:
				return ScriptedResponse{}
			}
		},
	})
	if h := c.Health(context.Background()); !h.OK {
		t.Fatalf("Health: %v", h.Err)
	}
	s, err := c.Stats(context.Background())
	if err != nil {
		t.Fatalf("Stats error = %v", err)
	}
	if s.Backend != "turbovec" {
		t.Errorf("Backend = %s", s.Backend)
	}
	if s.VectorCount != 42 {
		t.Errorf("VectorCount = %d, want 42", s.VectorCount)
	}
	if s.ToolCount != 10 {
		t.Errorf("ToolCount = %d, want 10", s.ToolCount)
	}
}

func TestClient_RequestTimeout(t *testing.T) {
	t.Parallel()
	c := newTestClient(t, &ScriptedSidecar{
		OnRequest: func(req map[string]any) ScriptedResponse {
			return ScriptedResponse{
				Delay:    500 * time.Millisecond,
				Response: healthOK(req),
			}
		},
	})
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	h := c.Health(ctx)
	if h.Available {
		t.Error("Health should report unavailable after request timeout")
	}
	if h.Err == nil {
		t.Error("Health.Err should be non-nil after timeout")
	}
	if !errors.Is(h.Err, context.DeadlineExceeded) {
		t.Logf("timeout error = %v (not DeadlineExceeded; acceptable)", h.Err)
	}
	if c.Available() {
		t.Error("Client should be unavailable after request timeout")
	}
}

func TestClient_ExitMidSession(t *testing.T) {
	t.Parallel()
	c := newTestClient(t, &ScriptedSidecar{
		OnRequest: func(req map[string]any) ScriptedResponse {
			return ScriptedResponse{
				Response: healthOK(req),
				Exit:     true,
			}
		},
	})
	// First request succeeds.
	h := c.Health(context.Background())
	if !h.OK {
		t.Fatalf("first Health: OK = false, err = %v", h.Err)
	}

	// Give the readLoop a moment to notice EOF.
	time.Sleep(50 * time.Millisecond)

	// Second request should be rejected because the sidecar exited
	// and the client is now unhealthy.
	h2 := c.Health(context.Background())
	if h2.Available {
		t.Error("second Health should report unavailable after sidecar exit")
	}
	if !errIsUnavailable(h2.Err) {
		t.Errorf("second Health error = %v, want unavailable", h2.Err)
	}
}

// errIsUnavailable reports whether an error indicates the sidecar is
// unavailable (sentinel, "stdout closed", "reader exited", "closed pipe",
// or "unavailable" substring).
func errIsUnavailable(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, errUnavailable) {
		return true
	}
	msg := err.Error()
	return strings.Contains(msg, "unavailable") ||
		strings.Contains(msg, "stdout closed") ||
		strings.Contains(msg, "reader exited") ||
		strings.Contains(msg, "closed pipe")
}

func TestClient_GarbageResponse(t *testing.T) {
	t.Parallel()
	c := newTestClient(t, &ScriptedSidecar{
		OnRequest: func(req map[string]any) ScriptedResponse {
			return ScriptedResponse{Garbage: "this is not JSON at all"}
		},
	})
	h := c.Health(context.Background())
	if h.Available {
		t.Error("Health should report unavailable after garbage response")
	}
	if h.Err == nil {
		t.Error("Health.Err should be non-nil after garbage response")
	}
	if c.Available() {
		t.Error("Client should be unavailable after garbage response")
	}
}

func TestClient_MalformedResponse_MissingID(t *testing.T) {
	t.Parallel()
	c := newTestClient(t, &ScriptedSidecar{
		OnRequest: func(req map[string]any) ScriptedResponse {
			return ScriptedResponse{
				Response: map[string]any{
					"ok":    true,
					"model": "test",
				},
			}
		},
	})
	h := c.Health(context.Background())
	if h.Available {
		t.Error("Health should report unavailable after response missing id")
	}
	if h.Err == nil {
		t.Error("Health.Err should be non-nil")
	}
}

func TestClient_Close_Idempotent(t *testing.T) {
	t.Parallel()
	c := newTestClient(t, &ScriptedSidecar{
		OnRequest: func(req map[string]any) ScriptedResponse {
			return ScriptedResponse{Response: healthOK(req)}
		},
	})
	if h := c.Health(context.Background()); !h.OK {
		t.Fatalf("Health: %v", h.Err)
	}
	if err := c.Close(); err != nil {
		t.Errorf("first Close: %v", err)
	}
	if err := c.Close(); err != nil {
		t.Errorf("second Close: %v", err)
	}
}

func TestClient_UnavailableAfterClose(t *testing.T) {
	t.Parallel()
	c := newTestClient(t, &ScriptedSidecar{
		OnRequest: func(req map[string]any) ScriptedResponse {
			return ScriptedResponse{Response: healthOK(req)}
		},
	})
	if h := c.Health(context.Background()); !h.OK {
		t.Fatalf("Health: %v", h.Err)
	}
	if err := c.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if c.Available() {
		t.Error("Client should be unavailable after Close")
	}
	_, err := c.Upsert(context.Background(), []UpsertItem{
		{ToolRef: "test", Text: "test", ContentHash: "abc", ServerID: "srv"},
	})
	if err == nil {
		t.Error("Upsert after Close should return error")
	}
	if !errors.Is(err, errUnavailable) {
		t.Errorf("Upsert error = %v, want errUnavailable", err)
	}
}

func TestClient_UnavailableAfterUnhealthy(t *testing.T) {
	t.Parallel()
	c := newTestClient(t, &ScriptedSidecar{
		OnRequest: func(req map[string]any) ScriptedResponse {
			return ScriptedResponse{Response: healthNotOK(req)}
		},
	})
	h := c.Health(context.Background())
	if h.Available {
		t.Fatal("Health should report unavailable")
	}
	_, err := c.Query(context.Background(), "query", 5, SearchFilter{})
	if err == nil {
		t.Error("Query after unhealthy should return error")
	}
	if !errors.Is(err, errUnavailable) {
		t.Errorf("Query error = %v, want errUnavailable", err)
	}
}

func TestClient_Health_CanReProbe(t *testing.T) {
	t.Parallel()
	first := true
	c := newTestClient(t, &ScriptedSidecar{
		OnRequest: func(req map[string]any) ScriptedResponse {
			if first {
				first = false
				return ScriptedResponse{Response: healthNotOK(req)}
			}
			return ScriptedResponse{Response: healthOK(req)}
		},
	})
	h1 := c.Health(context.Background())
	if h1.Available {
		t.Fatal("first Health should be unavailable")
	}
	h2 := c.Health(context.Background())
	if !h2.OK {
		t.Fatalf("second Health: OK = false, err = %v", h2.Err)
	}
	if !c.Available() {
		t.Error("Client should be available after Health re-probe")
	}
}

func TestClient_Available_BeforeHealth(t *testing.T) {
	t.Parallel()
	c := newTestClient(t, &ScriptedSidecar{
		OnRequest: func(req map[string]any) ScriptedResponse {
			return ScriptedResponse{Response: healthOK(req)}
		},
	})
	if c.Available() {
		t.Error("Client should be unavailable before first Health")
	}
}

func TestSearchFilter_IsEmpty(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		filter SearchFilter
		empty  bool
	}{
		{"zero", SearchFilter{}, true},
		{"empty server ID", SearchFilter{ServerID: ""}, true},
		{"with server ID", SearchFilter{ServerID: "atlassian"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.filter.IsEmpty(); got != tt.empty {
				t.Errorf("IsEmpty() = %v, want %v", got, tt.empty)
			}
		})
	}
}

// ── SemanticAdapter tests ──────────────────────────────────────────

func TestSemanticAdapter_Query_HappyPath(t *testing.T) {
	t.Parallel()
	c := newTestClient(t, &ScriptedSidecar{
		OnRequest: func(req map[string]any) ScriptedResponse {
			op, _ := req["op"].(string)
			switch op {
			case opHealth:
				return ScriptedResponse{Response: healthOK(req)}
			case opQuery:
				return ScriptedResponse{Response: map[string]any{
					"id": req["id"],
					"ok": true,
					"hits": []any{
						map[string]any{"toolRef": "a.search", "score": 0.88},
					},
				}}
			default:
				return ScriptedResponse{}
			}
		},
	})
	if h := c.Health(context.Background()); !h.OK {
		t.Fatalf("Health: %v", h.Err)
	}
	adapter := NewSemanticAdapter(c)
	if !adapter.Available() {
		t.Fatal("adapter should be available after health")
	}

	hits, err := adapter.Query(context.Background(), "search phrase", 3, search.Filter{})
	if err != nil {
		t.Fatalf("Query error = %v", err)
	}
	if len(hits) != 1 {
		t.Fatalf("hits = %d, want 1", len(hits))
	}
	if hits[0].ToolRef != "a.search" {
		t.Errorf("ToolRef = %s, want a.search", hits[0].ToolRef)
	}
	if hits[0].Score != 0.88 {
		t.Errorf("Score = %.2f, want 0.88", hits[0].Score)
	}
}

func TestSemanticAdapter_Available_ReflectsClient(t *testing.T) {
	t.Parallel()
	c := newTestClient(t, &ScriptedSidecar{
		OnRequest: func(req map[string]any) ScriptedResponse {
			return ScriptedResponse{Response: healthOK(req)}
		},
	})
	adapter := NewSemanticAdapter(c)
	if adapter.Available() {
		t.Error("adapter should be unavailable before Health")
	}
	if h := c.Health(context.Background()); !h.OK {
		t.Fatalf("Health: %v", h.Err)
	}
	if !adapter.Available() {
		t.Error("adapter should be available after Health")
	}
}

func TestSemanticAdapter_DegradesOnError(t *testing.T) {
	t.Parallel()
	c := newTestClient(t, &ScriptedSidecar{
		OnRequest: func(req map[string]any) ScriptedResponse {
			op, _ := req["op"].(string)
			switch op {
			case opHealth:
				return ScriptedResponse{Response: healthOK(req)}
			case opQuery:
				return ScriptedResponse{Garbage: "not json"}
			default:
				return ScriptedResponse{}
			}
		},
	})
	if h := c.Health(context.Background()); !h.OK {
		t.Fatalf("Health: %v", h.Err)
	}
	adapter := NewSemanticAdapter(c)
	if !adapter.Available() {
		t.Fatal("adapter should be available before query")
	}

	hits, err := adapter.Query(context.Background(), "query", 3, search.Filter{})
	if err != nil {
		t.Fatalf("adapter.Query should not return error (degrade, not fail): %v", err)
	}
	if hits != nil {
		t.Error("hits should be nil on error")
	}
	if adapter.Available() {
		t.Error("adapter should be unavailable after query error")
	}
}

func TestSemanticAdapter_FilterToolRefsDropped(t *testing.T) {
	t.Parallel()
	c := newTestClient(t, &ScriptedSidecar{
		OnRequest: func(req map[string]any) ScriptedResponse {
			op, _ := req["op"].(string)
			switch op {
			case opHealth:
				return ScriptedResponse{Response: healthOK(req)}
			case opQuery:
				return ScriptedResponse{Response: map[string]any{
					"id":   req["id"],
					"ok":   true,
					"hits": []any{},
				}}
			default:
				return ScriptedResponse{}
			}
		},
	})
	if h := c.Health(context.Background()); !h.OK {
		t.Fatalf("Health: %v", h.Err)
	}
	adapter := NewSemanticAdapter(c)
	// ToolRefs should be logged and dropped, not cause error.
	filter := search.Filter{ToolRefs: []string{"a", "b"}, ServerID: "srv"}
	hits, err := adapter.Query(context.Background(), "query", 3, filter)
	if err != nil {
		t.Fatalf("Query with ToolRefs should not error: %v", err)
	}
	_ = hits // may be empty but must not be nil-error
}

// TestSemanticAdapter_TypeCheck verifies the adapter satisfies
// search.Semantic. The primary check is the compile-time assertion in
// semantic.go; this test uses a fake client so the adapter can be
// constructed without panicking.
func TestSemanticAdapter_TypeCheck(t *testing.T) {
	t.Parallel()
	c := newTestClient(t, &ScriptedSidecar{
		OnRequest: func(req map[string]any) ScriptedResponse {
			return ScriptedResponse{Response: healthOK(req)}
		},
	})
	var _ search.Semantic = NewSemanticAdapter(c)
}
