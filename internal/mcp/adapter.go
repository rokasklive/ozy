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
	"fmt"
	"sort"
	"strings"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/rokasklive/ozy/internal/broker"
	"github.com/rokasklive/ozy/internal/contract"
)

// MaxBreadcrumbServers bounds the server list appended to the findTool
// description so the always-loaded agent surface stays small.
const MaxBreadcrumbServers = 12

// Breadcrumb renders a bounded, sorted summary of the given downstream server
// ids for the findTool description. It returns "" when there are no servers, so
// callers can append it unconditionally.
func Breadcrumb(servers []string) string {
	if len(servers) == 0 {
		return ""
	}
	sorted := append([]string(nil), servers...)
	sort.Strings(sorted)
	shown, overflow := sorted, 0
	if len(sorted) > MaxBreadcrumbServers {
		shown, overflow = sorted[:MaxBreadcrumbServers], len(sorted)-MaxBreadcrumbServers
	}
	out := "Available downstream servers (use findTool to reach their tools): " + strings.Join(shown, ", ")
	if overflow > 0 {
		out += fmt.Sprintf(" (+%d more)", overflow)
	}
	return out + "."
}

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

// BrokerProvider yields the current shared broker. The adapter reads it per
// request rather than capturing it once, so a background sidecar provisioning
// pass that swaps the runtime's broker (lexical -> hybrid) is seen immediately
// without reconstructing the adapter. *daemon.Daemon satisfies this.
type BrokerProvider interface {
	Broker() broker.Broker
}

// staticProvider wraps a fixed broker for callers and tests that do not swap.
type staticProvider struct{ b broker.Broker }

func (s staticProvider) Broker() broker.Broker { return s.b }

// StaticProvider adapts a fixed broker to a BrokerProvider for callers that
// never swap (evals, tests).
func StaticProvider(b broker.Broker) BrokerProvider { return staticProvider{b} }

// Adapter serves the Ozy MCP tools by delegating to a shared broker read from
// the provider on each call.
type Adapter struct {
	provider        BrokerProvider
	version         string
	findDescription string
}

// New returns an MCP adapter backed by the given broker provider. A non-empty
// breadcrumb (see Breadcrumb) is appended to the findTool description so the
// agent sees the available downstream servers before its first call.
func New(p BrokerProvider, version, breadcrumb string) *Adapter {
	desc := OzyFindDescription
	if breadcrumb != "" {
		desc += "\n\n" + breadcrumb
	}
	return &Adapter{provider: p, version: version, findDescription: desc}
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
		Description: a.findDescription,
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
	res, _ := a.provider.Broker().FindTool(ctx, in.Query)
	return jsonResult(res, false), nil, nil
}

func (a *Adapter) handleDescribe(ctx context.Context, _ *mcpsdk.CallToolRequest, in describeInput) (*mcpsdk.CallToolResult, any, error) {
	res, err := a.provider.Broker().DescribeTool(ctx, in.ToolRef)
	if err != nil {
		return jsonResult(contract.NewErrorEnvelope(asContractError(err)), true), nil, nil
	}
	return jsonResult(res, false), nil, nil
}

func (a *Adapter) handleCall(ctx context.Context, _ *mcpsdk.CallToolRequest, in callInput) (*mcpsdk.CallToolResult, any, error) {
	res, err := a.provider.Broker().CallTool(ctx, in.ToolRef, in.Arguments)
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
		// Carry the structured payload once, as compact JSON in content. Ozy's
		// tools declare no outputSchema, so a second StructuredContent copy would
		// only duplicate the same bytes for an agent that reads content text.
		if data, err := json.Marshal(v); err == nil {
			out.Content = []mcpsdk.Content{&mcpsdk.TextContent{Text: string(data)}}
		} else {
			out.Content = []mcpsdk.Content{&mcpsdk.TextContent{Text: res.ResultSummary}}
		}
	}
	return out
}

// jsonResult wraps a contract value as a CallToolResult, carrying it once as
// compact JSON text in content. Ozy's tools declare no outputSchema, so a
// duplicate StructuredContent copy would only repeat the same payload to an
// agent that reads content text; pretty-printing would likewise spend tokens on
// whitespace the agent does not need.
func jsonResult(v any, isError bool) *mcpsdk.CallToolResult {
	data, err := json.Marshal(v)
	if err != nil {
		data = []byte(`{"ok":false,"error":{"type":"DOWNSTREAM_CALL_FAILED","message":"failed to encode response"}}`)
		isError = true
	}
	return &mcpsdk.CallToolResult{
		Content: []mcpsdk.Content{&mcpsdk.TextContent{Text: string(data)}},
		IsError: isError,
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
