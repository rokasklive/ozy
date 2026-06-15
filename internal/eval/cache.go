package eval

import (
	"context"

	"github.com/rokasklive/ozy/internal/broker"
	"github.com/rokasklive/ozy/internal/catalog"
	"github.com/rokasklive/ozy/internal/config"
	"github.com/rokasklive/ozy/internal/contract"
	"github.com/rokasklive/ozy/internal/search"
)

// CacheEffectivenessMetrics report how much redundant broker work the result
// cache removes over a fixed repeated-call workload. RedundantCallReduction is
// the fraction of cacheable operations served from cache instead of re-executed
// (served / cacheable). The token figures show the downstream/search payload that
// was re-served from cache rather than re-fetched — work avoided, not agent
// tokens. Deterministic for a fixed corpus and estimator.
type CacheEffectivenessMetrics struct {
	Estimator              string  `json:"estimator"`
	CacheableOps           int     `json:"cacheableOps"`
	ServedFromCache        int     `json:"servedFromCache"`
	DelegatedOps           int     `json:"delegatedOps"`
	RedundantCallReduction float64 `json:"redundantCallReduction"`
	WorkloadResponseTokens int     `json:"workloadResponseTokens"`
	ExecutedResponseTokens int     `json:"executedResponseTokens"`
	TokensAvoided          int     `json:"tokensAvoided"`
}

// Fixed cache workload: a read-only tool's find/describe/call each issued twice
// (the second of each must hit), plus a write tool's call twice (must always be
// invoked live). Constant so the metric is reproducible.
const (
	cacheQuery    = "search the wiki for the deployment runbook"
	cacheReadOnly = "confluence.search_pages"
	cacheWriteRef = "jira.create_issue"
)

var (
	cacheReadArgs  = map[string]any{"query": "deployment runbook"}
	cacheWriteArgs = map[string]any{"projectKey": "OPS", "summary": "cache eval"}
)

type cacheOp struct {
	kind    string // "find" | "describe" | "call"
	query   string
	toolRef string
	args    map[string]any
}

func cacheWorkload() []cacheOp {
	return []cacheOp{
		{kind: "find", query: cacheQuery},
		{kind: "find", query: cacheQuery},
		{kind: "describe", toolRef: cacheReadOnly},
		{kind: "describe", toolRef: cacheReadOnly},
		{kind: "call", toolRef: cacheReadOnly, args: cacheReadArgs},
		{kind: "call", toolRef: cacheReadOnly, args: cacheReadArgs},
		{kind: "call", toolRef: cacheWriteRef, args: cacheWriteArgs},
		{kind: "call", toolRef: cacheWriteRef, args: cacheWriteArgs},
	}
}

// RunCacheEffectiveness drives the fixed workload through the production cache
// decorator layered over a call-counting broker, classifying each op as a cache
// hit or a delegated miss by the counter delta.
func RunCacheEffectiveness(ctx context.Context, corpus *Corpus, est TokenEstimator) (*CacheEffectivenessMetrics, error) {
	if est == nil {
		est = DefaultEstimator
	}
	store, err := corpus.Store()
	if err != nil {
		return nil, err
	}
	live := broker.NewLive(store, corpusConfig(corpus), &fixtureConnector{}, search.New(store, nil))
	counter := &countingBroker{inner: live}
	// Large TTL/size so nothing expires or evicts mid-workload.
	cached := broker.NewCaching(counter, store, config.CacheConfig{Enabled: true, TTLSeconds: 3600, MaxEntries: 100000})

	m := &CacheEffectivenessMetrics{Estimator: est.Name()}
	for _, op := range cacheWorkload() {
		cacheable := op.kind != "call" || isReadOnlyTool(ctx, store, op.toolRef)
		before := counter.calls
		resp := dispatchCacheOp(ctx, cached, op)
		tok := estimateJSON(est, resp)
		m.WorkloadResponseTokens += tok
		if counter.calls == before { // not delegated → served from cache
			m.ServedFromCache++
			m.TokensAvoided += tok
		} else {
			m.DelegatedOps++
		}
		if cacheable {
			m.CacheableOps++
		}
	}
	m.ExecutedResponseTokens = m.WorkloadResponseTokens - m.TokensAvoided
	if m.CacheableOps > 0 {
		m.RedundantCallReduction = float64(m.ServedFromCache) / float64(m.CacheableOps)
	}
	return m, nil
}

func dispatchCacheOp(ctx context.Context, bk broker.Broker, op cacheOp) any {
	switch op.kind {
	case "find":
		res, _ := bk.FindTool(ctx, op.query)
		return res
	case "describe":
		res, _ := bk.DescribeTool(ctx, op.toolRef)
		return res
	default:
		res, _ := bk.CallTool(ctx, op.toolRef, op.args)
		return res
	}
}

func isReadOnlyTool(ctx context.Context, store catalog.Store, toolRef string) bool {
	t, ok, err := store.GetTool(ctx, toolRef)
	return err == nil && ok && t.ReadOnly
}

// countingBroker counts calls delegated to the inner broker so the cache family
// can derive hits from miss deltas.
type countingBroker struct {
	inner broker.Broker
	calls int
}

func (b *countingBroker) FindTool(ctx context.Context, q string) (*contract.FindResult, error) {
	b.calls++
	return b.inner.FindTool(ctx, q)
}

func (b *countingBroker) DescribeTool(ctx context.Context, ref string) (*contract.DescribeResult, error) {
	b.calls++
	return b.inner.DescribeTool(ctx, ref)
}

func (b *countingBroker) CallTool(ctx context.Context, ref string, args map[string]any) (*contract.CallResult, error) {
	b.calls++
	return b.inner.CallTool(ctx, ref, args)
}

func (b *countingBroker) List(ctx context.Context) (*contract.ListResult, error) {
	b.calls++
	return b.inner.List(ctx)
}
