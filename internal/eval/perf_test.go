package eval

import (
	"context"
	"testing"

	"github.com/rokasklive/ozy/evals"
	"github.com/rokasklive/ozy/internal/broker"
	"github.com/rokasklive/ozy/internal/search"
)

// benchEngine loads the committed corpus into a lexical-only engine for the
// performance benchmarks. Run with: go test -bench=. ./internal/eval/
func benchEngine(b *testing.B) *search.Engine {
	b.Helper()
	corpus, err := Load(evals.Data())
	if err != nil {
		b.Fatalf("Load: %v", err)
	}
	store, err := corpus.Store()
	if err != nil {
		b.Fatalf("Store: %v", err)
	}
	return search.New(store, nil)
}

// BenchmarkLexicalFind measures the BM25 lexical ranking path over the corpus.
func BenchmarkLexicalFind(b *testing.B) {
	engine := benchEngine(b)
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := engine.Find(ctx, "search confluence wiki pages"); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkFusionDecide measures RRF fusion + decision mapping on a ranked result.
func BenchmarkFusionDecide(b *testing.B) {
	engine := benchEngine(b)
	ranking, err := engine.Find(context.Background(), "search code on github")
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = search.Decide(ranking)
	}
}

// BenchmarkBrokerFindTool measures the end-to-end findTool broker path (ranking,
// decision, and instructional response assembly). FindTool never touches the
// downstream connector, so a nil connector/config is safe here.
func BenchmarkBrokerFindTool(b *testing.B) {
	corpus, err := Load(evals.Data())
	if err != nil {
		b.Fatalf("Load: %v", err)
	}
	store, err := corpus.Store()
	if err != nil {
		b.Fatalf("Store: %v", err)
	}
	bk := broker.NewLive(store, nil, nil, search.New(store, nil))
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := bk.FindTool(ctx, "let everyone know the release is out"); err != nil {
			b.Fatal(err)
		}
	}
}
