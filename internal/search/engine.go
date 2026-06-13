package search

import (
	"context"
)

// Find ranks all cataloged tools for a capability query.
func (e *Engine) Find(ctx context.Context, query string) (*Ranking, error) {
	tools, err := e.store.Tools(ctx)
	if err != nil {
		return nil, err
	}

	ranking := &Ranking{}

	// Lexical baseline — always available.
	if len(tools) > 0 {
		lexEntries := rankTools(query, tools)
		ranking.Entries = lexEntries
	}

	// Semantic signal — optional, only when available.
	if e.semantic != nil && e.semantic.Available() {
		ranking.SemanticAvailable = true
		semScores, sErr := e.semantic.Score(ctx, query, tools)
		if sErr != nil || len(semScores) != len(tools) {
			ranking.SemanticDegraded = true
		} else {
			for i := range ranking.Entries {
				if i < len(semScores) {
					ranking.Entries[i].FusedScore = fuseScores(ranking.Entries[i].LexicalScore, semScores[i], true)
				}
			}
		}
	} else {
		for i := range ranking.Entries {
			ranking.Entries[i].FusedScore = normalizeLexical(ranking.Entries[i].LexicalScore)
		}
	}

	return ranking, nil
}
