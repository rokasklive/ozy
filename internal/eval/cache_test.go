package eval

import (
	"context"
	"testing"
)

// cacheTestCorpus is a minimal corpus whose tools match the fixed cache
// workload: a read-only confluence tool and a write jira tool.
func cacheTestCorpus() *Corpus {
	obj := map[string]any{"type": "object"}
	return &Corpus{Catalog: Catalog{
		Version: 1,
		Servers: []CatalogServer{{ID: "confluence", Status: "online"}, {ID: "jira", Status: "online"}},
		Tools: []CatalogTool{
			{ToolRef: "confluence.search_pages", ServerID: "confluence", Name: "search_pages", InputSchema: obj, ReadOnly: true},
			{ToolRef: "jira.create_issue", ServerID: "jira", Name: "create_issue", InputSchema: obj},
		},
	}}
}

func TestStore_MapsReadOnly(t *testing.T) {
	store, err := cacheTestCorpus().Store()
	if err != nil {
		t.Fatalf("Store: %v", err)
	}
	ctx := context.Background()
	for ref, want := range map[string]bool{"confluence.search_pages": true, "jira.create_issue": false} {
		tool, ok, err := store.GetTool(ctx, ref)
		if err != nil || !ok {
			t.Fatalf("GetTool(%q) ok=%v err=%v", ref, ok, err)
		}
		if tool.ReadOnly != want {
			t.Errorf("%s ReadOnly = %v, want %v", ref, tool.ReadOnly, want)
		}
	}
}

func TestRunCacheEffectiveness(t *testing.T) {
	ctx := context.Background()
	m, err := RunCacheEffectiveness(ctx, cacheTestCorpus(), nil)
	if err != nil {
		t.Fatalf("RunCacheEffectiveness: %v", err)
	}

	// Workload: find×2, describe×2, read-only call×2 (6 cacheable; 3 second-occurrences hit),
	// write call×2 (always delegated, never cacheable).
	if m.CacheableOps != 6 {
		t.Errorf("CacheableOps = %d, want 6", m.CacheableOps)
	}
	if m.ServedFromCache != 3 {
		t.Errorf("ServedFromCache = %d, want 3 (write calls must never hit)", m.ServedFromCache)
	}
	if m.DelegatedOps != 5 {
		t.Errorf("DelegatedOps = %d, want 5 (3 read-only misses + 2 write calls)", m.DelegatedOps)
	}
	if m.RedundantCallReduction != 0.5 {
		t.Errorf("RedundantCallReduction = %v, want 0.5", m.RedundantCallReduction)
	}
	if m.TokensAvoided <= 0 || m.ExecutedResponseTokens != m.WorkloadResponseTokens-m.TokensAvoided {
		t.Errorf("token accounting off: workload=%d executed=%d avoided=%d",
			m.WorkloadResponseTokens, m.ExecutedResponseTokens, m.TokensAvoided)
	}

	// Deterministic across runs.
	m2, err := RunCacheEffectiveness(ctx, cacheTestCorpus(), nil)
	if err != nil {
		t.Fatalf("second run: %v", err)
	}
	if *m2 != *m {
		t.Errorf("non-deterministic: %+v vs %+v", m2, m)
	}
}
