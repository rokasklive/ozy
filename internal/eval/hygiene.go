package eval

import (
	"context"
	"fmt"

	"github.com/rokasklive/ozy/internal/search"
)

// HygieneFinding flags a gold-set quality problem — currently a "semantic" intent
// the lexical baseline already wins outright, which means the label is not
// actually testing semantic matching (it's a lexical freebie).
type HygieneFinding struct {
	Intent   string `json:"intent"`
	Category string `json:"category"`
	Issue    string `json:"issue"`
}

// Hygiene runs the lexical-only baseline over the corpus and returns findings
// where a semantic-paraphrase intent is already won at rank 1 by lexical search.
// It keeps the "semantic" gold set honest: a genuine semantic case must give the
// semantic leg something to do. Returns an error only if the engine fails.
func Hygiene(corpus *Corpus) ([]HygieneFinding, error) {
	store, err := corpus.Store()
	if err != nil {
		return nil, err
	}
	engine := search.New(store, nil) // lexical only by construction
	ctx := context.Background()

	var findings []HygieneFinding
	for _, c := range corpus.Discovery {
		if c.Category != CategorySemantic {
			continue
		}
		ranking, err := engine.Find(ctx, c.Intent)
		if err != nil {
			return nil, err
		}
		order := rankedRefs(ranking)
		if firstAcceptableRank(order, c.Acceptable) == 1 {
			findings = append(findings, HygieneFinding{
				Intent:   c.Intent,
				Category: c.Category,
				Issue: fmt.Sprintf("lexical baseline already ranks an acceptable tool first (%s); "+
					"this 'semantic' case is a lexical freebie and should be reworded or recategorized", order[0]),
			})
		}
	}
	return findings, nil
}
