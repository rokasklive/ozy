// Package mcp adapts Ozy's broker to the agent-facing MCP surface.
//
// It registers exactly the three stable Ozy tools — findTool, describeTool, and
// callTool (SPEC.md §4.3, §9) — and exposes no downstream tools, preserving the
// small-surface and capability-brokerage principles. The official MCP Go SDK is
// kept behind this package so the broker never imports it and a library swap
// touches only this file.
package mcp

import (
	"context"
	"encoding/json"
	"errors"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/rokasklive/ozy/internal/broker"
	"github.com/rokasklive/ozy/internal/contract"
)

// Tool descriptions are written to make Ozy the obvious first reach: they name
// concrete capabilities and tell the agent when to prefer the broker over
// guessing or its built-in tools, so a small surface still wins the tool
// selection. internal/eval/economy.go references these constants so its
// token-economy estimate mirrors the real advertised surface.
const (
	OzyFindDescription = "Discover the right hidden tool for the current task. Ozy performs semantic " +
		"capability search over downstream MCP tools that are not loaded into your context up front, " +
		"then returns a small decision payload: the best matching toolRef, why it matches, and the " +
		"next call shape to use.\n\n" +
		"Use this first when you need code search, documentation lookup, git/history inspection, " +
		"database/query access, file intelligence, external service calls, or any capability beyond " +
		"your built-in tools. This avoids guessing tool names, loading large schemas, or spending " +
		"context on irrelevant tools. Prefer this before broad shell exploration when the needed " +
		"information may be available through the environment's tool catalog."

	OzyDescribeDescription = "Get everything needed to call a downstream tool correctly on the first " +
		"try: its exact input schema, what each argument means, usage guidance, and a recommended " +
		"call shape. Run it on the toolRef returned by findTool before calling, so you never have to " +
		"guess at arguments."

	OzyCallDescription = "Actually run a downstream tool through Ozy — execute the query, read the " +
		"file, search the history, hit the API. This is how a capability you found with findTool gets " +
		"performed: pass the toolRef and its arguments, and Ozy validates and routes the call to the " +
		"live downstream server."
)

// Adapter serves the Ozy MCP tools by delegating to a shared broker.
type Adapter struct {
	broker  broker.Broker
	version string
}

// New returns an MCP adapter backed by the given broker.
func New(b broker.Broker, version string) *Adapter {
	return &Adapter{broker: b, version: version}
}

type findInput struct {
	Query string `json:"query" jsonschema:"capability query describing the tool you need"`
}

type describeInput struct {
	ToolRef string `json:"toolRef" jsonschema:"stable Ozy tool reference, e.g. atlassian.confluence_search"`
}

type callInput struct {
	ToolRef   string         `json:"toolRef" jsonschema:"stable Ozy tool reference to invoke"`
	Arguments map[string]any `json:"arguments,omitempty" jsonschema:"arguments to pass to the downstream tool"`
}

// Server builds an MCP server with the three Ozy tools registered. Only these
// tools are advertised; downstream tools are never exposed directly.
func (a *Adapter) Server() *mcpsdk.Server {
	s := mcpsdk.NewServer(&mcpsdk.Implementation{
		Name:    "ozy",
		Title:   "Ozy capability broker",
		Version: a.version,
	}, nil)

	mcpsdk.AddTool(s, &mcpsdk.Tool{
		Name:        "findTool",
		Title:       "Find a tool for a capability",
		Description: OzyFindDescription,
	}, a.handleFind)

	mcpsdk.AddTool(s, &mcpsdk.Tool{
		Name:        "describeTool",
		Title:       "Describe a known tool",
		Description: OzyDescribeDescription,
	}, a.handleDescribe)

	mcpsdk.AddTool(s, &mcpsdk.Tool{
		Name:        "callTool",
		Title:       "Invoke a tool through Ozy",
		Description: OzyCallDescription,
	}, a.handleCall)

	return s
}

// Serve runs the adapter over stdio until the client disconnects or ctx is done.
func (a *Adapter) Serve(ctx context.Context) error {
	return a.Server().Run(ctx, &mcpsdk.StdioTransport{})
}

func (a *Adapter) handleFind(ctx context.Context, _ *mcpsdk.CallToolRequest, in findInput) (*mcpsdk.CallToolResult, any, error) {
	res, _ := a.broker.FindTool(ctx, in.Query)
	return jsonResult(res, false), nil, nil
}

func (a *Adapter) handleDescribe(ctx context.Context, _ *mcpsdk.CallToolRequest, in describeInput) (*mcpsdk.CallToolResult, any, error) {
	res, err := a.broker.DescribeTool(ctx, in.ToolRef)
	if err != nil {
		return jsonResult(contract.NewErrorEnvelope(asContractError(err)), true), nil, nil
	}
	return jsonResult(res, false), nil, nil
}

func (a *Adapter) handleCall(ctx context.Context, _ *mcpsdk.CallToolRequest, in callInput) (*mcpsdk.CallToolResult, any, error) {
	res, err := a.broker.CallTool(ctx, in.ToolRef, in.Arguments)
	if err != nil {
		return jsonResult(contract.NewErrorEnvelope(asContractError(err)), true), nil, nil
	}
	return callResult(res), nil, nil
}

// callResult surfaces a successful callTool payload directly: a textual
// downstream result becomes readable text content, a structured result is
// preserved as structured content (with a JSON rendering for text-only
// clients), and Ozy's call metadata (toolRef, resultSummary, any nextActions)
// rides in _meta exactly once — never a second stringified copy of the whole
// §9.3 envelope wrapped around the payload.
func callResult(res *contract.CallResult) *mcpsdk.CallToolResult {
	out := &mcpsdk.CallToolResult{
		Meta: mcpsdk.Meta{
			"toolRef":       res.ToolRef,
			"resultSummary": res.ResultSummary,
		},
	}
	if len(res.NextActions) > 0 {
		out.Meta["nextActions"] = res.NextActions
	}
	switch v := res.Result.(type) {
	case nil:
		out.Content = []mcpsdk.Content{&mcpsdk.TextContent{Text: ""}}
	case string:
		out.Content = []mcpsdk.Content{&mcpsdk.TextContent{Text: v}}
	default:
		out.StructuredContent = v
		if data, err := json.Marshal(v); err == nil {
			out.Content = []mcpsdk.Content{&mcpsdk.TextContent{Text: string(data)}}
		} else {
			out.Content = []mcpsdk.Content{&mcpsdk.TextContent{Text: res.ResultSummary}}
		}
	}
	return out
}

// jsonResult wraps a contract value as a CallToolResult, carrying it both as
// structured content and as JSON text so any MCP client can read it.
func jsonResult(v any, isError bool) *mcpsdk.CallToolResult {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		data = []byte(`{"ok":false,"error":{"type":"DOWNSTREAM_CALL_FAILED","message":"failed to encode response"}}`)
		isError = true
	}
	return &mcpsdk.CallToolResult{
		Content:           []mcpsdk.Content{&mcpsdk.TextContent{Text: string(data)}},
		StructuredContent: v,
		IsError:           isError,
	}
}

// asContractError recovers the structured error, synthesizing one if the broker
// ever returns a non-contract error so the response stays §9.3-shaped.
func asContractError(err error) *contract.Error {
	var ce *contract.Error
	if errors.As(err, &ce) {
		return ce
	}
	return &contract.Error{
		Type:             contract.ErrTypeDownstreamCallFailed,
		Retryable:        false,
		Message:          err.Error(),
		AgentInstruction: "Report this unexpected failure to the user; do not retry blindly.",
	}
}
