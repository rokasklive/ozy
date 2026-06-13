package broker

import (
	"context"
	"errors"
	"testing"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/rokasklive/ozy/internal/catalog"
	"github.com/rokasklive/ozy/internal/config"
	"github.com/rokasklive/ozy/internal/contract"
	"github.com/rokasklive/ozy/internal/downstream"
)

type fakeConnector struct {
	results []downstream.Result
}

func (f fakeConnector) ConnectAll(context.Context, *config.Config) []downstream.Result {
	return f.results
}

func (fakeConnector) Connect(_ context.Context, _ string, _ config.ServerConfig) downstream.Result {
	return downstream.Result{ServerID: "unused"}
}

type fakeSession struct {
	tools []*mcpsdk.Tool
	err   error
}

func (f fakeSession) ListTools(context.Context, *mcpsdk.ListToolsParams) (*mcpsdk.ListToolsResult, error) {
	return &mcpsdk.ListToolsResult{Tools: f.tools}, f.err
}

func (fakeSession) CallTool(context.Context, *mcpsdk.CallToolParams) (*mcpsdk.CallToolResult, error) {
	return &mcpsdk.CallToolResult{}, nil
}

func (fakeSession) Close() error { return nil }

type failingSession struct{}

func (failingSession) ListTools(context.Context, *mcpsdk.ListToolsParams) (*mcpsdk.ListToolsResult, error) {
	return nil, errors.New("tools/list internal error")
}

func (failingSession) CallTool(context.Context, *mcpsdk.CallToolParams) (*mcpsdk.CallToolResult, error) {
	return nil, errors.New("tools/call not implemented in failingSession")
}

func (failingSession) Close() error { return nil }

func newLiveBroker(t *testing.T, connector Connector) Broker {
	t.Helper()
	return NewLive(catalog.NewMemory(), &config.Config{}, connector)
}

func TestFindTool_LiveDiscoveryReturnsChooseFromCandidates(t *testing.T) {
	t.Parallel()
	b := newLiveBroker(t, fakeConnector{results: []downstream.Result{
		{
			ServerID: "atlassian",
			Session: fakeSession{tools: []*mcpsdk.Tool{
				{
					Name:        "confluence_search",
					Title:       "Confluence Search",
					Description: "Search Confluence wiki",
					InputSchema: map[string]any{
						"type":       "object",
						"properties": map[string]any{"query": map[string]any{"type": "string"}},
					},
				},
			}},
		},
		{
			ServerID: "filesystem",
			Session: fakeSession{tools: []*mcpsdk.Tool{
				{
					Name:        "read_file",
					Title:       "Read File",
					Description: "Read file contents",
				},
				{
					Name:        "write_file",
					Title:       "Write File",
					Description: "Write file contents",
				},
			}},
		},
	}})

	res, err := b.FindTool(context.Background(), "search")
	if err != nil {
		t.Fatalf("FindTool() unexpected error = %v", err)
	}
	if res.Decision != contract.DecisionChooseFromCandidates {
		t.Errorf("Decision = %q, want %q", res.Decision, contract.DecisionChooseFromCandidates)
	}
	if len(res.Candidates) != 3 {
		t.Errorf("Candidates len = %d, want 3", len(res.Candidates))
	}
	if res.AgentInstruction == "" {
		t.Error("result must carry an agentInstruction")
	}

	candidateRefs := make(map[string]bool)
	for _, c := range res.Candidates {
		candidateRefs[c.ToolRef] = true
		if c.ToolRef == "" || c.ServerID == "" || c.DownstreamToolName == "" {
			t.Errorf("candidate %+v missing required fields", c)
		}
	}
	if !candidateRefs["atlassian.confluence_search"] {
		t.Error("missing expected candidate atlassian.confluence_search")
	}
	if !candidateRefs["filesystem.read_file"] {
		t.Error("missing expected candidate filesystem.read_file")
	}
	if !candidateRefs["filesystem.write_file"] {
		t.Error("missing expected candidate filesystem.write_file")
	}
	if res.Candidates[0].InputSchema != nil {
		schema, ok := res.Candidates[0].InputSchema["type"]
		if !ok || schema != "object" {
			t.Errorf("input schema type = %v, want object", schema)
		}
	}
}

func TestFindTool_LiveDiscoveryZeroTools(t *testing.T) {
	t.Parallel()
	b := newLiveBroker(t, fakeConnector{results: []downstream.Result{
		{
			ServerID: "atlassian",
			Session:  fakeSession{},
		},
	}})

	res, err := b.FindTool(context.Background(), "search")
	if err != nil {
		t.Fatalf("FindTool() unexpected error = %v", err)
	}
	if res.Decision != contract.DecisionNoGoodMatch {
		t.Errorf("Decision = %q, want %q", res.Decision, contract.DecisionNoGoodMatch)
	}
	if res.AgentInstruction == "" {
		t.Error("zero-tools result must carry an agentInstruction")
	}
	if len(res.Candidates) != 0 {
		t.Errorf("Candidates len = %d, want 0", len(res.Candidates))
	}
}

func TestFindTool_PartialServerFailure(t *testing.T) {
	t.Parallel()
	b := newLiveBroker(t, fakeConnector{results: []downstream.Result{
		{
			ServerID: "atlassian",
			Session: fakeSession{tools: []*mcpsdk.Tool{
				{
					Name:        "confluence_search",
					Title:       "Confluence Search",
					Description: "Search Confluence",
				},
			}},
		},
		{
			ServerID: "broken-server",
			Error: &contract.Error{
				Type:             contract.ErrTypeDownstreamServerOffline,
				ServerID:         "broken-server",
				Retryable:        true,
				Message:          "could not connect to server \"broken-server\": connection refused",
				AgentInstruction: "Check connectivity.",
			},
		},
	}})

	res, err := b.FindTool(context.Background(), "search")
	if err != nil {
		t.Fatalf("FindTool() unexpected error = %v", err)
	}
	if res.Decision != contract.DecisionChooseFromCandidates {
		t.Errorf("Decision = %q, want %q", res.Decision, contract.DecisionChooseFromCandidates)
	}
	if len(res.Candidates) != 1 {
		t.Errorf("Candidates len = %d, want 1", len(res.Candidates))
	}
	if len(res.Errors) != 1 {
		t.Errorf("Errors len = %d, want 1", len(res.Errors))
	}
	if res.Errors[0].ServerID != "broken-server" {
		t.Errorf("Error ServerID = %q, want broken-server", res.Errors[0].ServerID)
	}
	if res.AgentInstruction == "" {
		t.Error("partial-failure result must carry an agentInstruction")
	}
	if !contains(res.AgentInstruction, "partial") {
		t.Errorf("agentInstruction should mention partial: %s", res.AgentInstruction)
	}
}

func TestFindTool_TotalDownstreamFailure(t *testing.T) {
	t.Parallel()
	b := newLiveBroker(t, fakeConnector{results: []downstream.Result{
		{
			ServerID: "server-a",
			Error: &contract.Error{
				Type:             contract.ErrTypeDownstreamServerOffline,
				ServerID:         "server-a",
				Retryable:        true,
				Message:          "could not connect to server \"server-a\": connection refused",
				AgentInstruction: "Check connectivity.",
			},
		},
		{
			ServerID: "server-b",
			Error: &contract.Error{
				Type:             contract.ErrTypeAuthUnavailable,
				ServerID:         "server-b",
				Retryable:        false,
				Message:          "oauth authentication is required but Ozy does not implement OAuth flow",
				AgentInstruction: "Ask the user to configure a non-OAuth credential path.",
			},
		},
	}})

	res, err := b.FindTool(context.Background(), "search")
	if err != nil {
		t.Fatalf("FindTool() unexpected error = %v", err)
	}
	if res.Decision != contract.DecisionKnownButUnavailable {
		t.Errorf("Decision = %q, want %q", res.Decision, contract.DecisionKnownButUnavailable)
	}
	if len(res.Errors) != 2 {
		t.Errorf("Errors len = %d, want 2", len(res.Errors))
	}
	if len(res.Candidates) != 0 {
		t.Errorf("Candidates len = %d, want 0", len(res.Candidates))
	}
	if res.AgentInstruction == "" {
		t.Error("total-failure result must carry an agentInstruction")
	}
}

func TestFindTool_NoConfiguredServers(t *testing.T) {
	t.Parallel()
	b := newLiveBroker(t, fakeConnector{results: nil})

	res, err := b.FindTool(context.Background(), "search")
	if err != nil {
		t.Fatalf("FindTool() unexpected error = %v", err)
	}
	if res.Decision != contract.DecisionCatalogEmpty {
		t.Errorf("Decision = %q, want %q", res.Decision, contract.DecisionCatalogEmpty)
	}
}

func TestFindTool_DisabledServersAreSkipped(t *testing.T) {
	t.Parallel()
	b := newLiveBroker(t, fakeConnector{results: []downstream.Result{
		{
			ServerID: "disabled-server",
			Skipped:  true,
		},
	}})

	res, err := b.FindTool(context.Background(), "search")
	if err != nil {
		t.Fatalf("FindTool() unexpected error = %v", err)
	}
	if res.Decision != contract.DecisionNoGoodMatch {
		t.Errorf("Decision = %q, want %q (all servers skipped — no enabled servers)", res.Decision, contract.DecisionNoGoodMatch)
	}
}

func TestFindTool_SessionListToolsFails(t *testing.T) {
	t.Parallel()
	b := newLiveBroker(t, fakeConnector{results: []downstream.Result{
		{
			ServerID: "atlassian",
			Session:  failingSession{},
		},
	}})

	res, err := b.FindTool(context.Background(), "search")
	if err != nil {
		t.Fatalf("FindTool() unexpected error = %v", err)
	}
	if res.Decision != contract.DecisionKnownButUnavailable {
		t.Errorf("Decision = %q, want %q", res.Decision, contract.DecisionKnownButUnavailable)
	}
	if len(res.Errors) != 1 {
		t.Errorf("Errors len = %d, want 1", len(res.Errors))
	}
	if res.Errors[0].Type != contract.ErrTypeDownstreamCallFailed {
		t.Errorf("Error type = %q, want %q", res.Errors[0].Type, contract.ErrTypeDownstreamCallFailed)
	}
}

func TestLiveBroker_PreservesDescribeCallPlaceholders(t *testing.T) {
	t.Parallel()
	b := newLiveBroker(t, fakeConnector{})

	_, err := b.DescribeTool(context.Background(), "atlassian.missing")
	var ce *contract.Error
	if !errors.As(err, &ce) {
		t.Fatalf("DescribeTool() error = %v, want *contract.Error", err)
	}
	if ce.Type != contract.ErrTypeToolNotFound {
		t.Errorf("error type = %q, want %q", ce.Type, contract.ErrTypeToolNotFound)
	}
}

func TestLiveBroker_CallToolUnknownServerReturnsConfigError(t *testing.T) {
	t.Parallel()
	b := newLiveBroker(t, fakeConnector{})

	_, err := b.CallTool(context.Background(), "atlassian.missing", nil)
	var ce *contract.Error
	if !errors.As(err, &ce) {
		t.Fatalf("CallTool() error = %v, want *contract.Error", err)
	}
	if ce.Type != contract.ErrTypeConfigError {
		t.Errorf("error type = %q, want %q", ce.Type, contract.ErrTypeConfigError)
	}
	if ce.ServerID != "atlassian" {
		t.Errorf("ServerID = %q, want atlassian", ce.ServerID)
	}
	if ce.AgentInstruction == "" {
		t.Error("structured failure must carry an agentInstruction")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchSubstring(s, substr)
}

func searchSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
