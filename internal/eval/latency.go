package eval

import (
	"context"
	"math"
	"sort"
	"time"

	"github.com/rokasklive/ozy/internal/broker"
	"github.com/rokasklive/ozy/internal/search"
)

// latencyIters is the per-path sample count for the latency distribution. It is
// modest so it adds little to a run while still yielding a stable p50/p95.
const latencyIters = 100

// latencyQuery is the representative capability query the latency paths exercise.
const latencyQuery = "search confluence wiki pages for the deployment runbook"

// LatencyStats summarizes a path's latency distribution and throughput. Timings
// are environment-dependent, so these are reported (with the run's host
// provenance), never gated.
type LatencyStats struct {
	N         int     `json:"n"`
	P50Micros float64 `json:"p50Micros"`
	P95Micros float64 `json:"p95Micros"`
	OpsPerSec float64 `json:"opsPerSec"`
}

// LatencyReport holds per-path latency for the lexical ranker, RRF fusion, the
// end-to-end broker findTool path, and (when the leg ran) the semantic path.
type LatencyReport struct {
	Lexical  *LatencyStats `json:"lexical,omitempty"`
	Fusion   *LatencyStats `json:"fusion,omitempty"`
	EndToEnd *LatencyStats `json:"endToEnd,omitempty"`
	Semantic *LatencyStats `json:"semantic,omitempty"`
}

// RunLatency measures the latency distribution of each retrieval path. lexEngine
// and bk are the lexical-only baseline; semEngine (optional) measures the real
// semantic path when the gated leg ran.
func RunLatency(ctx context.Context, lexEngine *search.Engine, bk broker.Broker, semEngine *search.Engine) *LatencyReport {
	report := &LatencyReport{
		Lexical: measureLatency(func() { _, _ = lexEngine.Find(ctx, latencyQuery) }),
		EndToEnd: measureLatency(func() {
			if bk != nil {
				_, _ = bk.FindTool(ctx, latencyQuery)
			}
		}),
	}
	// Fusion is measured on a precomputed ranking so it times only the RRF +
	// decision step, not the underlying retrieval.
	if ranking, err := lexEngine.Find(ctx, latencyQuery); err == nil {
		report.Fusion = measureLatency(func() { _ = search.Decide(ranking) })
	}
	if semEngine != nil {
		report.Semantic = measureLatency(func() { _, _ = semEngine.Find(ctx, latencyQuery) })
	}
	return report
}

// measureLatency runs fn latencyIters times (after one warmup) and returns the
// p50/p95 and throughput. fn must be side-effect-free for the measurement.
func measureLatency(fn func()) *LatencyStats {
	fn() // warm up caches so the first sample is not an outlier
	durs := make([]time.Duration, latencyIters)
	start := time.Now()
	for i := 0; i < latencyIters; i++ {
		t0 := time.Now()
		fn()
		durs[i] = time.Since(t0)
	}
	total := time.Since(start)

	sort.Slice(durs, func(i, j int) bool { return durs[i] < durs[j] })
	ops := 0.0
	if total > 0 {
		ops = float64(latencyIters) / total.Seconds()
	}
	return &LatencyStats{
		N:         latencyIters,
		P50Micros: micros(percentile(durs, 0.50)),
		P95Micros: micros(percentile(durs, 0.95)),
		OpsPerSec: ops,
	}
}

// percentile returns the p-quantile of a sorted duration slice (nearest-rank).
func percentile(sorted []time.Duration, p float64) time.Duration {
	if len(sorted) == 0 {
		return 0
	}
	idx := int(math.Ceil(p*float64(len(sorted)))) - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}

// micros converts a duration to fractional microseconds (sub-µs paths like
// fusion would round to zero otherwise).
func micros(d time.Duration) float64 {
	return float64(d.Nanoseconds()) / 1000.0
}
