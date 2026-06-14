package eval

import (
	"context"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/rokasklive/ozy/evals"
	"github.com/rokasklive/ozy/internal/search"
)

// miniCatalog is a tiny valid catalog used by the in-memory dataset tests.
const miniCatalog = `{
  "version": 1,
  "servers": [{"id": "slack", "status": "online"}, {"id": "gmail", "status": "online"}],
  "tools": [
    {"toolRef": "slack.post_message", "serverId": "slack", "name": "post_message", "title": "Post Slack Message", "description": "Send a message to a Slack channel.", "inputSchema": {"type": "object"}},
    {"toolRef": "gmail.send_email", "serverId": "gmail", "name": "send_email", "title": "Send Email", "description": "Compose and send an email.", "inputSchema": {"type": "object"}}
  ]
}`

func TestLoadEmbeddedCorpus(t *testing.T) {
	corpus, err := Load(evals.Data())
	if err != nil {
		t.Fatalf("Load(embedded) error = %v", err)
	}
	if len(corpus.Catalog.Tools) == 0 {
		t.Fatal("embedded corpus has no tools")
	}
	seen := map[string]bool{}
	for _, c := range corpus.Discovery {
		seen[c.Category] = true
	}
	for _, want := range []string{CategoryLexical, CategorySemantic, CategoryNoMatch, CategoryAmbiguous, CategoryWrongServer} {
		if !seen[want] {
			t.Errorf("embedded discovery set missing category %q", want)
		}
	}
}

func TestLoadRejectsDanglingToolRef(t *testing.T) {
	fsys := fstest.MapFS{
		"catalog/world.json":  {Data: []byte(miniCatalog)},
		"discovery/bad.jsonl": {Data: []byte(`{"intent": "x", "category": "lexical", "acceptable": ["slack.nonexistent"], "rationale": "r"}`)},
	}
	_, err := Load(fsys)
	if err == nil {
		t.Fatal("Load should reject a dangling toolRef")
	}
	if !strings.Contains(err.Error(), "discovery/bad.jsonl") || !strings.Contains(err.Error(), "not present") {
		t.Errorf("error %q should name the file and the dangling-ref problem", err)
	}
}

func TestLoadRejectsMissingRationale(t *testing.T) {
	fsys := fstest.MapFS{
		"catalog/world.json": {Data: []byte(miniCatalog)},
		"discovery/r.jsonl":  {Data: []byte(`{"intent": "post to slack", "category": "lexical", "acceptable": ["slack.post_message"]}`)},
	}
	_, err := Load(fsys)
	if err == nil || !strings.Contains(err.Error(), "rationale") {
		t.Fatalf("Load should require a rationale, got %v", err)
	}
}

func TestHygieneFiresOnLexicalFreebie(t *testing.T) {
	// A "semantic" intent that is really a lexical match (echoes the tool name).
	fsys := fstest.MapFS{
		"catalog/world.json": {Data: []byte(miniCatalog)},
		"discovery/s.jsonl": {Data: []byte(
			`{"intent": "post message slack channel", "category": "semantic", "acceptable": ["slack.post_message"], "rationale": "planted leak"}`)},
	}
	corpus, err := Load(fsys)
	if err != nil {
		t.Fatalf("Load error = %v", err)
	}
	findings, err := Hygiene(corpus)
	if err != nil {
		t.Fatalf("Hygiene error = %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("expected 1 hygiene finding for the planted leak, got %d", len(findings))
	}
}

func TestRunDiscoveryDeterministic(t *testing.T) {
	corpus, err := Load(evals.Data())
	if err != nil {
		t.Fatalf("Load error = %v", err)
	}
	store, err := corpus.Store()
	if err != nil {
		t.Fatalf("Store error = %v", err)
	}
	engine := search.New(store, nil)
	first, err := RunDiscovery(context.Background(), engine, corpus.Discovery)
	if err != nil {
		t.Fatalf("RunDiscovery error = %v", err)
	}
	second, err := RunDiscovery(context.Background(), engine, corpus.Discovery)
	if err != nil {
		t.Fatalf("RunDiscovery error = %v", err)
	}
	if first.Overall != second.Overall {
		t.Errorf("discovery metrics are not deterministic: %+v vs %+v", first.Overall, second.Overall)
	}
}

func TestNoMatchCountsRefusalAsCorrect(t *testing.T) {
	corpus, err := Load(evals.Data())
	if err != nil {
		t.Fatalf("Load error = %v", err)
	}
	store, _ := corpus.Store()
	engine := search.New(store, nil)
	report, err := RunDiscovery(context.Background(), engine, corpus.Discovery)
	if err != nil {
		t.Fatalf("RunDiscovery error = %v", err)
	}
	nm, ok := report.ByCategory[CategoryNoMatch]
	if !ok || nm.N == 0 {
		t.Fatal("no_match category should have cases")
	}
	// NoMatchCorrectness is a rate in [0,1]; just assert it is computed.
	if nm.NoMatchCorrectness < 0 || nm.NoMatchCorrectness > 1 {
		t.Errorf("noMatchCorrectness = %v, want a rate in [0,1]", nm.NoMatchCorrectness)
	}
}

func TestGateSkipsSemanticWhenLegAbsent(t *testing.T) {
	report := &DiscoveryReport{
		Overall:    DiscoveryMetrics{N: 1, Top1: 1},
		ByCategory: map[string]DiscoveryMetrics{CategorySemantic: {N: 1, Top1: 0}},
	}
	minTop1 := 0.5
	th := &Thresholds{Discovery: map[string]DiscoveryGate{
		CategorySemantic: {Top1Min: &minTop1, RequiresSemanticLeg: true},
	}}
	gates := th.EvaluateDiscovery(report, false) // semantic leg did not run
	if len(gates) != 1 || !gates[0].Skipped {
		t.Fatalf("semantic gate should be skipped when the leg did not run, got %+v", gates)
	}
	if verdict(gates) != VerdictPass {
		t.Errorf("a run with only a skipped gate should pass, got %s", verdict(gates))
	}
	// When the leg runs, the same gate is enforced and fails (top1 0 < 0.5).
	gates = th.EvaluateDiscovery(report, true)
	if gates[0].Skipped || gates[0].Pass {
		t.Errorf("semantic gate should be enforced and fail when the leg ran, got %+v", gates[0])
	}
	if verdict(gates) != VerdictFail {
		t.Error("a failing enforced gate should fail the verdict")
	}
}

func TestRunEndToEnd(t *testing.T) {
	res, err := Run(context.Background(), Options{OutDir: ""})
	if err != nil {
		t.Fatalf("Run error = %v", err)
	}
	if res.Discovery == nil || res.Discovery.Overall.N == 0 {
		t.Fatal("Run should produce discovery metrics")
	}
	if res.Verdict != VerdictPass && res.Verdict != VerdictFail {
		t.Errorf("verdict = %q, want pass or fail", res.Verdict)
	}
	if res.Provenance.TokenEstimator == "" {
		t.Error("run provenance should record the token estimator")
	}
}
