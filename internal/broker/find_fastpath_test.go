package broker

import (
	"context"
	"strings"
	"testing"

	"github.com/rokasklive/ozy/internal/catalog"
	"github.com/rokasklive/ozy/internal/config"
	"github.com/rokasklive/ozy/internal/contract"
	"github.com/rokasklive/ozy/internal/search"
)

func smallSchema() map[string]any {
	return map[string]any{
		"type":       "object",
		"required":   []any{"query"},
		"properties": map[string]any{"query": map[string]any{"type": "string", "description": "search query"}},
	}
}

func hugeSchema() map[string]any {
	props := map[string]any{}
	for r := 'a'; r <= 'z'; r++ {
		props[strings.Repeat(string(r), 8)] = map[string]any{
			"type":        "string",
			"description": strings.Repeat("padding words for size ", 8),
		}
	}
	return map[string]any{"type": "object", "properties": props}
}

func findBroker(t *testing.T, cfg *config.Config, tools ...catalog.Tool) Broker {
	t.Helper()
	store := catalog.NewMemory()
	for _, tl := range tools {
		if err := store.PutTool(context.Background(), tl); err != nil {
			t.Fatalf("PutTool: %v", err)
		}
	}
	return NewLive(store, cfg, fakeConnector{}, search.New(store, nil))
}

func TestFindTool_SmallSchemaFastPathSkipsDescribe(t *testing.T) {
	t.Parallel()
	b := findBroker(t, &config.Config{},
		catalog.Tool{ToolRef: "web.search", ServerID: "web", DownstreamToolName: "search",
			Title: "Web search", Description: "Search the web", InputSchema: smallSchema()},
		catalog.Tool{ToolRef: "mail.send", ServerID: "mail", DownstreamToolName: "send",
			Title: "Send mail", Description: "Send an email message"},
	)

	res, err := b.FindTool(context.Background(), "web search")
	if err != nil {
		t.Fatalf("FindTool: %v", err)
	}
	if res.Decision != contract.DecisionUse {
		t.Fatalf("Decision = %q, want use", res.Decision)
	}
	sel := res.Selected
	if sel == nil || sel.InputSchema == nil {
		t.Fatalf("small schema must be inlined, got %+v", sel)
	}
	if sel.SchemaPreview != nil {
		t.Error("inlined schema supersedes the preview; both is duplicate bytes")
	}
	if sel.RecommendedCall == nil || sel.RecommendedCall.Tool != "callTool" {
		t.Fatalf("fast path must carry a recommendedCall, got %+v", sel.RecommendedCall)
	}
	args, _ := sel.RecommendedCall.Arguments["arguments"].(map[string]any)
	if args["query"] != "<string>" {
		t.Errorf("argument skeleton = %v, want typed placeholder for required field", sel.RecommendedCall.Arguments)
	}
	if res.NextAction == nil || res.NextAction.Tool != "callTool" {
		t.Fatalf("fast path nextAction must be callTool, got %+v", res.NextAction)
	}
	if !strings.Contains(res.AgentInstruction, "callTool directly") {
		t.Errorf("instruction should say to call directly, got %q", res.AgentInstruction)
	}
}

func TestFindTool_LargeSchemaKeepsDescribeFirst(t *testing.T) {
	t.Parallel()
	b := findBroker(t, &config.Config{},
		catalog.Tool{ToolRef: "web.search", ServerID: "web", DownstreamToolName: "search",
			Title: "Web search", Description: "Search the web", InputSchema: hugeSchema()},
		catalog.Tool{ToolRef: "mail.send", ServerID: "mail", DownstreamToolName: "send",
			Title: "Send mail", Description: "Send an email message"},
	)

	res, err := b.FindTool(context.Background(), "web search")
	if err != nil {
		t.Fatalf("FindTool: %v", err)
	}
	if res.Decision != contract.DecisionUse {
		t.Fatalf("Decision = %q, want use", res.Decision)
	}
	if res.Selected.InputSchema != nil {
		t.Error("oversized schema must not be inlined")
	}
	if res.Selected.SchemaPreview == nil {
		t.Error("large-schema selection keeps the bounded preview")
	}
	if res.NextAction == nil || res.NextAction.Tool != "describeTool" {
		t.Fatalf("large-schema nextAction must stay describeTool, got %+v", res.NextAction)
	}
}

func TestFindTool_MaxResultsBoundsAlternatives(t *testing.T) {
	t.Parallel()
	// One clear winner ("fleet" is unique to it and in its title), several
	// database-adjacent runner-ups, and unrelated fillers — so the component
	// floor passes and the ranking still has plenty of alternatives to bound.
	tools := []catalog.Tool{
		{ToolRef: "db.alpha", ServerID: "db", DownstreamToolName: "alpha",
			Title: "Fleet database query", Description: "Query the fleet database directly with SQL"},
		{ToolRef: "db.beta", ServerID: "db", DownstreamToolName: "beta",
			Title: "Database backup", Description: "Back up a database volume"},
		{ToolRef: "db.gamma", ServerID: "db", DownstreamToolName: "gamma",
			Title: "Database migrations", Description: "Run database migrations"},
		{ToolRef: "mail.delta", ServerID: "mail", DownstreamToolName: "delta",
			Title: "Send mail", Description: "Send an email message"},
		{ToolRef: "fs.epsilon", ServerID: "fs", DownstreamToolName: "epsilon",
			Title: "Read file", Description: "Read a file from disk"},
		{ToolRef: "ci.zeta", ServerID: "ci", DownstreamToolName: "zeta",
			Title: "Trigger build", Description: "Trigger a CI build"},
	}
	cfg := &config.Config{Budgets: config.BudgetsConfig{FindTool: config.FindToolBudget{MaxResults: 3}}}
	b := findBroker(t, cfg, tools...)

	res, err := b.FindTool(context.Background(), "query the fleet database")
	if err != nil {
		t.Fatalf("FindTool: %v", err)
	}
	if res.SelectedToolRef == "" {
		t.Fatalf("expected a selection, got decision %q", res.Decision)
	}
	if len(res.Alternatives) != 2 {
		t.Fatalf("alternatives = %d, want maxResults-1 = 2", len(res.Alternatives))
	}
	for _, alt := range res.Alternatives {
		if alt.ToolRef == "" || alt.Reason == "" {
			t.Fatalf("every alternative carries a toolRef and reason, got %+v", alt)
		}
	}
}
