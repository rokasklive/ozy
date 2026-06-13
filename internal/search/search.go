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
type RankedEntry struct {
	Tool                  catalog.Tool
	LexicalScore          float64 // raw BM25 score
	FusedScore            float64 // combined lexical+semantic score in [0,1]
	MatchedTerms          []string
	TopContributingFields []string
	Reason                string
}

// Semantic is an optional semantic scoring seam. The default implementation
// reports unavailable so the runtime stays lexical-only until a provider lands.
type Semantic interface {
	Score(ctx context.Context, query string, tools []catalog.Tool) ([]float64, error)
	Available() bool
}

// Engine ranks cataloged tools for a capability query.
type Engine struct {
	store    catalog.Store
	semantic Semantic
}

// New constructs a search Engine. Semantics may be nil (lexical-only).
func New(store catalog.Store, semantic Semantic) *Engine {
	return &Engine{store: store, semantic: semantic}
}
