// Package search ranks cataloged tools for a capability query with a lexical
// baseline and an optional semantic signal (SPEC.md §10).
package search

import (
	"context"

	"github.com/rokasklive/ozy/internal/catalog"
)

// Ranking is the ranked result of a capability search.
type Ranking struct {
	Entries           []RankedEntry
	SemanticAvailable bool
	SemanticDegraded  bool
}

// RankedEntry is one ranked tool with its scores, matched terms, and explanation.
//
// LexicalScore and SemanticScore are the raw component signals used by Decide
// to gate the no_good_match confidence floor. FusedScore is the RRF sum used
// purely for ordering. Reason is the human-readable explanation, e.g.
// "Matched terms [search, confluence] in title, description".
type RankedEntry struct {
	Tool                  catalog.Tool
	LexicalScore          float64
	SemanticScore         float64
	FusedScore            float64
	MatchedTerms          []string
	TopContributingFields []string
	Reason                string
}

// SemanticHit is one ranked semantic nearest neighbor returned by a provider.
type SemanticHit struct {
	ToolRef string
	Score   float64
}

// Filter restricts a semantic query to a subset of the indexed tools. An
// empty Filter means "search the entire catalog".
type Filter struct {
	ToolRefs []string
	ServerID string
}

// Semantic is the semantic ranking seam. A real provider (e.g. the sidecar)
// implements Query by embedding the query, searching its vector index, and
// returning ranked toolRefs + cosine similarity. The default implementation
// reports Available() == false, so the engine stays lexical-only.
type Semantic interface {
	Query(ctx context.Context, query string, k int, filter Filter) ([]SemanticHit, error)
	Available() bool
}

// Engine ranks cataloged tools for a capability query by combining a lexical
// rank list with an optional semantic rank list through Reciprocal Rank Fusion.
type Engine struct {
	store    catalog.Store
	semantic Semantic
}

// New constructs a search Engine. semantic may be nil (lexical-only).
func New(store catalog.Store, semantic Semantic) *Engine {
	return &Engine{store: store, semantic: semantic}
}
