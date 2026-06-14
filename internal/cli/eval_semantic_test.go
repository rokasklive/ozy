package cli

import (
	"context"
	"os"
	"testing"

	"github.com/rokasklive/ozy/internal/eval"
)

// TestSemanticLegSkipsWhenDisabled verifies the harness records the semantic leg
// as not-run when neither --semantic nor OZY_EVAL_SEMANTIC requests it, and the
// lexical + structural evals still produce a result. No model needed.
func TestSemanticLegSkipsWhenDisabled(t *testing.T) {
	res, err := eval.Run(context.Background(), eval.Options{Families: []string{eval.FamilyDiscovery}})
	if err != nil {
		t.Fatalf("Run error = %v", err)
	}
	if res.Provenance.SemanticRan {
		t.Error("semantic leg should not run when disabled")
	}
	if res.Discovery == nil || res.Discovery.Overall.N == 0 {
		t.Error("lexical discovery should still run with semantic disabled")
	}
}

// TestSemanticLegImprovesDiscovery runs the real-model leg and asserts hybrid
// fusion lifts semantic-category top-1 above the lexical baseline. Gated behind
// OZY_EVAL_SEMANTIC=1 (mirrors the sidecar's slow-model convention) so the
// default `go test ./...` never provisions a sidecar or downloads a model.
func TestSemanticLegImprovesDiscovery(t *testing.T) {
	if os.Getenv("OZY_EVAL_SEMANTIC") != "1" {
		t.Skip("set OZY_EVAL_SEMANTIC=1 to run the real-model semantic eval")
	}
	ctx := context.Background()
	lex, err := eval.Run(ctx, eval.Options{Families: []string{eval.FamilyDiscovery}})
	if err != nil {
		t.Fatalf("lexical Run error = %v", err)
	}
	sem, err := eval.Run(ctx, eval.Options{
		Families:        []string{eval.FamilyDiscovery},
		Semantic:        true,
		SemanticBuilder: sidecarSemanticBuilder(""),
	})
	if err != nil {
		t.Fatalf("semantic Run error = %v", err)
	}
	if !sem.Provenance.SemanticRan {
		t.Fatal("semantic leg did not run despite being enabled (sidecar/model unavailable?)")
	}
	lexTop1 := lex.Discovery.ByCategory[eval.CategorySemantic].Top1
	semTop1 := sem.Discovery.ByCategory[eval.CategorySemantic].Top1
	if semTop1 <= lexTop1 {
		t.Errorf("hybrid leg did not improve semantic top-1: lexical=%.3f hybrid=%.3f", lexTop1, semTop1)
	}
}
