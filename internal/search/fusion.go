package search

import (
	"fmt"
	"sort"
)

// Tunable fusion parameters — conservative defaults, calibrated by evals.
const (
	// RRFK is the RRF dampening constant. k=60 is the documented default
	// (Cormack et al. 2009) and tolerates small ANN error from the sidecar.
	RRFK = 60.0

	// LexSatK is the saturation constant for lexical score → [0,1] mapping.
	LexSatK = 6.0

	// LexicalRelevanceFloor is the minimum normalized lexical score for the
	// top tool to count as a "real" match. With s/(s+k), s=1.5 maps to ~0.2.
	LexicalRelevanceFloor = 0.20

	// SemanticRelevanceFloor is the minimum cosine similarity for the top
	// tool to count as a "real" match on the semantic leg alone. Cosine is
	// in [-1, 1]; 0.30 is a sensible threshold for short BGE embeddings.
	SemanticRelevanceFloor = 0.30

	// SeparationMargin is the minimum RRF gap between the top two entries
	// for a confident `use` decision. With k=60, the natural gap between
	// adjacent ranks is ~1/(k+1) - 1/(k+2) ≈ 0.00027, so a margin of 0.0001
	// admits lex-rank-1 vs lex-rank-2 as confident while still flagging
	// truly tied entries (gap = 0) as ambiguous.
	SeparationMargin = 0.0001

	// HighConfidenceThreshold is the RRF score above which confidence is
	// reported as "high". 0.030 is just below the max (1/61 + 1/61 ≈ 0.0328),
	// so a tool at the top of both legs is "high" while a tool that only
	// leads one leg is "medium".
	HighConfidenceThreshold = 0.030
)

// normalizeLexical maps a raw BM25 score to [0,1] using saturating transform s/(s+k).
func normalizeLexical(score float64) float64 {
	if score <= 0 {
		return 0
	}
	return score / (score + LexSatK)
}

// rrfScore returns the RRF sum for one entry given its ranks in the two lists.
func rrfScore(lexRank, semRank int) float64 {
	return 1.0/(RRFK+float64(lexRank)) + 1.0/(RRFK+float64(semRank))
}

// Decision values for the ranking outcome.
const (
	DecisionUse          = "use"
	DecisionAmbiguous    = "ambiguous"
	DecisionNoGoodMatch  = "no_good_match"
	DecisionCatalogEmpty = "catalog_empty"
)

// Decision is the broker-facing result derived from a Ranking.
type Decision struct {
	Verdict    string
	Selected   *RankedEntry
	RunnerUp   *RankedEntry
	Confidence string
}

// Decide maps a Ranking to a Decision using:
//
//   - RRF orders the entries (the engine already sorted by FusedScore).
//   - The top tool must clear LexicalRelevanceFloor OR SemanticRelevanceFloor
//     on a component signal; otherwise the verdict is no_good_match. The RRF
//     score is rank-based and intentionally does not carry absolute relevance.
//   - use vs. ambiguous is decided by the RRF gap between top and runner-up.
//   - catalog_empty is preserved unchanged.
func Decide(ranking *Ranking) *Decision {
	if len(ranking.Entries) == 0 {
		return &Decision{Verdict: DecisionCatalogEmpty}
	}

	top := &ranking.Entries[0]
	if !passesComponentFloor(top) {
		return &Decision{
			Verdict:    DecisionNoGoodMatch,
			Selected:   top,
			RunnerUp:   runnerUp(ranking, 1),
			Confidence: "",
		}
	}

	var secondScore float64
	if len(ranking.Entries) > 1 {
		secondScore = ranking.Entries[1].FusedScore
	}

	gap := top.FusedScore - secondScore
	if gap >= SeparationMargin {
		conf := "medium"
		if top.FusedScore >= HighConfidenceThreshold {
			conf = "high"
		}
		return &Decision{
			Verdict:    DecisionUse,
			Selected:   top,
			RunnerUp:   runnerUp(ranking, 1),
			Confidence: conf,
		}
	}

	return &Decision{
		Verdict:  DecisionAmbiguous,
		Selected: top,
		RunnerUp: runnerUp(ranking, 1),
	}
}

// passesComponentFloor returns true if the entry clears either the lexical
// relevance floor or the semantic cosine floor, so RRF — which is rank-based —
// cannot declare a confident match on a low-relevance candidate.
func passesComponentFloor(e *RankedEntry) bool {
	return normalizeLexical(e.LexicalScore) >= LexicalRelevanceFloor ||
		e.SemanticScore >= SemanticRelevanceFloor
}

func runnerUp(r *Ranking, idx int) *RankedEntry {
	if idx < len(r.Entries) {
		return &r.Entries[idx]
	}
	return nil
}

// formatDecision produces a human-readable reason for the decision.
func formatDecision(d *Decision, query string) string {
	switch d.Verdict {
	case DecisionUse:
		return fmt.Sprintf("Best indexed tool for %s.", query)
	case DecisionAmbiguous:
		if d.Selected != nil && d.RunnerUp != nil {
			return fmt.Sprintf("Multiple tools match %q — top two are too close to separate confidently.", query)
		}
		return fmt.Sprintf("Multiple matches for %q with low separation.", query)
	case DecisionNoGoodMatch:
		return fmt.Sprintf("No indexed tool strongly matches %q.", query)
	case DecisionCatalogEmpty:
		return "The catalog has no indexed tools."
	default:
		return "Unknown decision."
	}
}

// sortedEntries returns entries sorted by fused score descending.
func sortedEntries(entries []RankedEntry) []RankedEntry {
	sorted := make([]RankedEntry, len(entries))
	copy(sorted, entries)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].FusedScore > sorted[j].FusedScore })
	return sorted
}
