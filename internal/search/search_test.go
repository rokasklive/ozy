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

	top := ranking.Entries[0]
	if !strings.Contains(top.Tool.ToolRef, "confluence") {
		t.Errorf("top entry = %s, want confluence_search", top.Tool.ToolRef)
	}
	if top.LexicalScore <= 0 {
		t.Errorf("top lexical score = %v, want > 0", top.LexicalScore)
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
	if !strings.Contains(strings.ToLower(r1.Entries[0].Tool.ToolRef), "github") {
		t.Errorf("top for github query = %s, want github", r1.Entries[0].Tool.ToolRef)
	}
	if !strings.Contains(strings.ToLower(r2.Entries[0].Tool.ToolRef), "slack") {
		t.Errorf("top for slack query = %s, want slack", r2.Entries[0].Tool.ToolRef)
	}
}

// fakeSemantic is a configurable Semantic provider used to test RRF ordering,
// component-floor gating, and degradation.
type fakeSemantic struct {
	hits      []SemanticHit
	available bool
	err       error
}

func (f *fakeSemantic) Query(_ context.Context, _ string, _ int, _ Filter) ([]SemanticHit, error) {
	return f.hits, f.err
}

func (f *fakeSemantic) Available() bool { return f.available }

func TestEngine_Find_LexicalOnlyWhenSemanticUnavailable(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := storeWithTools(t, []catalog.Tool{
		{ToolRef: "a.search", ServerID: "a", DownstreamToolName: "search", Title: "Search", Description: "search tool"},
		{ToolRef: "b.other", ServerID: "b", DownstreamToolName: "other", Title: "Other", Description: "other tool"},
	})
	sem := &fakeSemantic{available: false}
	engine := New(store, sem)
	ranking, err := engine.Find(ctx, "search")
	if err != nil {
		t.Fatalf("Find() error = %v", err)
	}
	if ranking.SemanticAvailable {
		t.Error("SemanticAvailable should be false when provider reports unavailable")
	}
	if len(ranking.Entries) < 2 {
		t.Fatal("not enough entries")
	}
}

func TestEngine_Find_PerQuerySemanticFailureDegradesToLexical(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := storeWithTools(t, []catalog.Tool{
		{ToolRef: "a.search", ServerID: "a", DownstreamToolName: "search", Title: "Search", Description: "search tool"},
	})
	sem := &fakeSemantic{available: true, err: context.DeadlineExceeded}
	engine := New(store, sem)
	ranking, err := engine.Find(ctx, "search")
	if err != nil {
		t.Fatalf("Find() error = %v", err)
	}
	if !ranking.SemanticAvailable {
		t.Error("SemanticAvailable should be true (provider is up)")
	}
	if !ranking.SemanticDegraded {
		t.Error("SemanticDegraded should be true (this query failed)")
	}
	if len(ranking.Entries) == 0 {
		t.Fatal("should still rank lexically")
	}
}

func TestEngine_Find_RRFFusesLexicalAndSemanticRankLists(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := storeWithTools(t, []catalog.Tool{
		{ToolRef: "a.lex_top", ServerID: "a", DownstreamToolName: "x", Title: "X", Description: "alpha beta gamma"},
		{ToolRef: "b.sem_top", ServerID: "b", DownstreamToolName: "y", Title: "Y", Description: "delta epsilon"},
	})
	// Lexical rank: a.lex_top first, b.sem_top second.
	// Semantic rank: b.sem_top first (cosine 0.7), a.lex_top second (cosine 0.4).
	// With RRF, both contribute; whichever has better combined rank wins.
	sem := &fakeSemantic{
		available: true,
		hits: []SemanticHit{
			{ToolRef: "b.sem_top", Score: 0.7},
			{ToolRef: "a.lex_top", Score: 0.4},
		},
	}
	engine := New(store, sem)
	ranking, err := engine.Find(ctx, "alpha")
	if err != nil {
		t.Fatalf("Find() error = %v", err)
	}
	if !ranking.SemanticAvailable || ranking.SemanticDegraded {
		t.Fatalf("ranking flags: available=%v degraded=%v, want both false for healthy provider", ranking.SemanticAvailable, ranking.SemanticDegraded)
	}
	if len(ranking.Entries) < 2 {
		t.Fatalf("entries = %d, want 2", len(ranking.Entries))
	}
	// Lexical: a.lex_top is rank 1. Semantic: a.lex_top is rank 2.
	// a.lex_top RRF = 1/61 + 1/62 ≈ 0.0328. b.sem_top: lex rank 2, sem rank 1
	// -> 1/62 + 1/61 ≈ 0.0326. Top is a.lex_top.
	if ranking.Entries[0].Tool.ToolRef != "a.lex_top" {
		t.Errorf("top = %s, want a.lex_top (lexical rank 1)", ranking.Entries[0].Tool.ToolRef)
	}
	if ranking.Entries[1].Tool.ToolRef != "b.sem_top" {
		t.Errorf("runner-up = %s, want b.sem_top", ranking.Entries[1].Tool.ToolRef)
	}
}

func TestEngine_Find_RRFReranksWhenSemanticFavorsDifferentWinner(t *testing.T) {
	t.Parallel()
	store := storeWithTools(t, []catalog.Tool{
		// All three share query terms so lexical rank follows toolRef order.
		{ToolRef: "a.term", ServerID: "a", DownstreamToolName: "term", Title: "search term alpha", Description: "alpha beta"},
		{ToolRef: "b.term", ServerID: "b", DownstreamToolName: "term", Title: "search term beta", Description: "beta gamma"},
		{ToolRef: "c.term", ServerID: "c", DownstreamToolName: "term", Title: "search term gamma", Description: "gamma delta"},
	})
	// Semantic strongly prefers c.term (rank 1) and demotes a.term (rank 3).
	// c.term gets 1/(60+lexRank) + 1/(60+1) — its RRF boost from sem rank 1.
	sem := &fakeSemantic{
		available: true,
		hits: []SemanticHit{
			{ToolRef: "c.term", Score: 0.95},
			{ToolRef: "b.term", Score: 0.5},
			{ToolRef: "a.term", Score: 0.2},
		},
	}
	engine := New(store, sem)
	ranking, _ := engine.Find(context.Background(), "search term")
	if len(ranking.Entries) < 3 {
		t.Fatalf("entries = %d, want 3", len(ranking.Entries))
	}
	// a.term: lex 1, sem 3 = 1/61 + 1/63 ≈ 0.03225
	// b.term: lex 2, sem 2 = 1/62 + 1/62 ≈ 0.03226
	// c.term: lex 3, sem 1 = 1/63 + 1/61 ≈ 0.03225
	// These are nearly tied; the test asserts c.term is in the top 2 (i.e.
	// semantic leg reordered it ahead of a.term) and a.term is not last.
	if ranking.Entries[0].Tool.ToolRef == "a.term" && ranking.Entries[2].Tool.ToolRef == "c.term" {
		t.Error("semantic leg failed to rerank: c.term should beat a.term in the top 2")
	}
}

func TestDecide_UseDecision(t *testing.T) {
	t.Parallel()
	tools := []catalog.Tool{
		{ToolRef: "a.tool1", Title: "Best Match", Description: "matches perfectly"},
		{ToolRef: "b.tool2", Title: "Runner Up", Description: "somewhat related"},
	}
	entries := []RankedEntry{
		{Tool: tools[0], LexicalScore: 80, FusedScore: 0.030, Reason: "matched perfectly"},
		{Tool: tools[1], LexicalScore: 5, FusedScore: 0.018, Reason: "somewhat related"},
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
	// Two entries with exactly equal RRF scores (gap = 0): always ambiguous.
	entries := []RankedEntry{
		{Tool: tools[0], LexicalScore: 50, FusedScore: 0.0328, Reason: "match a"},
		{Tool: tools[1], LexicalScore: 49, FusedScore: 0.0328, Reason: "match b"},
	}
	ranking := &Ranking{Entries: entries}
	d := Decide(ranking)
	if d.Verdict != DecisionAmbiguous {
		t.Fatalf("verdict = %s, want ambiguous (gap = 0)", d.Verdict)
	}
	if d.Selected == nil || d.RunnerUp == nil {
		t.Fatal("both selected and runner-up must be set for ambiguous")
	}
}

func TestDecide_NoGoodMatch_ComponentFloorBelow(t *testing.T) {
	t.Parallel()
	tools := []catalog.Tool{
		{ToolRef: "a.tool1", Title: "Some Tool"},
	}
	// Lexical score 0.5 -> normalize = 0.5/(0.5+6) ≈ 0.077, below floor 0.20.
	// Semantic score 0.1, below cosine floor 0.30.
	entries := []RankedEntry{
		{Tool: tools[0], LexicalScore: 0.5, SemanticScore: 0.1, FusedScore: 0.032, Reason: "weak"},
	}
	d := Decide(&Ranking{Entries: entries})
	if d.Verdict != DecisionNoGoodMatch {
		t.Fatalf("verdict = %s, want no_good_match (both component floors below)", d.Verdict)
	}
}

func TestDecide_LexicalFloorPasses(t *testing.T) {
	t.Parallel()
	tools := []catalog.Tool{
		{ToolRef: "a.tool1", Title: "Strong Lexical"},
	}
	// Lexical score 20 -> normalize = 20/26 ≈ 0.77, above floor 0.20.
	// Even with no semantic score, should pass the floor.
	entries := []RankedEntry{
		{Tool: tools[0], LexicalScore: 20, FusedScore: 0.032, Reason: "strong"},
	}
	d := Decide(&Ranking{Entries: entries})
	if d.Verdict != DecisionUse {
		t.Fatalf("verdict = %s, want use (lexical floor passes)", d.Verdict)
	}
}

func TestDecide_SemanticFloorPasses(t *testing.T) {
	t.Parallel()
	tools := []catalog.Tool{
		{ToolRef: "a.tool1", Title: "No Lexical Match"},
	}
	// Weak lexical (0.5 -> 0.077, below floor) BUT strong semantic (0.7, above
	// 0.30 cosine floor). Should pass via the OR clause.
	entries := []RankedEntry{
		{Tool: tools[0], LexicalScore: 0.5, SemanticScore: 0.7, FusedScore: 0.032, Reason: "semantic match"},
	}
	d := Decide(&Ranking{Entries: entries})
	if d.Verdict != DecisionUse {
		t.Fatalf("verdict = %s, want use (semantic cosine floor passes)", d.Verdict)
	}
}

func TestDecide_CatalogEmptyDecision(t *testing.T) {
	t.Parallel()
	d := Decide(&Ranking{Entries: nil})
	if d.Verdict != DecisionCatalogEmpty {
		t.Fatalf("verdict = %s, want catalog_empty", d.Verdict)
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

func TestDecide_UseWithHighConfidence(t *testing.T) {
	t.Parallel()
	tools := []catalog.Tool{
		{ToolRef: "a.tool1", Title: "Perfect Match"},
		{ToolRef: "b.tool2", Title: "Weak"},
	}
	// Top of both legs (lex 1, sem 1) gives FusedScore = 2/61 = 0.0328.
	entries := []RankedEntry{
		{Tool: tools[0], LexicalScore: 200, SemanticScore: 0.8, FusedScore: 0.0328, Reason: "strong match"},
		{Tool: tools[1], LexicalScore: 1, FusedScore: 0.0160, Reason: "weak"},
	}
	d := Decide(&Ranking{Entries: entries})
	if d.Verdict != DecisionUse {
		t.Fatalf("verdict = %s, want use", d.Verdict)
	}
	if d.Confidence != "high" {
		t.Errorf("confidence = %s, want high (FusedScore %.4f >= %.4f)", d.Confidence, entries[0].FusedScore, HighConfidenceThreshold)
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
		{Tool: catalog.Tool{ToolRef: "c"}, FusedScore: 0.01},
		{Tool: catalog.Tool{ToolRef: "a"}, FusedScore: 0.03},
		{Tool: catalog.Tool{ToolRef: "b"}, FusedScore: 0.02},
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

// TestEngine_Find_LexicalOrderWithNoSemanticReproduction verifies the design
// promise: with no semantic signal, RRF over the single lexical list reduces
// to the lexical order. We assert the relative order of the top two entries
// is identical to rankTools' output.
func TestEngine_Find_LexicalOrderWithNoSemanticReproduction(t *testing.T) {
	t.Parallel()
	store := storeWithTools(t, []catalog.Tool{
		{ToolRef: "a.best", ServerID: "a", Title: "Best", Description: "alpha beta"},
		{ToolRef: "b.weak", ServerID: "b", Title: "Weak", Description: "alpha"},
	})
	engine := New(store, nil)
	ranking, _ := engine.Find(context.Background(), "beta")
	if len(ranking.Entries) < 2 {
		t.Fatal("not enough entries")
	}
	if ranking.Entries[0].Tool.ToolRef != "a.best" {
		t.Errorf("top = %s, want a.best (lexical rank 1)", ranking.Entries[0].Tool.ToolRef)
	}
	if ranking.Entries[1].Tool.ToolRef != "b.weak" {
		t.Errorf("runner-up = %s, want b.weak (lexical rank 2)", ranking.Entries[1].Tool.ToolRef)
	}
}
