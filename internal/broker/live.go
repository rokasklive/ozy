package broker

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/rokasklive/ozy/internal/catalog"
	"github.com/rokasklive/ozy/internal/config"
	"github.com/rokasklive/ozy/internal/contract"
	"github.com/rokasklive/ozy/internal/downstream"
)

// Connector is the downstream seam used by the live broker. ConnectAll powers
// FindTool; Connect is the single-server connection used by CallTool so only
// the target server is contacted for a single invocation.
type Connector interface {
	ConnectAll(ctx context.Context, cfg *config.Config) []downstream.Result
	Connect(ctx context.Context, serverID string, server config.ServerConfig) downstream.Result
}

// live is the broker that performs live downstream tool discovery for FindTool
// and live brokered invocation for CallTool, while delegating describeTool and
// List to the skeleton backed by the catalog store.
type live struct {
	skeleton  *skeleton
	cfg       *config.Config
	connector Connector
}

// NewLive returns a Broker that discovers tools live from configured downstream
// MCP servers when FindTool is called and performs live brokered invocation
// when CallTool is called. describeTool and List remain catalog-backed.
func NewLive(store catalog.Store, cfg *config.Config, connector Connector) Broker {
	return &live{
		skeleton:  &skeleton{store: store},
		cfg:       cfg,
		connector: connector,
	}
}

func (l *live) FindTool(ctx context.Context, _ string) (*contract.FindResult, error) {
	results := l.connector.ConnectAll(ctx, l.cfg)

	var (
		candidates []contract.Candidate
		errors     []contract.Error
		anyReached bool
		anyTools   bool
		anySkipped bool
		anyFailed  bool
	)

	for _, r := range results {
		if r.Skipped {
			anySkipped = true
			continue
		}
		if r.Error != nil {
			anyFailed = true
			errors = append(errors, *r.Error)
			continue
		}
		if r.Session == nil {
			anyFailed = true
			errors = append(errors, contract.Error{
				Type:             contract.ErrTypeDownstreamServerOffline,
				ServerID:         r.ServerID,
				Retryable:        true,
				Message:          "downstream connector returned no session",
				AgentInstruction: "Retry after checking the server connection.",
			})
			continue
		}

		anyReached = true
		server := l.serverConfig(r.ServerID)
		tools, err := l.listSessionTools(ctx, r.ServerID, server, r.Session)
		_ = r.Session.Close()

		if err != nil {
			anyFailed = true
			errors = append(errors, *err)
			continue
		}

		for _, tool := range tools {
			candidates = append(candidates, l.normalizeCandidate(r.ServerID, tool))
			anyTools = true
		}
	}

	switch {
	case !anyReached && !anyFailed:
		if anySkipped {
			return &contract.FindResult{
				Decision:         contract.DecisionNoGoodMatch,
				AgentInstruction: "No enabled downstream MCP servers were found. Enable at least one server in your Ozy config and retry.",
			}, nil
		}
		return &contract.FindResult{
			Decision:         contract.DecisionCatalogEmpty,
			AgentInstruction: "No downstream MCP servers are configured. Add servers to your Ozy config and retry.",
		}, nil

	case !anyReached && anyFailed:
		return &contract.FindResult{
			Decision:         contract.DecisionKnownButUnavailable,
			Errors:           errors,
			AgentInstruction: "All configured downstream servers failed to respond. Review the per-server errors below, check connectivity and credentials, then retry. Do not fabricate tool calls.",
		}, nil

	case anyTools && anyFailed:
		toolList := candidateRefs(candidates)
		return &contract.FindResult{
			Decision:         contract.DecisionChooseFromCandidates,
			Candidates:       candidates,
			Errors:           errors,
			AgentInstruction: fmt.Sprintf("Some servers failed. The tool list below is partial — review errors before selecting. Available tools: %s", toolList),
		}, nil

	case anyTools:
		toolList := candidateRefs(candidates)
		return &contract.FindResult{
			Decision:         contract.DecisionChooseFromCandidates,
			Candidates:       candidates,
			AgentInstruction: fmt.Sprintf("All configured servers responded. Select the most relevant tool from below, then call describeTool for its full schema. Available tools: %s", toolList),
		}, nil

	case !anyTools && anyFailed:
		return &contract.FindResult{
			Decision:         contract.DecisionKnownButUnavailable,
			Errors:           errors,
			AgentInstruction: "No downstream tools were discovered and some servers failed. Review the per-server errors below, check downstream server capabilities, then retry.",
		}, nil

	default:
		// Reached servers but zero tools returned.
		return &contract.FindResult{
			Decision:         contract.DecisionNoGoodMatch,
			AgentInstruction: "All configured downstream servers were reachable but returned zero tools. Check that your downstream MCP servers expose tools via tools/list and that they are correctly configured. Do not invent tools.",
		}, nil
	}
}

func (l *live) DescribeTool(ctx context.Context, toolRef string) (*contract.DescribeResult, error) {
	return l.skeleton.DescribeTool(ctx, toolRef)
}

func (l *live) CallTool(ctx context.Context, toolRef string, args map[string]any) (*contract.CallResult, error) {
	serverID, downstreamName, ok := splitToolRef(toolRef)
	if !ok {
		return nil, malformedToolRef(toolRef)
	}
	server, ok := l.serverLookup(serverID)
	if !ok {
		return nil, unknownServer(toolRef, serverID)
	}
	if !server.IsEnabled() {
		return nil, disabledServer(toolRef, serverID)
	}

	callCtx, cancel := context.WithTimeout(ctx, server.DiscoveryTimeout())
	defer cancel()

	result := l.connector.Connect(callCtx, serverID, server)
	if result.Error != nil {
		result.Error.ToolRef = toolRef
		result.Error.Message = downstream.Scrub(result.Error.Message, server)
		return nil, result.Error
	}
	if result.Session == nil {
		return nil, &contract.Error{
			Type:             contract.ErrTypeDownstreamServerOffline,
			ServerID:         serverID,
			ToolRef:          toolRef,
			Retryable:        true,
			Message:          "downstream connector returned no session",
			AgentInstruction: "Retry after checking the server connection.",
		}
	}
	defer func() { _ = result.Session.Close() }()

	callRes, err := result.Session.CallTool(callCtx, &mcpsdk.CallToolParams{
		Name:      downstreamName,
		Arguments: args,
	})
	if err != nil {
		return nil, &contract.Error{
			Type:     contract.ErrTypeDownstreamCallFailed,
			ServerID: serverID,
			ToolRef:  toolRef,
			Retryable: true,
			Message:          fmt.Sprintf("tools/call failed on server %q: %v", serverID, downstream.Scrub(err.Error(), server)),
			AgentInstruction: "Check the downstream server health and the call arguments, then retry.",
		}
	}
	if callRes.IsError {
		return nil, &contract.Error{
			Type:     contract.ErrTypeDownstreamCallFailed,
			ServerID: serverID,
			ToolRef:  toolRef,
			Retryable: false,
			Message:          downstream.Scrub(extractErrorText(callRes), server),
			AgentInstruction: "Inspect the downstream error and the call arguments; do not retry the same call unchanged.",
		}
	}

	normalized := normalizeCallResult(callRes)
	summary := summarizeCall(downstreamName, normalized)
	if truncated, marker, ok := applyCallBudget(l.cfg, normalized); ok {
		return &contract.CallResult{
			OK:            true,
			ToolRef:       toolRef,
			Result:        truncated,
			ResultSummary: summary + " " + marker,
		}, nil
	}
	return &contract.CallResult{
		OK:            true,
		ToolRef:       toolRef,
		Result:        normalized,
		ResultSummary: summary,
	}, nil
}

func (l *live) List(ctx context.Context) (*contract.ListResult, error) {
	return l.skeleton.List(ctx)
}

func (l *live) serverConfig(serverID string) config.ServerConfig {
	if l.cfg != nil {
		return l.cfg.MCP[serverID]
	}
	return config.ServerConfig{}
}

func (l *live) listSessionTools(ctx context.Context, serverID string, _ config.ServerConfig, session downstream.Session) ([]*mcpsdk.Tool, *contract.Error) {
	tools, err := session.ListTools(ctx, nil)
	if err != nil {
		return nil, &contract.Error{
			Type:             contract.ErrTypeDownstreamCallFailed,
			ServerID:         serverID,
			Retryable:        true,
			Message:          fmt.Sprintf("tools/list failed on server %q: %v", serverID, err),
			AgentInstruction: "Check the downstream server health and retry.",
		}
	}
	return tools.Tools, nil
}

func (l *live) normalizeCandidate(serverID string, tool *mcpsdk.Tool) contract.Candidate {
	toolRef := serverID + "." + tool.Name
	return contract.Candidate{
		ToolRef:            toolRef,
		ServerID:           serverID,
		DownstreamToolName: tool.Name,
		Title:              tool.Title,
		Description:        tool.Description,
		InputSchema:        normalizeInputSchema(tool.InputSchema),
	}
}

func normalizeInputSchema(schema any) map[string]any {
	if schema == nil {
		return map[string]any{"type": "object"}
	}
	switch s := schema.(type) {
	case map[string]any:
		return s
	default:
		return map[string]any{"type": "object", "raw": schema}
	}
}

func candidateRefs(candidates []contract.Candidate) string {
	refs := make([]string, len(candidates))
	for i, c := range candidates {
		refs[i] = c.ToolRef
	}
	result := ""
	for i, r := range refs {
		if i > 0 {
			result += ", "
		}
		result += r
	}
	return result
}

// splitToolRef parses "<serverId>.<downstreamToolName>" by splitting on the
// first '.' so downstream tool names may themselves contain dots.
func splitToolRef(toolRef string) (serverID, downstreamName string, ok bool) {
	idx := strings.IndexByte(toolRef, '.')
	if idx <= 0 || idx == len(toolRef)-1 {
		return "", "", false
	}
	return toolRef[:idx], toolRef[idx+1:], true
}

func malformedToolRef(toolRef string) *contract.Error {
	return &contract.Error{
		Type:      contract.ErrTypeToolNotFound,
		ToolRef:   toolRef,
		Retryable: false,
		Message:   "toolRef is missing the '<serverId>.<toolName>' separator",
		AgentInstruction: "Call findTool to discover a valid toolRef (format: <serverId>.<downstreamToolName>) " +
			"before invoking.",
	}
}

func unknownServer(toolRef, serverID string) *contract.Error {
	return &contract.Error{
		Type:      contract.ErrTypeConfigError,
		ServerID:  serverID,
		ToolRef:   toolRef,
		Retryable: false,
		Message:   fmt.Sprintf("server %q is not in the configuration", serverID),
		AgentInstruction: "Run findTool again to discover valid toolRefs, or fix the server configuration " +
			"before retrying this call.",
	}
}

func disabledServer(toolRef, serverID string) *contract.Error {
	return &contract.Error{
		Type:      contract.ErrTypeConfigError,
		ServerID:  serverID,
		ToolRef:   toolRef,
		Retryable: false,
		Message:   fmt.Sprintf("server %q is disabled in the configuration", serverID),
		AgentInstruction: "Enable the server in your Ozy config or pick a different toolRef from findTool " +
			"before retrying this call.",
	}
}

func (l *live) serverLookup(serverID string) (config.ServerConfig, bool) {
	if l.cfg == nil {
		return config.ServerConfig{}, false
	}
	server, ok := l.cfg.MCP[serverID]
	return server, ok
}

// extractErrorText concatenates TextContent text from a downstream IsError
// result. Non-text content is skipped.
func extractErrorText(res *mcpsdk.CallToolResult) string {
	if res == nil {
		return ""
	}
	var b strings.Builder
	for _, c := range res.Content {
		tc, ok := c.(*mcpsdk.TextContent)
		if !ok {
			continue
		}
		if b.Len() > 0 {
			b.WriteString("\n")
		}
		b.WriteString(tc.Text)
	}
	return strings.TrimSpace(b.String())
}

// normalizeCallResult prefers StructuredContent when present, otherwise joins
// the TextContent fragments into a single string (SPEC.md §9.3).
func normalizeCallResult(res *mcpsdk.CallToolResult) any {
	if res == nil {
		return nil
	}
	if res.StructuredContent != nil {
		return res.StructuredContent
	}
	if len(res.Content) == 0 {
		return ""
	}
	if len(res.Content) == 1 {
		if tc, ok := res.Content[0].(*mcpsdk.TextContent); ok {
			return tc.Text
		}
	}
	parts := make([]string, 0, len(res.Content))
	for _, c := range res.Content {
		if tc, ok := c.(*mcpsdk.TextContent); ok {
			parts = append(parts, tc.Text)
		}
	}
	return strings.Join(parts, "\n")
}

func summarizeCall(downstreamName string, result any) string {
	switch v := result.(type) {
	case string:
		if v == "" {
			return fmt.Sprintf("tool %q returned no content", downstreamName)
		}
		return firstLine(fmt.Sprintf("tool %q returned: %s", downstreamName, v))
	default:
		return fmt.Sprintf("tool %q returned structured content", downstreamName)
	}
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i] + " ..."
	}
	return s
}

// applyCallBudget returns (truncatedResult, marker, true) when the encoded
// normalized result exceeds budgets.callTool.maxResultBytes, otherwise
// (nil, "", false).
func applyCallBudget(cfg *config.Config, result any) (any, string, bool) {
	if cfg == nil || cfg.Budgets.CallTool.MaxResultBytes <= 0 {
		return nil, "", false
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		return nil, "", false
	}
	if len(encoded) <= cfg.Budgets.CallTool.MaxResultBytes {
		return nil, "", false
	}
	limit := cfg.Budgets.CallTool.MaxResultBytes
	if limit < 16 {
		limit = 16
	}
	truncated := append(encoded[:limit], []byte("...[truncated]")...)
	marker := fmt.Sprintf("(result exceeded budgets.callTool.maxResultBytes=%d bytes; narrow the call for full output)", cfg.Budgets.CallTool.MaxResultBytes)
	return string(truncated), marker, true
}
