package search

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"unicode"

	"github.com/rokasklive/ozy/internal/catalog"
)

// field boos weights for BM25 (higher = more important).
const (
	boostToolRef           = 8.0
	boostDownstreamName    = 6.0
	boostTitle             = 5.0
	boostCapabilityAlias   = 3.0
	boostServerID          = 2.0
	boostDescription       = 1.5
	boostSchemaFieldName   = 0.8
	boostSchemaDescription = 0.5
)

// BM25 parameters.
const (
	bm25k1 = 1.2
	bm25b  = 0.75
)

// tokenize splits a query or text into lowercase token terms.
// Keeps terms of length ≥ 2 and skips pure symbols.
func tokenize(text string) []string {
	lower := strings.ToLower(text)
	parts := strings.FieldsFunc(lower, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if len(p) >= 2 {
			out = append(out, p)
		}
	}
	return out
}

// fieldTokenizer is a pair of (field name, weight, tokens).
type fieldTokens struct {
	name   string
	weight float64
	tokens []string
}

// extractFields builds the weighted indexed fields for a tool.
func extractFields(tool catalog.Tool) []fieldTokens {
	out := make([]fieldTokens, 0, 8)

	if tool.ToolRef != "" {
		out = append(out, fieldTokens{"toolRef", boostToolRef, tokenize(tool.ToolRef)})
	}
	if tool.DownstreamToolName != "" {
		out = append(out, fieldTokens{"downstreamToolName", boostDownstreamName, tokenize(tool.DownstreamToolName)})
	}
	if tool.Title != "" {
		out = append(out, fieldTokens{"title", boostTitle, tokenize(tool.Title)})
	}
	if tool.ServerID != "" {
		out = append(out, fieldTokens{"serverID", boostServerID, tokenize(tool.ServerID)})
	}
	if tool.Description != "" {
		out = append(out, fieldTokens{"description", boostDescription, tokenize(tool.Description)})
	}
	for _, alias := range tool.CapabilityText {
		if alias != "" {
			out = append(out, fieldTokens{"capabilityAlias", boostCapabilityAlias, tokenize(alias)})
		}
	}
	if tool.InputSchema != nil {
		schemaFieldNames, schemaDescriptions := extractSchemaText(tool.InputSchema)
		if len(schemaFieldNames) > 0 {
			out = append(out, fieldTokens{"schemaFieldName", boostSchemaFieldName, tokenize(strings.Join(schemaFieldNames, " "))})
		}
		if len(schemaDescriptions) > 0 {
			out = append(out, fieldTokens{"schemaDescription", boostSchemaDescription, tokenize(strings.Join(schemaDescriptions, " "))})
		}
	}
	return out
}

func extractSchemaText(schema map[string]any) (fieldNames, fieldDescriptions []string) {
	props, ok := schema["properties"].(map[string]any)
	if !ok {
		return nil, nil
	}
	for name, val := range props {
		fieldNames = append(fieldNames, name)
		propMap, ok := val.(map[string]any)
		if !ok {
			continue
		}
		if desc, ok := propMap["description"].(string); ok && desc != "" {
			fieldDescriptions = append(fieldDescriptions, desc)
		}
	}
	return fieldNames, fieldDescriptions
}

// termStats holds per-term document frequency and idf across the corpus.
type termStats struct {
	df  int
	idf float64
}

// lexicalScorer holds precomputed corpus-level stats for one query.
type lexicalScorer struct {
	queryTerms     []string
	termStats      map[string]termStats
	avgFieldLength float64
}

// newLexicalScorer builds corpus stats for a set of tools relative to a query.
func newLexicalScorer(query string, tools []catalog.Tool) *lexicalScorer {
	ls := &lexicalScorer{
		queryTerms: tokenize(query),
		termStats:  make(map[string]termStats),
	}
	if len(ls.queryTerms) == 0 {
		return ls
	}

	totalFields := 0
	totalTokens := 0
	for _, tool := range tools {
		for _, ft := range extractFields(tool) {
			totalFields++
			totalTokens += len(ft.tokens)
		}
	}
	if totalFields > 0 {
		ls.avgFieldLength = float64(totalTokens) / float64(totalFields)
	}

	df := make(map[string]int)
	for _, tool := range tools {
		seen := make(map[string]bool)
		for _, ft := range extractFields(tool) {
			for _, tok := range ft.tokens {
				if !seen[tok] {
					df[tok]++
					seen[tok] = true
				}
			}
		}
	}
	N := float64(len(tools))
	for _, qt := range ls.queryTerms {
		d := df[qt]
		ls.termStats[qt] = termStats{df: d, idf: idf(d, N)}
	}
	return ls
}

func idf(df int, n float64) float64 {
	if df == 0 {
		return 0
	}
	return math.Log((n-float64(df)+0.5)/(float64(df)+0.5) + 1)
}

// scoreFields computes BM25 for a single tool against the scorer's query terms.
// Returns total score and per-field contribution information.
func (ls *lexicalScorer) scoreFields(tool catalog.Tool) (score float64, fieldContrib map[string]float64) {
	fieldContrib = make(map[string]float64)
	if len(ls.queryTerms) == 0 {
		return 0, fieldContrib
	}

	allFields := extractFields(tool)
	for _, ft := range allFields {
		localScore := ls.scoreField(ft)
		if localScore > 0 {
			fieldContrib[ft.name] += localScore
			score += localScore
		}
	}
	return score, fieldContrib
}

func (ls *lexicalScorer) scoreField(ft fieldTokens) float64 {
	dl := float64(len(ft.tokens))
	if dl == 0 {
		return 0
	}

	tokenCounts := make(map[string]float64)
	for _, t := range ft.tokens {
		tokenCounts[t]++
	}

	var score float64
	for _, qt := range ls.queryTerms {
		ts, ok := ls.termStats[qt]
		if !ok || ts.idf <= 0 {
			continue
		}
		tf := tokenCounts[qt]
		if tf <= 0 {
			continue
		}
		denom := math.Max(1, bm25k1*(1-bm25b+bm25b*(dl/ls.avgFieldLength)))
		bm25 := ts.idf * (tf * (bm25k1 + 1)) / (tf + denom)
		score += bm25 * ft.weight
	}
	return score
}

// rankResult sorts entries descending by lexical score and fills reasons.
type rankedEntry struct {
	tool            catalog.Tool
	score           float64
	fieldContrib    map[string]float64
	matchedTermsSet map[string]bool
}

// rankTools scores and orders tools by lexical relevance.
func rankTools(query string, tools []catalog.Tool) []RankedEntry {
	if len(tools) == 0 {
		return nil
	}
	ls := newLexicalScorer(query, tools)

	var entries []rankedEntry
	for _, tool := range tools {
		score, fc := ls.scoreFields(tool)
		matched := make(map[string]bool)
		for _, qt := range ls.queryTerms {
			indexedTokens := gatherTokens(tool)
			if containsToken(indexedTokens, qt) {
				matched[qt] = true
			}
		}
		entries = append(entries, rankedEntry{
			tool:            tool,
			score:           score,
			fieldContrib:    fc,
			matchedTermsSet: matched,
		})
	}

	sort.Slice(entries, func(i, j int) bool { return entries[i].score > entries[j].score })

	result := make([]RankedEntry, len(entries))
	for i, e := range entries {
		terms := make([]string, 0, len(e.matchedTermsSet))
		for t := range e.matchedTermsSet {
			terms = append(terms, t)
		}
		sort.Strings(terms)
		reasonTerms := topSignalTerms(terms, ls, maxReasonTerms)

		type fieldScore struct {
			name  string
			score float64
		}
		var fs []fieldScore
		for name, s := range e.fieldContrib {
			fs = append(fs, fieldScore{name: name, score: s})
		}
		sort.Slice(fs, func(i, j int) bool { return fs[i].score > fs[j].score })
		topFields := make([]string, 0, 3)
		for j := 0; j < len(fs) && j < 3; j++ {
			topFields = append(topFields, fs[j].name)
		}
		if len(topFields) == 0 {
			topFields = append(topFields, "description")
		}

		reason := fmt.Sprintf("Matched terms [%s] in %s", strings.Join(reasonTerms, ", "), strings.Join(topFields, ", "))
		if len(terms) == 0 && len(tools) > 0 {
			reason = "No direct term match"
		}

		result[i] = RankedEntry{
			Tool:                  e.tool,
			LexicalScore:          e.score,
			FusedScore:            e.score, // set during fusion
			MatchedTerms:          terms,
			TopContributingFields: topFields,
			Reason:                reason,
		}
	}
	return result
}

// maxReasonTerms caps how many matched terms a reason string names.
const maxReasonTerms = 4

// reasonIdfFloor drops a matched term from the reason when its IDF falls under
// this fraction of the strongest matched term's IDF, so corpus-specific noise
// is trimmed relative to the real evidence.
const reasonIdfFloor = 0.4

// reasonStopwords are English function words excluded from reason strings —
// display only, never from scoring. Small catalogs compress IDF too much for
// the corpus alone to separate connectives from evidence ("and" at df 30/69
// still clears any sane relative floor), and presenting "and"/"the" as match
// evidence makes the ranking read like a toy keyword matcher.
var reasonStopwords = map[string]bool{
	"a": true, "an": true, "and": true, "are": true, "as": true, "at": true,
	"be": true, "by": true, "for": true, "from": true, "in": true, "is": true,
	"it": true, "my": true, "of": true, "on": true, "or": true, "the": true,
	"to": true, "with": true,
}

// topSignalTerms returns at most n matched terms ranked by corpus IDF
// descending: the highest-signal evidence. Function-word stopwords and terms
// under the relative IDF floor are dropped from the display (scoring is
// untouched); when only stopwords matched, the strongest of them is kept so a
// matched entry never shows an empty reason.
func topSignalTerms(terms []string, ls *lexicalScorer, n int) []string {
	if len(terms) == 0 {
		return terms
	}
	ranked := append([]string(nil), terms...)
	sort.SliceStable(ranked, func(i, j int) bool {
		return ls.termStats[ranked[i]].idf > ls.termStats[ranked[j]].idf
	})
	floor := ls.termStats[ranked[0]].idf * reasonIdfFloor
	kept := make([]string, 0, len(ranked))
	for _, t := range ranked {
		if !reasonStopwords[t] && ls.termStats[t].idf >= floor {
			kept = append(kept, t)
		}
	}
	if len(kept) == 0 {
		kept = ranked[:1]
	}
	if len(kept) > n {
		kept = kept[:n]
	}
	sort.Strings(kept)
	return kept
}

func gatherTokens(tool catalog.Tool) map[string]bool {
	tokens := make(map[string]bool)
	for _, ft := range extractFields(tool) {
		for _, tok := range ft.tokens {
			tokens[tok] = true
		}
	}
	return tokens
}

func containsToken(tokens map[string]bool, qt string) bool {
	return tokens[qt]
}
