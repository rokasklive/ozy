package broker

import (
	"context"
	"testing"

	"github.com/rokasklive/ozy/internal/catalog"
	"github.com/rokasklive/ozy/internal/config"
	"github.com/rokasklive/ozy/internal/contract"
	"github.com/rokasklive/ozy/internal/search"
)

// tieSemantic is a fake search.Semantic returning a fixed hit list. Listing the
// hits in the reverse of the lexical order makes the RRF legs offset exactly
// (lex rank 1 + sem rank 2 == lex rank 2 + sem rank 1), forcing a gap-0 tie so
// the broker takes the ambiguous branch deterministically.
type tieSemantic struct{ hits []search.SemanticHit }

func (t tieSemantic) Available() bool { return true }

func (t tieSemantic) Query(context.Context, string, int, search.Filter) ([]search.SemanticHit, error) {
	return t.hits, nil
}

func TestFindTool_AmbiguousCarriesNextAction(t *testing.T) {
	t.Parallel()
	store := catalog.NewMemory()
	tools := []catalog.Tool{
		{ToolRef: "atlassian.confluence_search", ServerID: "atlassian", DownstreamToolName: "confluence_search",
			Title: "Search Confluence wiki pages", Description: "Search Confluence wiki pages for internal documentation"},
		{ToolRef: "github.search_code", ServerID: "github", DownstreamToolName: "search_code",
			Title: "Search GitHub code", Description: "Find code across GitHub repositories"},
	}
	for _, tl := range tools {
		if err := store.PutTool(context.Background(), tl); err != nil {
			t.Fatalf("PutTool: %v", err)
		}
	}
	// Reverse of the expected lexical order (confluence leads lexically on the
	// query) so the fused scores tie exactly.
	sem := tieSemantic{hits: []search.SemanticHit{
		{ToolRef: "github.search_code", Score: 0.90},
		{ToolRef: "atlassian.confluence_search", Score: 0.85},
	}}
	b := NewLive(store, &config.Config{}, fakeConnector{}, search.New(store, sem))

	res, err := b.FindTool(context.Background(), "search wiki pages confluence")
	if err != nil {
		t.Fatalf("FindTool() error = %v", err)
	}
	if res.Decision != contract.DecisionAmbiguous {
		t.Fatalf("Decision = %q, want ambiguous", res.Decision)
	}
	if res.NextAction == nil {
		t.Fatal("ambiguous decision must carry a structured nextAction")
	}
	if res.NextAction.Tool != "describeTool" {
		t.Errorf("NextAction.Tool = %q, want describeTool", res.NextAction.Tool)
	}
	if res.NextAction.ToolRef == "" || res.NextAction.ToolRef != res.SelectedToolRef {
		t.Errorf("NextAction.ToolRef = %q, want selected %q", res.NextAction.ToolRef, res.SelectedToolRef)
	}
}

func TestFindTool_NoGoodMatchCarriesNextAction(t *testing.T) {
	t.Parallel()
	b := newBrokerWithTools(t, []catalog.Tool{
		{ToolRef: "slack.send_message", ServerID: "slack", DownstreamToolName: "send_message",
			Title: "Send Slack Message", Description: "Send a message to a Slack channel"},
	})

	res, err := b.FindTool(context.Background(), "xyzzy flarble glorph")
	if err != nil {
		t.Fatalf("FindTool() error = %v", err)
	}
	if res.Decision != contract.DecisionNoGoodMatch {
		t.Fatalf("Decision = %q, want no_good_match", res.Decision)
	}
	if res.NextAction == nil || res.NextAction.Tool != "findTool" {
		t.Errorf("no_good_match should carry a nextAction directing to findTool, got %+v", res.NextAction)
	}
}
