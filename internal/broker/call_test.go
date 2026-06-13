package broker

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/rokasklive/ozy/internal/catalog"
	"github.com/rokasklive/ozy/internal/config"
	"github.com/rokasklive/ozy/internal/contract"
	"github.com/rokasklive/ozy/internal/downstream"
	"github.com/rokasklive/ozy/internal/search"
)

// callSession is a fake downstream.Session that returns a configured
// *mcpsdk.CallToolResult from CallTool. It records the params and the
// arguments map, and tracks Close calls.
type callSession struct {
	res     *mcpsdk.CallToolResult
	err     error
	gotName atomic.Value // string
	gotArgs atomic.Value // map[string]any
	closed  atomic.Bool
}

func (c *callSession) ListTools(context.Context, *mcpsdk.ListToolsParams) (*mcpsdk.ListToolsResult, error) {
	return &mcpsdk.ListToolsResult{}, nil
}

func (c *callSession) CallTool(_ context.Context, params *mcpsdk.CallToolParams) (*mcpsdk.CallToolResult, error) {
	if params != nil {
		c.gotName.Store(params.Name)
		if params.Arguments != nil {
			if m, ok := params.Arguments.(map[string]any); ok {
				c.gotArgs.Store(m)
			}
		}
	}
	return c.res, c.err
}

func (c *callSession) Close() error {
	c.closed.Store(true)
	return nil
}

// callConnector is a fake broker.Connector that returns a fixed Connect result
// and an empty ConnectAll. It records which serverIDs it was asked to connect
// to so tests can assert only-the-target-server behavior.
type callConnector struct {
	connectResult downstream.Result
	connectedIDs  atomic.Value // []string
}

func (c *callConnector) ConnectAll(context.Context, *config.Config) []downstream.Result {
	return nil
}

func (c *callConnector) Connect(_ context.Context, serverID string, _ config.ServerConfig) downstream.Result {
	ids, _ := c.connectedIDs.Load().([]string)
	c.connectedIDs.Store(append(ids, serverID))
	return c.connectResult
}

func newCallBroker(t *testing.T, cfg *config.Config, conn Connector) Broker {
	t.Helper()
	return NewLive(catalog.NewMemory(), cfg, conn, search.New(catalog.NewMemory(), nil))
}

func TestCallTool_SuccessfulInvocationNormalizesAndSummarizes(t *testing.T) {
	t.Parallel()
	session := &callSession{
		res: &mcpsdk.CallToolResult{
			Content: []mcpsdk.Content{
				&mcpsdk.TextContent{Text: "first line\nsecond line"},
			},
		},
	}
	connector := &callConnector{connectResult: downstream.Result{ServerID: "atlassian", Session: session}}
	b := newCallBroker(t, &config.Config{
		MCP: map[string]config.ServerConfig{
			"atlassian": {Type: "remote", URL: "memory", Enabled: true},
			"other":     {Type: "remote", URL: "memory", Enabled: true},
		},
	}, connector)

	res, err := b.CallTool(context.Background(), "atlassian.confluence_search", map[string]any{"q": "billing"})
	if err != nil {
		t.Fatalf("CallTool() unexpected error = %v", err)
	}
	if !res.OK {
		t.Errorf("CallResult.OK = false, want true")
	}
	if res.ToolRef != "atlassian.confluence_search" {
		t.Errorf("ToolRef = %q, want atlassian.confluence_search", res.ToolRef)
	}
	if res.Result != "first line\nsecond line" {
		t.Errorf("Result = %v, want text content joined", res.Result)
	}
	if res.ResultSummary == "" {
		t.Error("ResultSummary must be non-empty")
	}
	if got, _ := session.gotName.Load().(string); got != "confluence_search" {
		t.Errorf("downstream CallTool Name = %q, want confluence_search", got)
	}
	if got, _ := session.gotArgs.Load().(map[string]any); got["q"] != "billing" {
		t.Errorf("downstream CallTool Arguments = %v, want q=billing", got)
	}
	if !session.closed.Load() {
		t.Error("session.Close() was not called after successful call")
	}
	ids, _ := connector.connectedIDs.Load().([]string)
	if len(ids) != 1 || ids[0] != "atlassian" {
		t.Errorf("connected to %v, want only [atlassian]", ids)
	}
	if res.ResultSummary == "" || !strings.Contains(res.ResultSummary, "first line") {
		t.Errorf("ResultSummary should describe the result, got %q", res.ResultSummary)
	}
}

func TestCallTool_StructuredContentPreferredOverText(t *testing.T) {
	t.Parallel()
	session := &callSession{
		res: &mcpsdk.CallToolResult{
			Content: []mcpsdk.Content{
				&mcpsdk.TextContent{Text: "ignored when structured is set"},
			},
			StructuredContent: map[string]any{"answer": 42},
		},
	}
	connector := &callConnector{connectResult: downstream.Result{ServerID: "atlassian", Session: session}}
	b := newCallBroker(t, &config.Config{
		MCP: map[string]config.ServerConfig{
			"atlassian": {Type: "remote", URL: "memory", Enabled: true},
		},
	}, connector)

	res, err := b.CallTool(context.Background(), "atlassian.calculator", map[string]any{"expr": "6*7"})
	if err != nil {
		t.Fatalf("CallTool() unexpected error = %v", err)
	}
	got, ok := res.Result.(map[string]any)
	if !ok {
		t.Fatalf("Result = %T, want map[string]any", res.Result)
	}
	if got["answer"] != 42 {
		t.Errorf("Result[answer] = %v, want 42", got["answer"])
	}
	if !strings.Contains(res.ResultSummary, "structured content") {
		t.Errorf("ResultSummary should mention structured content, got %q", res.ResultSummary)
	}
}

func TestCallTool_MalformedToolRefReturnsToolNotFound(t *testing.T) {
	t.Parallel()
	b := newCallBroker(t, &config.Config{
		MCP: map[string]config.ServerConfig{
			"atlassian": {Type: "remote", URL: "memory", Enabled: true},
		},
	}, &callConnector{})

	_, err := b.CallTool(context.Background(), "noDotHere", map[string]any{})
	var ce *contract.Error
	if !errors.As(err, &ce) {
		t.Fatalf("CallTool() error = %v, want *contract.Error", err)
	}
	if ce.Type != contract.ErrTypeToolNotFound {
		t.Errorf("error type = %q, want %q", ce.Type, contract.ErrTypeToolNotFound)
	}
	if ce.AgentInstruction == "" {
		t.Error("malformed-toolRef failure must carry an agentInstruction")
	}
	if !strings.Contains(strings.ToLower(ce.AgentInstruction), "findtool") {
		t.Errorf("agentInstruction should direct the agent to findTool, got %q", ce.AgentInstruction)
	}
}

func TestCallTool_UnknownServerReturnsConfigError(t *testing.T) {
	t.Parallel()
	b := newCallBroker(t, &config.Config{
		MCP: map[string]config.ServerConfig{
			"atlassian": {Type: "remote", URL: "memory", Enabled: true},
		},
	}, &callConnector{})

	_, err := b.CallTool(context.Background(), "ghost.missing", map[string]any{})
	var ce *contract.Error
	if !errors.As(err, &ce) {
		t.Fatalf("CallTool() error = %v, want *contract.Error", err)
	}
	if ce.Type != contract.ErrTypeConfigError {
		t.Errorf("error type = %q, want %q", ce.Type, contract.ErrTypeConfigError)
	}
	if ce.ServerID != "ghost" {
		t.Errorf("ServerID = %q, want ghost", ce.ServerID)
	}
	if ce.AgentInstruction == "" {
		t.Error("unknown-server failure must carry an agentInstruction")
	}
}

func TestCallTool_DisabledServerReturnsConfigError(t *testing.T) {
	t.Parallel()
	b := newCallBroker(t, &config.Config{
		MCP: map[string]config.ServerConfig{
			"atlassian": {Type: "remote", URL: "memory", Enabled: false},
		},
	}, &callConnector{})

	_, err := b.CallTool(context.Background(), "atlassian.confluence_search", map[string]any{})
	var ce *contract.Error
	if !errors.As(err, &ce) {
		t.Fatalf("CallTool() error = %v, want *contract.Error", err)
	}
	if ce.Type != contract.ErrTypeConfigError {
		t.Errorf("error type = %q, want %q", ce.Type, contract.ErrTypeConfigError)
	}
	if ce.ServerID != "atlassian" {
		t.Errorf("ServerID = %q, want atlassian", ce.ServerID)
	}
	if ce.AgentInstruction == "" {
		t.Error("disabled-server failure must carry an agentInstruction")
	}
}

func TestCallTool_UnreachableServerReturnsDownstreamServerOffline(t *testing.T) {
	t.Parallel()
	const secret = "Bearer supersecretvalue"
	connector := &callConnector{
		connectResult: downstream.Result{
			ServerID: "atlassian",
			Error: &contract.Error{
				Type:             contract.ErrTypeDownstreamServerOffline,
				ServerID:         "atlassian",
				Retryable:        true,
				Message:          "could not connect to server \"atlassian\": dial " + secret,
				AgentInstruction: "retry later",
			},
		},
	}
	b := newCallBroker(t, &config.Config{
		MCP: map[string]config.ServerConfig{
			"atlassian": {Type: "remote", URL: "memory", Headers: map[string]string{"Authorization": secret}, Enabled: true},
		},
	}, connector)

	_, err := b.CallTool(context.Background(), "atlassian.confluence_search", map[string]any{})
	var ce *contract.Error
	if !errors.As(err, &ce) {
		t.Fatalf("CallTool() error = %v, want *contract.Error", err)
	}
	if ce.Type != contract.ErrTypeDownstreamServerOffline {
		t.Errorf("error type = %q, want %q", ce.Type, contract.ErrTypeDownstreamServerOffline)
	}
	if strings.Contains(ce.Message, "supersecretvalue") {
		t.Fatalf("downstream offline error leaked secret: %+v", ce)
	}
	if ce.AgentInstruction == "" {
		t.Error("unreachable-server failure must carry an agentInstruction")
	}
}

func TestCallTool_DownstreamToolErrorReturnsDownstreamCallFailed(t *testing.T) {
	t.Parallel()
	const secret = "Bearer supersecretvalue"
	session := &callSession{
		res: &mcpsdk.CallToolResult{
			Content: []mcpsdk.Content{&mcpsdk.TextContent{Text: "downstream rejected request: " + secret}},
			IsError: true,
		},
	}
	connector := &callConnector{connectResult: downstream.Result{ServerID: "atlassian", Session: session}}
	b := newCallBroker(t, &config.Config{
		MCP: map[string]config.ServerConfig{
			"atlassian": {
				Type:    "remote",
				URL:     "memory",
				Headers: map[string]string{"Authorization": secret},
				Enabled: true,
			},
		},
	}, connector)

	_, err := b.CallTool(context.Background(), "atlassian.confluence_search", map[string]any{})
	var ce *contract.Error
	if !errors.As(err, &ce) {
		t.Fatalf("CallTool() error = %v, want *contract.Error", err)
	}
	if ce.Type != contract.ErrTypeDownstreamCallFailed {
		t.Errorf("error type = %q, want %q", ce.Type, contract.ErrTypeDownstreamCallFailed)
	}
	if ce.Retryable {
		t.Error("downstream-tool error must not advertise retryable=true (avoids retry amplification)")
	}
	if strings.Contains(ce.Message, secret) {
		t.Fatalf("downstream tool error leaked secret: %+v", ce)
	}
	if ce.AgentInstruction == "" {
		t.Error("downstream-tool failure must carry an agentInstruction")
	}
}

func TestCallTool_TransportErrorDuringCallIsRedactedAndRetryable(t *testing.T) {
	t.Parallel()
	const secret = "Bearer supersecretvalue"
	session := &callSession{
		err: errors.New("rpc closed during call: " + secret),
	}
	connector := &callConnector{connectResult: downstream.Result{ServerID: "atlassian", Session: session}}
	b := newCallBroker(t, &config.Config{
		MCP: map[string]config.ServerConfig{
			"atlassian": {
				Type:    "remote",
				URL:     "memory",
				Headers: map[string]string{"Authorization": secret},
				Enabled: true,
			},
		},
	}, connector)

	_, err := b.CallTool(context.Background(), "atlassian.confluence_search", map[string]any{})
	var ce *contract.Error
	if !errors.As(err, &ce) {
		t.Fatalf("CallTool() error = %v, want *contract.Error", err)
	}
	if ce.Type != contract.ErrTypeDownstreamCallFailed {
		t.Errorf("error type = %q, want %q", ce.Type, contract.ErrTypeDownstreamCallFailed)
	}
	if !ce.Retryable {
		t.Error("transport error during call should be retryable")
	}
	if strings.Contains(ce.Message, secret) {
		t.Fatalf("transport error leaked secret: %+v", ce)
	}
}

func TestCallTool_ResultExceedingBudgetIsTruncated(t *testing.T) {
	t.Parallel()
	largeText := strings.Repeat("x", 2048)
	session := &callSession{
		res: &mcpsdk.CallToolResult{
			Content: []mcpsdk.Content{&mcpsdk.TextContent{Text: largeText}},
		},
	}
	connector := &callConnector{connectResult: downstream.Result{ServerID: "atlassian", Session: session}}
	b := newCallBroker(t, &config.Config{
		MCP: map[string]config.ServerConfig{
			"atlassian": {Type: "remote", URL: "memory", Enabled: true},
		},
		Budgets: config.BudgetsConfig{CallTool: config.CallToolBudget{MaxResultBytes: 128}},
	}, connector)

	res, err := b.CallTool(context.Background(), "atlassian.confluence_search", map[string]any{})
	if err != nil {
		t.Fatalf("CallTool() unexpected error = %v", err)
	}
	if !res.OK {
		t.Error("OK = false, want true (truncation is not a call failure)")
	}
	truncated, ok := res.Result.(string)
	if !ok {
		t.Fatalf("Result = %T, want string (truncated text)", res.Result)
	}
	if !strings.HasSuffix(truncated, "...[truncated]") {
		t.Errorf("truncated Result must end with ...[truncated], got %q", truncated[len(truncated)-32:])
	}
	if !strings.Contains(res.ResultSummary, "maxResultBytes") || !strings.Contains(res.ResultSummary, "128") {
		t.Errorf("ResultSummary should mention the budget, got %q", res.ResultSummary)
	}
}

func TestCallTool_ResultWithinBudgetIsUnchanged(t *testing.T) {
	t.Parallel()
	session := &callSession{
		res: &mcpsdk.CallToolResult{
			Content: []mcpsdk.Content{&mcpsdk.TextContent{Text: "small"}},
		},
	}
	connector := &callConnector{connectResult: downstream.Result{ServerID: "atlassian", Session: session}}
	b := newCallBroker(t, &config.Config{
		MCP: map[string]config.ServerConfig{
			"atlassian": {Type: "remote", URL: "memory", Enabled: true},
		},
		Budgets: config.BudgetsConfig{CallTool: config.CallToolBudget{MaxResultBytes: 1024}},
	}, connector)

	res, err := b.CallTool(context.Background(), "atlassian.confluence_search", map[string]any{})
	if err != nil {
		t.Fatalf("CallTool() unexpected error = %v", err)
	}
	if res.Result != "small" {
		t.Errorf("Result = %v, want small", res.Result)
	}
	if strings.Contains(res.ResultSummary, "truncated") {
		t.Errorf("ResultSummary should not mention truncation, got %q", res.ResultSummary)
	}
}
