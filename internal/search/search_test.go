package search

import (
	"context"
	"strings"
	"testing"

	"github.com/rokasklive/ozy/internal/catalog"
)

// storeWithTools returns an in-memory store pre-populated with test tools.
func storeWithTools(t *testing.T, tools []catalog.Tool) catalog.Store {
	t.Helper()
	store := catalog.NewMemory()
	for _, tool := range tools {
		if err := store.PutTool(context.Background(), tool); err != nil {
			t.Fatalf("PutTool(%s) = %v", tool.ToolRef, err)
		}
	}
	return store
}

func TestEngine_Find_RankingOrdersByLexicalRelevance(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := storeWithTools(t, []catalog.Tool{
		{
			ToolRef:            "jira.search_issues",
			ServerID:           "jira",
			DownstreamToolName: "search_issues",
			Title:              "Search Jira Issues",
			Description:        "Search for issues in Jira using JQL",
		},
		{
			ToolRef:            "atlassian.confluence_search",
			ServerID:           "atlassian",
			DownstreamToolName: "confluence_search",
			Title:              "Search Confluence",
			Description:        "Search Confluence wiki pages for internal documentation",
		},
		{
			ToolRef:            "github.search_code",
			ServerID:           "github",
			DownstreamToolName: "search_code",
			Title:              "Search GitHub Code",
			Description:        "Find code across GitHub repositories",
		},
		{
			ToolRef:            "slack.send_message",
			ServerID:           "slack",
			DownstreamToolName: "send_message",
			Title:              "Send Slack Message",
			Description:        "Send a message to a Slack channel",
		},
	})

	engine := New(store, nil)
	ranking, err := engine.Find(ctx, "search confluence wiki")
	if err != nil {
		t.Fatalf("Find() error = %v", err)
	}
	if len(ranking.Entries) < 3 {
		t.Fatalf("Find() returned %d entries, want at least 3", len(ranking.Entries))
	}

	// confluence should rank first.
	top := ranking.Entries[0]
	if !strings.Contains(top.Tool.ToolRef, "confluence") {
		t.Errorf("top entry = %s, want confluence_search", top.Tool.ToolRef)
	}
	if top.LexicalScore <= 0 {
		t.Errorf("top lexical score = %v, want > 0", top.LexicalScore)
	}

	// jira should rank below confluence for this query.
	foundConfluence := false
	foundJira := false
	for i, e := range ranking.Entries {
		if strings.Contains(e.Tool.ToolRef, "confluence") {
			foundConfluence = true
			if i > 0 && foundJira {
				t.Errorf("confluence ranked at %d, but jira appeared before it", i)
			}
		}
		if strings.Contains(e.Tool.ToolRef, "jira") {
			foundJira = true
		}
	}
	if !foundConfluence {
		t.Error("confluence tool not in results")
	}
}

func TestEngine_Find_IncludesMatchedTermsAndFields(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := storeWithTools(t, []catalog.Tool{
		{
			ToolRef:            "atlassian.confluence_search",
			ServerID:           "atlassian",
			DownstreamToolName: "confluence_search",
			Title:              "Search Confluence",
			Description:        "Search Confluence wiki pages for internal documentation",
			CapabilityText:     []string{"wiki", "confluence"},
		},
	})

	engine := New(store, nil)
	ranking, err := engine.Find(ctx, "search confluence")
	if err != nil {
		t.Fatalf("Find() error = %v", err)
	}
	if len(ranking.Entries) == 0 {
		t.Fatal("Find() returned no entries")
	}

	e := ranking.Entries[0]
	if len(e.MatchedTerms) == 0 {
		t.Error("matched terms empty, want at least one matched term")
	}
	if len(e.TopContributingFields) == 0 {
		t.Error("top contributing fields empty")
	}
	if e.Reason == "" {
		t.Error("reason empty")
	}
	// Reason must name matched basis, not just echo query.
	if strings.ToLower(e.Reason) == "search confluence" {
		t.Errorf("reason %q echoes the query, should name matched terms", e.Reason)
	}
}

func TestEngine_Find_EmptyCatalogReturnsNoEntries(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := catalog.NewMemory()
	engine := New(store, nil)
	ranking, err := engine.Find(ctx, "anything")
	if err != nil {
		t.Fatalf("Find() error = %v", err)
	}
	if len(ranking.Entries) != 0 {
		t.Errorf("entries = %d, want 0 for empty catalog", len(ranking.Entries))
	}
}

func TestEngine_Find_FieldBoostsToolRefOverDescription(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := storeWithTools(t, []catalog.Tool{
		{
			ToolRef:            "xyz.obscure",
			ServerID:           "xyz",
			DownstreamToolName: "obscure",
			Title:              "Widget",
			Description:        "keywordmatch here",
		},
		{
			ToolRef:            "keywordmatch.service",
			ServerID:           "svc",
			DownstreamToolName: "service",
			Title:              "Something",
			Description:        "other things",
		},
	})

	engine := New(store, nil)
	ranking, err := engine.Find(ctx, "keywordmatch")
	if err != nil {
		t.Fatalf("Find() error = %v", err)
	}
	if len(ranking.Entries) < 2 {
		t.Fatalf("entries = %d, want at least 2", len(ranking.Entries))
	}
	// toolRef match should outrank description-only match.
	top := ranking.Entries[0]
	if top.Tool.ToolRef != "keywordmatch.service" {
		t.Errorf("top toolRef = %s, want keywordmatch.service (toolRef boost > description)", top.Tool.ToolRef)
	}
}

func TestEngine_Find_ScoresAreDifferentForDifferentQueries(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := storeWithTools(t, []catalog.Tool{
		{
			ToolRef:            "github.search_code",
			ServerID:           "github",
			DownstreamToolName: "search_code",
			Title:              "GitHub Code Search",
			Description:        "Find code across GitHub repositories",
		},
		{
			ToolRef:            "slack.send_message",
			ServerID:           "slack",
			DownstreamToolName: "send_message",
			Title:              "Send Message",
			Description:        "Send slack messages",
		},
	})

	engine := New(store, nil)

	r1, _ := engine.Find(ctx, "github code search")
	r2, _ := engine.Find(ctx, "slack message")

	if len(r1.Entries) == 0 || len(r2.Entries) == 0 {
		t.Fatal("empty results")
	}

	// github should rank first for github query.
	if !strings.Contains(strings.ToLower(r1.Entries[0].Tool.ToolRef), "github") {
		t.Errorf("top for github query = %s, want github", r1.Entries[0].Tool.ToolRef)
	}
	// slack should rank first for slack query.
	if !strings.Contains(strings.ToLower(r2.Entries[0].Tool.ToolRef), "slack") {
		t.Errorf("top for slack query = %s, want slack", r2.Entries[0].Tool.ToolRef)
	}
}

// Fake semantic scorer for testing decision bands.
type fakeSemantic struct {
	scores    []float64
	available bool
}

func (f *fakeSemantic) Score(_ context.Context, _ string, _ []catalog.Tool) ([]float64, error) {
	return f.scores, nil
}

func (f *fakeSemantic) Available() bool { return f.available }

func TestDecide_UseDecision(t *testing.T) {
	t.Parallel()
	tools := []catalog.Tool{
		{ToolRef: "a.tool1", Title: "Best Match", Description: "matches perfectly"},
		{ToolRef: "b.tool2", Title: "Runner Up", Description: "somewhat related"},
	}
	entries := []RankedEntry{
		{Tool: tools[0], LexicalScore: 80, FusedScore: 0.9, Reason: "matched perfectly"},
		{Tool: tools[1], LexicalScore: 10, FusedScore: 0.3, Reason: "somewhat related"},
	}
	ranking := &Ranking{Entries: entries}
	d := Decide(ranking)
	if d.Verdict != DecisionUse {
		t.Fatalf("verdict = %s, want use", d.Verdict)
	}
	if d.Selected == nil {
		t.Fatal("selected is nil")
	}
	if d.Selected.Tool.ToolRef != "a.tool1" {
		t.Errorf("selected = %s, want a.tool1", d.Selected.Tool.ToolRef)
	}
	if d.RunnerUp == nil {
		t.Fatal("runner-up is nil")
	}
	if d.RunnerUp.Tool.ToolRef != "b.tool2" {
		t.Errorf("runner-up = %s, want b.tool2", d.RunnerUp.Tool.ToolRef)
	}
	if d.Confidence == "" {
		t.Error("confidence should be set")
	}
}

func TestDecide_AmbiguousDecision(t *testing.T) {
	t.Parallel()
	tools := []catalog.Tool{
		{ToolRef: "a.tool1", Title: "First"},
		{ToolRef: "b.tool2", Title: "Second"},
	}
	entries := []RankedEntry{
		{Tool: tools[0], LexicalScore: 30, FusedScore: 0.4, Reason: "match a"},
		{Tool: tools[1], LexicalScore: 28, FusedScore: 0.38, Reason: "match b"},
	}
	ranking := &Ranking{Entries: entries}
	d := Decide(ranking)
	if d.Verdict != DecisionAmbiguous {
		t.Fatalf("verdict = %s, want ambiguous", d.Verdict)
	}
	if d.Selected == nil || d.RunnerUp == nil {
		t.Fatal("both selected and runner-up must be set for ambiguous")
	}
}

func TestDecide_NoGoodMatchDecision(t *testing.T) {
	t.Parallel()
	tools := []catalog.Tool{
		{ToolRef: "a.tool1", Title: "Some Tool"},
	}
	entries := []RankedEntry{
		{Tool: tools[0], LexicalScore: 1, FusedScore: 0.1, Reason: "weak"},
	}
	ranking := &Ranking{Entries: entries}
	d := Decide(ranking)
	if d.Verdict != DecisionNoGoodMatch {
		t.Fatalf("verdict = %s, want no_good_match", d.Verdict)
	}
}

func TestDecide_CatalogEmptyDecision(t *testing.T) {
	t.Parallel()
	ranking := &Ranking{Entries: nil}
	d := Decide(ranking)
	if d.Verdict != DecisionCatalogEmpty {
		t.Fatalf("verdict = %s, want catalog_empty", d.Verdict)
	}
}

func TestFuseScores(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		lex    float64
		sem    float64
		hasSem bool
		want   float64
	}{
		{"lex only high", 80, 0, false, normalizeLexical(80)},
		{"lex only low", 2, 0, false, normalizeLexical(2)},
		{"both strong", 80, 0.9, true, DefaultLexWeight*normalizeLexical(80) + DefaultSemWeight*normalizeSemantic(0.9)},
		{"lex weak sem strong", 2, 0.8, true, DefaultLexWeight*normalizeLexical(2) + DefaultSemWeight*normalizeSemantic(0.8)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := fuseScores(tt.lex, tt.sem, tt.hasSem)
			if got != tt.want {
				t.Errorf("fuseScores(%v, %v, %v) = %v, want %v", tt.lex, tt.sem, tt.hasSem, got, tt.want)
			}
		})
	}
}

func TestNormalizeLexical(t *testing.T) {
	t.Parallel()
	if got := normalizeLexical(0); got != 0 {
		t.Errorf("normalizeLexical(0) = %v, want 0", got)
	}
	if got := normalizeLexical(LexSatK); got != 0.5 {
		t.Errorf("normalizeLexical(k=%v) = %v, want 0.5", LexSatK, got)
	}
	if got := normalizeLexical(-1); got != 0 {
		t.Errorf("normalizeLexical(-1) = %v, want 0", got)
	}
}

func TestNormalizeSemantic(t *testing.T) {
	t.Parallel()
	if got := normalizeSemantic(1); got != 1 {
		t.Errorf("normalizeSemantic(1) = %v, want 1", got)
	}
	if got := normalizeSemantic(0); got != 0.5 {
		t.Errorf("normalizeSemantic(0) = %v, want 0.5", got)
	}
	if got := normalizeSemantic(-1); got != 0 {
		t.Errorf("normalizeSemantic(-1) = %v, want 0", got)
	}
}

func TestDecide_UseWithHighConfidence(t *testing.T) {
	t.Parallel()
	tools := []catalog.Tool{
		{ToolRef: "a.tool1", Title: "Perfect Match"},
		{ToolRef: "b.tool2", Title: "Weak"},
	}
	entries := []RankedEntry{
		{Tool: tools[0], LexicalScore: 200, FusedScore: 0.95, Reason: "strong match"},
		{Tool: tools[1], LexicalScore: 1, FusedScore: 0.1, Reason: "weak"},
	}
	d := Decide(&Ranking{Entries: entries})
	if d.Verdict != DecisionUse {
		t.Fatalf("verdict = %s, want use", d.Verdict)
	}
	if d.Confidence != "high" {
		t.Errorf("confidence = %s, want high (score 0.95 >= 0.6)", d.Confidence)
	}
}

func TestFormatDecision(t *testing.T) {
	t.Parallel()
	tests := []struct {
		verdict  string
		query    string
		contains string
	}{
		{DecisionUse, "search wiki", "Best indexed tool"},
		{DecisionAmbiguous, "search", "too close"},
		{DecisionNoGoodMatch, "xyz", "No indexed tool"},
		{DecisionCatalogEmpty, "", "no indexed tools"},
	}
	for _, tt := range tests {
		t.Run(tt.verdict, func(t *testing.T) {
			d := &Decision{Verdict: tt.verdict, Selected: &RankedEntry{}, RunnerUp: &RankedEntry{}}
			got := formatDecision(d, tt.query)
			if !strings.Contains(strings.ToLower(got), strings.ToLower(tt.contains)) {
				t.Errorf("formatDecision() = %q, want containing %q", got, tt.contains)
			}
		})
	}
}

func TestSortedEntries(t *testing.T) {
	t.Parallel()
	entries := []RankedEntry{
		{Tool: catalog.Tool{ToolRef: "c"}, FusedScore: 0.1},
		{Tool: catalog.Tool{ToolRef: "a"}, FusedScore: 0.9},
		{Tool: catalog.Tool{ToolRef: "b"}, FusedScore: 0.5},
	}
	sorted := sortedEntries(entries)
	if len(sorted) != 3 {
		t.Fatalf("len = %d, want 3", len(sorted))
	}
	if sorted[0].Tool.ToolRef != "a" || sorted[1].Tool.ToolRef != "b" || sorted[2].Tool.ToolRef != "c" {
		t.Errorf("sorted order = %v, want [a b c]", toolRefs(sorted))
	}
}

func toolRefs(entries []RankedEntry) []string {
	out := make([]string, len(entries))
	for i, e := range entries {
		out[i] = e.Tool.ToolRef
	}
	return out
}
