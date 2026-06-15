package eval

import (
	"context"

	ozymcp "github.com/rokasklive/ozy/internal/mcp"
)

// TokenEconomyMetrics are the SPEC.md §13 token-economy numbers, computed
// deterministically from captured schemas and payloads with a documented,
// swappable estimator. The headline is the startup reduction: Ozy advertises
// three tool schemas where direct-MCP loads the entire downstream universe.
type TokenEconomyMetrics struct {
	Estimator             string  `json:"estimator"`
	DirectStartupTokens   int     `json:"directStartupTokens"`
	OzyStartupTokens      int     `json:"ozyStartupTokens"`
	StartupReductionRatio float64 `json:"startupReductionRatio"`
	DirectTokensToSuccess int     `json:"directTokensToSuccess"`
	OzyTokensToSuccess    int     `json:"ozyTokensToSuccess"`
	LargestPayloadTokens  int     `json:"largestPayloadTokens"`
	BrokerCalls           int     `json:"brokerCalls"`
}

// Representative task used for the per-task tokens-to-success measurement. The
// toolRef and arguments are fixed (not ranking-dependent) so the number reflects
// payload economy, not discovery accuracy, which the discovery family owns.
const (
	econQuery   = "search the wiki for the deployment runbook"
	econToolRef = "confluence.search_pages"
)

var econArgs = map[string]any{"query": "deployment runbook"}

// RunTokenEconomy measures startup and per-task token economy for the direct-MCP
// baseline (all downstream schemas) versus the Ozy broker path (three tools plus
// per-task find→describe→call). Deterministic for a fixed corpus and estimator.
func RunTokenEconomy(ctx context.Context, corpus *Corpus, est TokenEstimator) (*TokenEconomyMetrics, error) {
	if est == nil {
		est = DefaultEstimator
	}

	directStartup := 0
	for _, t := range corpus.Catalog.Tools {
		directStartup += estimateJSON(est, toolSchemaDoc(t))
	}
	ozyStartup := 0
	for _, def := range ozyToolDefs() {
		ozyStartup += estimateJSON(est, def)
	}

	bk, err := newCorpusBroker(corpus, nil)
	if err != nil {
		return nil, err
	}
	find, _ := bk.FindTool(ctx, econQuery)
	describe, derr := bk.DescribeTool(ctx, econToolRef)
	if derr != nil {
		return nil, derr
	}
	call, cerr := bk.CallTool(ctx, econToolRef, econArgs)
	if cerr != nil {
		return nil, cerr
	}

	findTok := estimateJSON(est, find)
	describeTok := estimateJSON(est, describe)
	callTok := estimateJSON(est, call)
	resultTok := estimateJSON(est, call.Result)

	m := &TokenEconomyMetrics{
		Estimator:             est.Name(),
		DirectStartupTokens:   directStartup,
		OzyStartupTokens:      ozyStartup,
		DirectTokensToSuccess: directStartup + resultTok,
		OzyTokensToSuccess:    ozyStartup + findTok + describeTok + callTok,
		LargestPayloadTokens:  max3(findTok, describeTok, callTok),
		BrokerCalls:           3,
	}
	if directStartup > 0 {
		m.StartupReductionRatio = 1 - float64(ozyStartup)/float64(directStartup)
	}
	return m, nil
}

// toolSchemaDoc is what an agent loads per downstream tool at startup on the
// direct-MCP path: name, description, and full input schema.
func toolSchemaDoc(t CatalogTool) map[string]any {
	return map[string]any{
		"name":        t.ToolRef,
		"description": t.Description,
		"inputSchema": t.InputSchema,
	}
}

// ozyToolDefs are the three stable Ozy tool definitions an agent loads at
// startup (SPEC.md §4.3), mirroring the MCP adapter surface.
func ozyToolDefs() []map[string]any {
	str := func(desc string) map[string]any {
		return map[string]any{"type": "string", "description": desc}
	}
	return []map[string]any{
		{
			"name":        "findTool",
			"description": ozymcp.OzyFindDescription,
			"inputSchema": map[string]any{
				"type":       "object",
				"required":   []any{"query"},
				"properties": map[string]any{"query": str("capability query describing the tool you need")},
			},
		},
		{
			"name":        "describeTool",
			"description": ozymcp.OzyDescribeDescription,
			"inputSchema": map[string]any{
				"type":       "object",
				"required":   []any{"toolRef"},
				"properties": map[string]any{"toolRef": str("stable Ozy tool reference, e.g. atlassian.confluence_search")},
			},
		},
		{
			"name":        "callTool",
			"description": ozymcp.OzyCallDescription,
			"inputSchema": map[string]any{
				"type":     "object",
				"required": []any{"toolRef"},
				"properties": map[string]any{
					"toolRef":   str("stable Ozy tool reference to invoke"),
					"arguments": map[string]any{"type": "object", "description": "arguments to pass to the downstream tool"},
				},
			},
		},
	}
}

// max3 returns the largest of three ints.
func max3(a, b, c int) int {
	m := a
	if b > m {
		m = b
	}
	if c > m {
		m = c
	}
	return m
}
