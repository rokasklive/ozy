package eval

import (
	"context"
	"errors"
	"fmt"
	"strings"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/rokasklive/ozy/internal/broker"
	"github.com/rokasklive/ozy/internal/config"
	"github.com/rokasklive/ozy/internal/contract"
	"github.com/rokasklive/ozy/internal/downstream"
	"github.com/rokasklive/ozy/internal/schema"
	"github.com/rokasklive/ozy/internal/search"
)

// InvocationMetrics are the SPEC.md §14.1 invocation/repair metrics over the
// scenario set. Rates are in [0,1]; a rate whose family has no scenarios is 0.
type InvocationMetrics struct {
	N                  int     `json:"n"`
	ValidArgumentRate  float64 `json:"validArgumentRate"`
	FirstCallSuccess   float64 `json:"firstCallSuccess"`
	RepairSuccess      float64 `json:"repairSuccess"`
	SchemaErrorRate    float64 `json:"schemaErrorRate"`
	OfflineHandledRate float64 `json:"offlineHandledRate"`
	ErrorClarity       float64 `json:"errorClarity"`
}

// InvocationReport is the invocation family result.
type InvocationReport struct {
	Overall InvocationMetrics `json:"overall"`
}

// RunInvocation evaluates every invocation scenario over the corpus, driving the
// real broker.CallTool seam against an in-process fixture downstream. Argument
// validation and schema-drift detection are modeled at the harness layer (the
// broker delegates validation downstream today, SPEC.md non-goal); the corrected
// repair call and the offline path run through the real broker. Deterministic.
func RunInvocation(ctx context.Context, corpus *Corpus) (*InvocationReport, error) {
	acc := &invocationAcc{}
	for _, s := range corpus.Invocation {
		if err := scoreInvocation(ctx, corpus, s, acc); err != nil {
			return nil, err
		}
	}
	return &InvocationReport{Overall: acc.metrics()}, nil
}

func scoreInvocation(ctx context.Context, corpus *Corpus, s InvocationScenario, acc *invocationAcc) error {
	tool, ok := corpus.toolByRef(s.ToolRef)
	if !ok {
		return fmt.Errorf("invocation scenario %q references unknown toolRef %q", s.Name, s.ToolRef)
	}
	offline := map[string]bool{}
	if s.ExpectedOutcome == OutcomeError && s.ExpectedError == contract.ErrTypeDownstreamServerOffline {
		server, _ := splitRef(s.ToolRef)
		offline[server] = true
	}
	bk, err := newCorpusBroker(corpus, offline)
	if err != nil {
		return err
	}

	switch s.ExpectedOutcome {
	case OutcomeSuccess:
		acc.nSuccess++
		acc.observeValidExpected(len(schema.Validate(tool.InputSchema, s.Arguments)) == 0)
		res, cerr := callWithModeling(ctx, bk, tool, nil, s.Arguments)
		acc.observeErr(cerr)
		if cerr == nil && res != nil && res.OK {
			acc.successOK++
		}

	case OutcomeRepair:
		acc.nRepair++
		_, firstErr := callWithModeling(ctx, bk, tool, nil, s.Arguments)
		acc.observeErr(firstErr)
		firstClassified := firstErr != nil && firstErr.Type == s.ExpectedError

		acc.observeValidExpected(len(schema.Validate(tool.InputSchema, s.Corrected)) == 0)
		res, correctedErr := callWithModeling(ctx, bk, tool, nil, s.Corrected)
		acc.observeErr(correctedErr)
		if firstClassified && correctedErr == nil && res != nil && res.OK {
			acc.repairOK++
		}

	case OutcomeError:
		_, cerr := callWithModeling(ctx, bk, tool, s.LiveSchema, s.Arguments)
		acc.observeErr(cerr)
		switch s.ExpectedError {
		case contract.ErrTypeToolSchemaChanged:
			acc.nDrift++
			if cerr != nil && cerr.Type == contract.ErrTypeToolSchemaChanged {
				acc.driftOK++
			}
		case contract.ErrTypeDownstreamServerOffline:
			acc.nOffline++
			if cerr != nil && cerr.Type == contract.ErrTypeDownstreamServerOffline {
				acc.offlineOK++
			}
		}
	}
	return nil
}

// callWithModeling models the agent-side schema checks Ozy's broker does not yet
// perform, then drives broker.CallTool for the real downstream interaction:
//
//  1. schema drift — args valid against the cataloged schema but invalid against
//     the live schema → TOOL_SCHEMA_CHANGED (the broker would otherwise surface a
//     confusing downstream error);
//  2. argument validation against the cataloged schema → ARGUMENT_VALIDATION_FAILED;
//  3. otherwise the call is issued through the real broker (where an offline
//     server yields DOWNSTREAM_SERVER_OFFLINE and a healthy server succeeds).
func callWithModeling(ctx context.Context, bk broker.Broker, tool CatalogTool, liveSchema, args map[string]any) (*contract.CallResult, *contract.Error) {
	if len(liveSchema) > 0 {
		validVsCatalog := len(schema.Validate(tool.InputSchema, args)) == 0
		validVsLive := len(schema.Validate(liveSchema, args)) == 0
		if validVsCatalog && !validVsLive {
			return nil, &contract.Error{
				Type:      contract.ErrTypeToolSchemaChanged,
				ToolRef:   tool.ToolRef,
				ServerID:  tool.ServerID,
				Retryable: false,
				Message:   "the cataloged schema for this tool no longer matches the downstream server",
				AgentInstruction: "Run `ozy index` to refresh the catalog, then call describeTool for the current schema " +
					"before retrying. Do not resend the stale arguments.",
			}
		}
	}
	if problems := schema.Validate(tool.InputSchema, args); len(problems) > 0 {
		return nil, &contract.Error{
			Type:      contract.ErrTypeArgumentValidationFailed,
			ToolRef:   tool.ToolRef,
			ServerID:  tool.ServerID,
			Retryable: false,
			Message:   "arguments do not satisfy the tool schema: " + strings.Join(problems, "; "),
			AgentInstruction: "Correct the arguments named in the message and re-call callTool; call describeTool to confirm " +
				"the exact schema first. Do not retry the same arguments unchanged.",
		}
	}
	res, err := bk.CallTool(ctx, tool.ToolRef, args)
	if err != nil {
		return nil, asContractErr(err)
	}
	return res, nil
}

// invocationAcc accumulates raw counts and turns them into rate metrics.
type invocationAcc struct {
	nSuccess, successOK int
	nRepair, repairOK   int
	nDrift, driftOK     int
	nOffline, offlineOK int
	validExpected       int
	validActual         int
	errN, errClear      int
}

func (a *invocationAcc) observeValidExpected(valid bool) {
	a.validExpected++
	if valid {
		a.validActual++
	}
}

func (a *invocationAcc) observeErr(e *contract.Error) {
	if e == nil {
		return
	}
	a.errN++
	if errorIsClear(e) {
		a.errClear++
	}
}

func (a *invocationAcc) metrics() InvocationMetrics {
	rate := func(num, den int) float64 {
		if den == 0 {
			return 0
		}
		return float64(num) / float64(den)
	}
	return InvocationMetrics{
		N:                  a.nSuccess + a.nRepair + a.nDrift + a.nOffline,
		ValidArgumentRate:  rate(a.validActual, a.validExpected),
		FirstCallSuccess:   rate(a.successOK, a.nSuccess),
		RepairSuccess:      rate(a.repairOK, a.nRepair),
		SchemaErrorRate:    rate(a.driftOK, a.nDrift),
		OfflineHandledRate: rate(a.offlineOK, a.nOffline),
		ErrorClarity:       rate(a.errClear, a.errN),
	}
}

// errorIsClear reports the structural §4.5/§9.3 clarity proxy: a structured error
// carries a type, a grounded agentInstruction, and a non-retry-amplifying
// disposition.
func errorIsClear(e *contract.Error) bool {
	if e == nil {
		return false
	}
	return e.Type != "" && strings.TrimSpace(e.AgentInstruction) != "" && isNonAmplifying(e)
}

// isNonAmplifying reports whether an error's retry disposition avoids hot-looping:
// non-retryable errors are inherently safe, and a retryable error must condition
// the retry on a precondition (backoff, a health check, a refresh) rather than
// inviting an immediate, unbounded retry.
func isNonAmplifying(e *contract.Error) bool {
	if e == nil || !e.Retryable {
		return true
	}
	instr := strings.ToLower(e.AgentInstruction)
	for _, w := range []string{"after", "check", "verify", "refresh", "reconnect", "wait", "fix", "backoff", "health", "later"} {
		if strings.Contains(instr, w) {
			return true
		}
	}
	return false
}

// asContractErr unwraps a broker error into its structured form, synthesizing a
// DOWNSTREAM_CALL_FAILED if a non-contract error ever surfaces.
func asContractErr(err error) *contract.Error {
	var ce *contract.Error
	if errors.As(err, &ce) {
		return ce
	}
	return &contract.Error{
		Type:             contract.ErrTypeDownstreamCallFailed,
		Retryable:        false,
		Message:          err.Error(),
		AgentInstruction: "Report this unexpected failure; do not retry blindly.",
	}
}

// newCorpusBroker builds a live broker over the corpus catalog with an in-process
// fixture downstream connector. Servers in offline are made unreachable so the
// offline path can be exercised through the real broker.CallTool seam.
func newCorpusBroker(corpus *Corpus, offline map[string]bool) (broker.Broker, error) {
	store, err := corpus.Store()
	if err != nil {
		return nil, err
	}
	return broker.NewLive(store, corpusConfig(corpus), &fixtureConnector{offline: offline}, search.New(store, nil)), nil
}

// corpusConfig enables every corpus server as a memory-backed remote so the
// broker's server lookup resolves; the fixture connector supplies the sessions.
func corpusConfig(corpus *Corpus) *config.Config {
	mcp := make(map[string]config.ServerConfig, len(corpus.Catalog.Servers))
	for _, s := range corpus.Catalog.Servers {
		mcp[s.ID] = config.ServerConfig{Type: "remote", URL: "memory", Enabled: true}
	}
	return &config.Config{MCP: mcp}
}

// fixtureConnector is an in-process broker.Connector: it returns a fixture
// session for healthy servers and a structured offline error for servers marked
// offline, so the harness drives the real broker without a live downstream.
type fixtureConnector struct {
	offline map[string]bool
}

func (c *fixtureConnector) ConnectAll(context.Context, *config.Config) []downstream.Result {
	return nil
}

func (c *fixtureConnector) Connect(_ context.Context, serverID string, _ config.ServerConfig) downstream.Result {
	if c.offline[serverID] {
		return downstream.Result{
			ServerID: serverID,
			Error: &contract.Error{
				Type:             contract.ErrTypeDownstreamServerOffline,
				ServerID:         serverID,
				Retryable:        true,
				Message:          fmt.Sprintf("could not connect to server %q", serverID),
				AgentInstruction: "Retry after the server is reachable again; verify connectivity with `ozy doctor`. Do not retry in a tight loop.",
			},
		}
	}
	return downstream.Result{ServerID: serverID, Session: fixtureSession{}}
}

// fixtureSession is a downstream.Session that succeeds for any call, echoing the
// invoked tool name. The harness only reaches it with schema-valid arguments, so
// "downstream ran" is the correct outcome.
type fixtureSession struct{}

func (fixtureSession) ListTools(context.Context, *mcpsdk.ListToolsParams) (*mcpsdk.ListToolsResult, error) {
	return &mcpsdk.ListToolsResult{}, nil
}

func (fixtureSession) CallTool(_ context.Context, params *mcpsdk.CallToolParams) (*mcpsdk.CallToolResult, error) {
	name := ""
	if params != nil {
		name = params.Name
	}
	return &mcpsdk.CallToolResult{
		Content: []mcpsdk.Content{&mcpsdk.TextContent{Text: fmt.Sprintf("ok: %s executed in fixture downstream", name)}},
	}, nil
}

func (fixtureSession) Close() error { return nil }
