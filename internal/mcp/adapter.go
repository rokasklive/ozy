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

	"github.com/rokask/ozy/internal/broker"
	"github.com/rokask/ozy/internal/contract"
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
		Description: "Find the best known downstream tool for a capability query. Returns a decision, not just a list.",
	}, a.handleFind)

	mcpsdk.AddTool(s, &mcpsdk.Tool{
		Name:        "describeTool",
		Title:       "Describe a known tool",
		Description: "Return the exact schema, usage guidance, and recommended call shape for one known toolRef.",
	}, a.handleDescribe)

	mcpsdk.AddTool(s, &mcpsdk.Tool{
		Name:        "callTool",
		Title:       "Invoke a tool through Ozy",
		Description: "Invoke a selected downstream tool through Ozy. Invocation is live-gated.",
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
	return jsonResult(res, false), nil, nil
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
