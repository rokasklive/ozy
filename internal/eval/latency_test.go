package eval

import (
	"context"
	"testing"
	"time"

	"github.com/rokasklive/ozy/internal/search"
)

func TestRunLatency(t *testing.T) {
	corpus := mustLoad(t)
	store, err := corpus.Store()
	if err != nil {
		t.Fatalf("Store error = %v", err)
	}
	bk, err := newCorpusBroker(corpus, nil)
	if err != nil {
		t.Fatalf("newCorpusBroker error = %v", err)
	}
	report := RunLatency(context.Background(), search.New(store, nil), bk, nil)

	for name, s := range map[string]*LatencyStats{
		"lexical":  report.Lexical,
		"fusion":   report.Fusion,
		"endToEnd": report.EndToEnd,
	} {
		if s == nil {
			t.Fatalf("%s latency not measured", name)
		}
		//nolint:staticcheck // SA5011: s guaranteed non-nil after Fatalf (staticcheck false positive)
		if s.N != latencyIters {
			t.Errorf("%s N = %d, want %d", name, s.N, latencyIters)
		}
		if s.P95Micros < s.P50Micros {
			t.Errorf("%s p95 (%.3f) < p50 (%.3f)", name, s.P95Micros, s.P50Micros)
		}
		if s.OpsPerSec <= 0 {
			t.Errorf("%s opsPerSec = %.1f, want > 0", name, s.OpsPerSec)
		}
	}
	// The semantic path was not provided, so it must be absent (not zero-valued).
	if report.Semantic != nil {
		t.Error("semantic latency should be nil when no semantic engine is passed")
	}
}

func TestPercentile(t *testing.T) {
	durs := []time.Duration{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	if got := percentile(durs, 0.50); got != 5 {
		t.Errorf("p50 = %v, want 5", got)
	}
	if got := percentile(durs, 0.95); got != 10 {
		t.Errorf("p95 = %v, want 10", got)
	}
	if got := percentile(nil, 0.5); got != 0 {
		t.Errorf("p50 of empty = %v, want 0", got)
	}
}
