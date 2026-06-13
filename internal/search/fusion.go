package search

import (
	"fmt"
	"sort"
)

// Tunable fusion parameters — conservative defaults, calibrated by evals.
const (
	// LexSatK is the saturation constant for lexical score → [0,1] mapping.
	LexSatK = 6.0

	// DefaultLexWeight is the lexical signal weight when semantic is present.
	DefaultLexWeight = 0.6

	// DefaultSemWeight is the semantic signal weight when present.
	DefaultSemWeight = 0.4

	// RelevanceFloor is the minimum fused score to consider any match.
	RelevanceFloor = 0.25

	// SeparationMargin is the minimum gap between top and second for a "use" decision.
	SeparationMargin = 0.10

	// HighConfidenceThreshold is the fused score above which confidence is "high".
	HighConfidenceThreshold = 0.6
)

// normalizeLexical maps a raw BM25 score to [0,1] using saturating transform s/(s+k).
func normalizeLexical(score float64) float64 {
	if score <= 0 {
		return 0
	}
	return score / (score + LexSatK)
}

// normalizeSemantic maps cosine similarity [-1,1] to [0,1] via (cos+1)/2.
func normalizeSemantic(cos float64) float64 {
	return (cos + 1) / 2
}

// fuseScores combines lexical and semantic scores into a single [0,1] fused score.
// When hasSem is false, lexical weight is renormalized to 1.
func fuseScores(lexRaw, semRaw float64, hasSem bool) float64 {
	lexNorm := normalizeLexical(lexRaw)
	if !hasSem {
		return lexNorm
	}
	semNorm := normalizeSemantic(semRaw)
	return DefaultLexWeight*lexNorm + DefaultSemWeight*semNorm
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

// Decide maps a Ranking to a Decision using floor, margin, and confidence thresholds.
func Decide(ranking *Ranking) *Decision {
	if len(ranking.Entries) == 0 {
		return &Decision{Verdict: DecisionCatalogEmpty}
	}

	top := &ranking.Entries[0]
	topScore := top.FusedScore

	// No semantic? Apply normalization to fused scores.
	if !ranking.SemanticAvailable || ranking.SemanticDegraded {
		topScore = normalizeLexical(top.LexicalScore)
		top.FusedScore = topScore
	}

	if topScore < RelevanceFloor {
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
		if !ranking.SemanticAvailable || ranking.SemanticDegraded {
			secondScore = normalizeLexical(ranking.Entries[1].LexicalScore)
		}
	}

	gap := topScore - secondScore
	if gap >= SeparationMargin {
		conf := "medium"
		if topScore >= HighConfidenceThreshold {
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
