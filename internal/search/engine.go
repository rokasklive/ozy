package search

import (
	"context"
	"sort"
)

// Find ranks all cataloged tools for a capability query. It builds a lexical
// rank list and, when a semantic provider is available, a semantic rank list
// (top-K nearest neighbors, default K = catalog size). The two lists are fused
// with Reciprocal Rank Fusion so ordering is rank-based and robust to the
// incomparable scales of BM25 and cosine similarity. With no semantic signal,
// RRF over the single lexical list reduces to the lexical order.
func (e *Engine) Find(ctx context.Context, query string) (*Ranking, error) {
	tools, err := e.store.Tools(ctx)
	if err != nil {
		return nil, err
	}

	ranking := &Ranking{}

	// Lexical rank list — always built, ordered by BM25 descending.
	var lexEntries []RankedEntry
	if len(tools) > 0 {
		lexEntries = rankTools(query, tools)
	}

	// Semantic rank list — only when a provider reports available.
	var semHits []SemanticHit
	if e.semantic != nil && e.semantic.Available() {
		ranking.SemanticAvailable = true
		k := len(tools)
		if k == 0 {
			k = 10
		}
		hits, sErr := e.semantic.Query(ctx, query, k, Filter{})
		if sErr != nil || hits == nil {
			ranking.SemanticDegraded = true
		} else {
			semHits = hits
		}
	}

	// Stash semantic cosine similarities on the corresponding lexical entries
	// so Decide can evaluate the absolute component floor.
	byRef := make(map[string]int, len(lexEntries))
	for i, ent := range lexEntries {
		byRef[ent.Tool.ToolRef] = i
	}
	for _, h := range semHits {
		if i, ok := byRef[h.ToolRef]; ok {
			lexEntries[i].SemanticScore = h.Score
		}
	}

	// RRF fusion: score each entry as Σ 1/(k+rank) over both rank lists.
	// When no semantic signal is available, the lexical leg alone produces
	// the lexical order (1/(k+lexRank) is monotonically decreasing in rank).
	for i := range lexEntries {
		lexRank := i + 1
		lexEntries[i].FusedScore = 1.0 / (RRFK + float64(lexRank))
		if len(semHits) > 0 {
			semRank := rankOfHits(semHits, lexEntries[i].Tool.ToolRef)
			lexEntries[i].FusedScore += 1.0 / (RRFK + float64(semRank))
		}
	}

	sort.SliceStable(lexEntries, func(i, j int) bool {
		return lexEntries[i].FusedScore > lexEntries[j].FusedScore
	})

	ranking.Entries = lexEntries
	return ranking, nil
}

func rankOfHits(hits []SemanticHit, toolRef string) int {
	for i, h := range hits {
		if h.ToolRef == toolRef {
			return i + 1
		}
	}
	return len(hits) + 1
}
