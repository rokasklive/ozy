package broker

import (
	"context"
	"errors"
	"strings"
	"testing"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/rokasklive/ozy/internal/catalog"
	"github.com/rokasklive/ozy/internal/config"
	"github.com/rokasklive/ozy/internal/contract"
	"github.com/rokasklive/ozy/internal/downstream"
	"github.com/rokasklive/ozy/internal/search"
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
	store := catalog.NewMemory()
	return NewLive(store, &config.Config{}, connector, search.New(store, nil))
}

func newBrokerWithTools(t *testing.T, tools []catalog.Tool) Broker {
	t.Helper()
	store := catalog.NewMemory()
	for _, tool := range tools {
		if err := store.PutTool(context.Background(), tool); err != nil {
			t.Fatalf("PutTool(%s) = %v", tool.ToolRef, err)
		}
	}
	return NewLive(store, &config.Config{}, fakeConnector{}, search.New(store, nil))
}

func TestFindTool_UsesTopMatchWithRunnerUp(t *testing.T) {
	t.Parallel()
	b := newBrokerWithTools(t, []catalog.Tool{
		{
			ToolRef:            "atlassian.confluence_search",
			ServerID:           "atlassian",
			DownstreamToolName: "confluence_search",
			Title:              "Search Confluence",
			Description:        "Search Confluence wiki pages for internal documentation",
			ServerStatus:       catalog.ServerOnline,
			CallableNow:        true,
		},
		{
			ToolRef:            "github.search_code",
			ServerID:           "github",
			DownstreamToolName: "search_code",
			Title:              "Search GitHub Code",
			Description:        "Find code across GitHub repositories",
			ServerStatus:       catalog.ServerOnline,
			CallableNow:        true,
		},
	})

	res, err := b.FindTool(context.Background(), "search confluence wiki")
	if err != nil {
		t.Fatalf("FindTool() error = %v", err)
	}
	if res.Decision != contract.DecisionUse {
		t.Fatalf("Decision = %q, want use", res.Decision)
	}
	if res.SelectedToolRef != "atlassian.confluence_search" {
		t.Errorf("SelectedToolRef = %q, want atlassian.confluence_search", res.SelectedToolRef)
	}
	if res.Selected == nil {
		t.Fatal("Selected is nil")
	}
	if res.Selected.ToolRef != "atlassian.confluence_search" {
		t.Errorf("Selected.ToolRef = %q", res.Selected.ToolRef)
	}
	if res.Confidence == "" {
		t.Error("Confidence should be set")
	}
	if res.Reason == "" {
		t.Error("Reason should be set")
	}
	if len(res.Alternatives) != 1 {
		t.Errorf("Alternatives len = %d, want 1 (exactly one runner-up)", len(res.Alternatives))
	}
	if res.NextAction == nil || res.NextAction.Tool != "describeTool" {
		t.Error("NextAction should direct to describeTool")
	}
}

func TestFindTool_CatalogEmpty(t *testing.T) {
	t.Parallel()
	store := catalog.NewMemory()
	b := NewLive(store, &config.Config{}, fakeConnector{}, search.New(store, nil))

	res, err := b.FindTool(context.Background(), "anything")
	if err != nil {
		t.Fatalf("FindTool() error = %v", err)
	}
	if res.Decision != contract.DecisionCatalogEmpty {
		t.Errorf("Decision = %q, want catalog_empty", res.Decision)
	}
	if res.AgentInstruction == "" {
		t.Error("must have instructional agentInstruction")
	}
}

func TestFindTool_NoGoodMatch(t *testing.T) {
	t.Parallel()
	b := newBrokerWithTools(t, []catalog.Tool{
		{
			ToolRef:            "slack.send_message",
			ServerID:           "slack",
			DownstreamToolName: "send_message",
			Title:              "Send Slack Message",
			Description:        "Send a message to a Slack channel",
		},
	})

	res, err := b.FindTool(context.Background(), "xyzzy flarble glorph")
	if err != nil {
		t.Fatalf("FindTool() error = %v", err)
	}
	if res.Decision != contract.DecisionNoGoodMatch {
		t.Errorf("Decision = %q, want no_good_match", res.Decision)
	}
}

func TestFindTool_ReasonNamesMatchedBasis(t *testing.T) {
	t.Parallel()
	b := newBrokerWithTools(t, []catalog.Tool{
		{
			ToolRef:            "atlassian.confluence_search",
			ServerID:           "atlassian",
			DownstreamToolName: "confluence_search",
			Title:              "Search Confluence",
			Description:        "Search Confluence wiki pages",
		},
	})

	res, err := b.FindTool(context.Background(), "confluence wiki search")
	if err != nil {
		t.Fatalf("FindTool() error = %v", err)
	}
	if res.Decision == contract.DecisionUse {
		// Reason must name what matched (terms, fields), not just echo the query.
		if res.Reason == "confluence wiki search" {
			t.Errorf("reason %q echoes the query, should name matched terms", res.Reason)
		}
		if !strings.Contains(strings.ToLower(res.Reason), "match") && !strings.Contains(strings.ToLower(res.Reason), "term") {
			t.Logf("reason: %s (should contain match/term info)", res.Reason)
		}
	}
}

func TestFindTool_OfflineToolStillDiscoverable(t *testing.T) {
	t.Parallel()
	b := newBrokerWithTools(t, []catalog.Tool{
		{
			ToolRef:            "atlassian.confluence_search",
			ServerID:           "atlassian",
			DownstreamToolName: "confluence_search",
			Title:              "Search Confluence",
			Description:        "Search Confluence wiki",
			ServerStatus:       catalog.ServerOffline,
			CallableNow:        false,
		},
	})

	res, err := b.FindTool(context.Background(), "confluence search")
	if err != nil {
		t.Fatalf("FindTool() error = %v", err)
	}
	if res.Decision != contract.DecisionUse {
		t.Errorf("Decision = %q, want use (catalog-backed is stale-tolerant)", res.Decision)
	}
	if res.Selected != nil && res.Selected.CallableNow {
		t.Error("offline tool should have CallableNow=false")
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
