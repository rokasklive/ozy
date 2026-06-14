package eval

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"

	"github.com/rokasklive/ozy/internal/broker"
	"github.com/rokasklive/ozy/internal/contract"
	"github.com/rokasklive/ozy/internal/render"
)

// Per-kind response-size budgets (bytes of the rendered JSON), the §13 ceilings
// the structural checks enforce. findTool stays small (a schema preview, never
// full schemas); describeTool legitimately carries one full schema; callTool
// results are bounded.
const (
	budgetFind     = 6000
	budgetDescribe = 20000
	budgetCall     = 8000
)

// ErgonomicsMetrics are the structural §4.5/§9/§13 conformance rates plus CLI↔MCP
// parity, organized by the agent-ergonomics domains: decision clarity
// (DecisionRate), actionable guidance (InstructionRate), error recoverability
// (ErrorDispositionRate), response economy (WithinBudgetRate), and surface
// consistency (ParityRate).
type ErgonomicsMetrics struct {
	N                    int     `json:"n"`
	DecisionRate         float64 `json:"decisionRate"`
	InstructionRate      float64 `json:"instructionRate"`
	ErrorDispositionRate float64 `json:"errorDispositionRate"`
	WithinBudgetRate     float64 `json:"withinBudgetRate"`
	ParityRate           float64 `json:"parityRate"`
}

// ErgonomicsReport is the ergonomics family result. The flagged-case lists make
// regressions self-explaining in the snapshot.
type ErgonomicsReport struct {
	Overall          ErgonomicsMetrics `json:"overall"`
	NonInstructional []string          `json:"nonInstructional,omitempty"`
	BudgetExceeded   []string          `json:"budgetExceeded,omitempty"`
	ParityMismatches []string          `json:"parityMismatches,omitempty"`
}

// RunErgonomics exercises every ergonomics case on both the CLI (--format json)
// path and the in-process MCP adapter over one shared broker, applies the
// structural conformance checks to the broker response, and asserts CLI↔MCP
// parity. Deterministic for a fixed corpus.
func RunErgonomics(ctx context.Context, corpus *Corpus) (*ErgonomicsReport, error) {
	bk, err := newCorpusBroker(corpus, nil)
	if err != nil {
		return nil, err
	}
	probe, err := newMCPProbe(ctx, bk)
	if err != nil {
		return nil, err
	}
	defer probe.Close()

	acc := &ergoAcc{}
	for _, c := range corpus.Ergonomics {
		cliDoc, native := cliSurface(ctx, bk, c)
		acc.scoreStructural(c, native)

		name, args := mcpArgs(c)
		mcpDoc, err := probe.call(ctx, name, args)
		if err != nil {
			return nil, err
		}
		acc.scoreParity(c, observedFromJSON(cliDoc), observedFromJSON(mcpDoc))
	}
	return acc.report(), nil
}

// nativeResult holds what the structural checks inspect: the typed findTool
// result, any structured error, and the rendered response size.
type nativeResult struct {
	find *contract.FindResult
	err  *contract.Error
	size int
}

// cliSurface runs one case through the broker and renders it exactly as the CLI
// `--format json` path does, returning the decoded document and the typed pieces
// the structural checks need.
func cliSurface(ctx context.Context, bk broker.Broker, c ErgonomicsCase) (map[string]any, nativeResult) {
	var native nativeResult
	var value any
	switch c.Kind {
	case KindFind:
		res, _ := bk.FindTool(ctx, c.Query)
		native.find = res
		value = res
	case KindDescribe:
		res, e := bk.DescribeTool(ctx, c.ToolRef)
		if e != nil {
			native.err = asContractErr(e)
			value = contract.NewErrorEnvelope(native.err)
		} else {
			value = res
		}
	case KindCall:
		res, e := bk.CallTool(ctx, c.ToolRef, c.Arguments)
		if e != nil {
			native.err = asContractErr(e)
			value = contract.NewErrorEnvelope(native.err)
		} else {
			value = res
		}
	}
	var buf bytes.Buffer
	_ = render.Output(&buf, contract.FormatJSON, value)
	native.size = buf.Len()
	doc := map[string]any{}
	_ = json.Unmarshal(buf.Bytes(), &doc)
	return doc, native
}

// mcpArgs maps an ergonomics case to the MCP tool name and arguments.
func mcpArgs(c ErgonomicsCase) (string, map[string]any) {
	switch c.Kind {
	case KindFind:
		return "findTool", map[string]any{"query": c.Query}
	case KindDescribe:
		return "describeTool", map[string]any{"toolRef": c.ToolRef}
	default:
		args := map[string]any{"toolRef": c.ToolRef}
		if c.Arguments != nil {
			args["arguments"] = c.Arguments
		}
		return "callTool", args
	}
}

type ergoAcc struct {
	nAll, budgetOK      int
	nFind, decisionOK   int
	instructionOK       int
	nErr, dispositionOK int
	parityN, parityOK   int
	nonInstr, budgetEx  []string
	parityMismatch      []string
}

func (a *ergoAcc) scoreStructural(c ErgonomicsCase, n nativeResult) {
	a.nAll++
	if n.size <= budgetFor(c.Kind) {
		a.budgetOK++
	} else {
		a.budgetEx = append(a.budgetEx, c.Name)
	}

	if c.Kind == KindFind {
		a.nFind++
		if knownDecision(n.find) {
			a.decisionOK++
		}
		if groundedNextStep(n.find, c.Query) {
			a.instructionOK++
		} else {
			a.nonInstr = append(a.nonInstr, c.Name)
		}
	}

	if n.err != nil {
		a.nErr++
		if n.err.Type != "" && strings.TrimSpace(n.err.AgentInstruction) != "" {
			a.dispositionOK++
		}
	}
}

func (a *ergoAcc) scoreParity(c ErgonomicsCase, cli, mcp observed) {
	a.parityN++
	if cli == mcp {
		a.parityOK++
	} else {
		a.parityMismatch = append(a.parityMismatch, c.Name)
	}
}

func (a *ergoAcc) report() *ErgonomicsReport {
	rate := func(num, den int) float64 {
		if den == 0 {
			return 0
		}
		return float64(num) / float64(den)
	}
	return &ErgonomicsReport{
		Overall: ErgonomicsMetrics{
			N:                    a.nAll,
			DecisionRate:         rate(a.decisionOK, a.nFind),
			InstructionRate:      rate(a.instructionOK, a.nFind),
			ErrorDispositionRate: rate(a.dispositionOK, a.nErr),
			WithinBudgetRate:     rate(a.budgetOK, a.nAll),
			ParityRate:           rate(a.parityOK, a.parityN),
		},
		NonInstructional: a.nonInstr,
		BudgetExceeded:   a.budgetEx,
		ParityMismatches: a.parityMismatch,
	}
}

func budgetFor(kind string) int {
	switch kind {
	case KindDescribe:
		return budgetDescribe
	case KindCall:
		return budgetCall
	default:
		return budgetFind
	}
}

// knownDecision reports whether a findTool result carries an explicit, known
// decision value (SPEC.md §9.1) — never an empty or freeform verdict.
func knownDecision(find *contract.FindResult) bool {
	if find == nil {
		return false
	}
	switch find.Decision {
	case contract.DecisionUse, contract.DecisionChooseFromCandidates,
		contract.DecisionKnownButUnavailable, contract.DecisionNoGoodMatch,
		contract.DecisionAmbiguous, contract.DecisionCatalogEmpty:
		return true
	default:
		return false
	}
}

// groundedNextStep reports whether a findTool result carries a concrete next
// action: an explicit NextAction tool, or an agentInstruction that names an
// actionable step rather than merely restating the query.
func groundedNextStep(find *contract.FindResult, query string) bool {
	if find == nil {
		return false
	}
	if find.NextAction != nil && find.NextAction.Tool != "" {
		return true
	}
	instr := strings.TrimSpace(find.AgentInstruction)
	if instr == "" {
		return false
	}
	if isQueryRestating(instr, query) {
		return false
	}
	return hasActionableVerb(instr)
}

// isQueryRestating flags an instruction that just echoes the query back without
// telling the agent what to do next.
func isQueryRestating(instr, query string) bool {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return false
	}
	return strings.Contains(strings.ToLower(instr), q) && !hasActionableVerb(instr)
}

// hasActionableVerb reports whether an instruction references a concrete Ozy
// action an agent can take next.
func hasActionableVerb(instr string) bool {
	l := strings.ToLower(instr)
	for _, v := range []string{"describetool", "calltool", "findtool", "ozy ", "refine", "run ", "use ", "inspect", "choose", "check", "index"} {
		if strings.Contains(l, v) {
			return true
		}
	}
	return false
}
