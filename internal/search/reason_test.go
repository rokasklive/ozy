package search

import (
	"strings"
	"testing"

	"github.com/rokasklive/ozy/internal/catalog"
)

// reasonTermList extracts the bracketed term list from a rankTools reason.
func reasonTermList(t *testing.T, reason string) []string {
	t.Helper()
	start, end := strings.IndexByte(reason, '['), strings.IndexByte(reason, ']')
	if start < 0 || end <= start {
		t.Fatalf("reason has no term list: %q", reason)
	}
	return strings.Split(reason[start+1:end], ", ")
}

func TestRankTools_ReasonPrefersHighSignalTerms(t *testing.T) {
	t.Parallel()
	// "the"/"for" appear in every tool (high df → low idf); the distinctive
	// terms appear once. With six matched terms the reason keeps the top four
	// by IDF, so both stopwords drop out.
	tools := []catalog.Tool{
		{ToolRef: "web.search", ServerID: "web", DownstreamToolName: "search",
			Description: "search the web for recent news"},
		{ToolRef: "mail.send", ServerID: "mail", DownstreamToolName: "send",
			Description: "send the message for a mailbox"},
		{ToolRef: "db.query", ServerID: "db", DownstreamToolName: "query",
			Description: "query the database for rows"},
	}

	ranked := rankTools("search the web for recent news", tools)
	if len(ranked) == 0 || ranked[0].Tool.ToolRef != "web.search" {
		t.Fatalf("expected web.search on top, got %+v", ranked)
	}
	terms := reasonTermList(t, ranked[0].Reason)
	if len(terms) > maxReasonTerms {
		t.Fatalf("reason lists %d terms, want at most %d: %q", len(terms), maxReasonTerms, ranked[0].Reason)
	}
	for _, stop := range []string{"the", "for"} {
		for _, got := range terms {
			if got == stop {
				t.Fatalf("stopword %q presented as match evidence: %q", stop, ranked[0].Reason)
			}
		}
	}
	want := map[string]bool{"search": true, "web": true, "recent": true, "news": true}
	for _, got := range terms {
		if !want[got] {
			t.Fatalf("unexpected reason term %q in %q", got, ranked[0].Reason)
		}
	}
}
