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
	"github.com/rokasklive/ozy/internal/schema"
	"github.com/rokasklive/ozy/internal/search"
)

// Connector is the downstream seam used by the live broker. ConnectAll powers
// FindTool; Connect is the single-server connection used by CallTool so only
// the target server is contacted for a single invocation.
type Connector interface {
	ConnectAll(ctx context.Context, cfg *config.Config) []downstream.Result
	Connect(ctx context.Context, serverID string, server config.ServerConfig) downstream.Result
}

// live is the broker that ranks cataloged tools for findTool via the search
// engine and performs live brokered invocation for CallTool, while delegating
// describeTool and List to the skeleton backed by the catalog store.
type live struct {
	skeleton  *skeleton
	cfg       *config.Config
	connector Connector
	engine    *search.Engine
}

// NewLive returns a Broker that ranks cataloged tools via the search engine when
// findTool is called and performs live brokered invocation when CallTool is
// called. describeTool and List remain catalog-backed.
func NewLive(store catalog.Store, cfg *config.Config, connector Connector, engine *search.Engine) Broker {
	return &live{
		skeleton:  &skeleton{store: store},
		cfg:       cfg,
		connector: connector,
		engine:    engine,
	}
}

func (l *live) FindTool(ctx context.Context, query string) (*contract.FindResult, error) {
	ranking, err := l.engine.Find(ctx, query)
	if err != nil {
		return nil, err
	}

	decision := search.Decide(ranking)
	stats, _ := l.stats(ctx)

	result := &contract.FindResult{
		Query:        query,
		Decision:     mapDecision(decision.Verdict),
		CatalogStats: stats,
	}

	switch decision.Verdict {
	case search.DecisionUse:
		result.Confidence = decision.Confidence
		if decision.Selected != nil {
			result.SelectedToolRef = decision.Selected.Tool.ToolRef
			result.Selected = selectedToolFromEntry(decision.Selected, l.schemaPreview(decision.Selected.Tool.InputSchema))
			result.Reason = decision.Selected.Reason
			result.Alternatives = l.alternatives(decision.RunnerUp)
			result.NextAction = &contract.NextAction{
				Tool:      "describeTool",
				ToolRef:   decision.Selected.Tool.ToolRef,
				Arguments: map[string]any{"toolRef": decision.Selected.Tool.ToolRef},
				Reason:    "Inspect exact schema before invoking through callTool.",
			}
			result.AgentInstruction = "Use describeTool to inspect the selected tool's full schema before invoking it through callTool."
		}

	case search.DecisionAmbiguous:
		if decision.Selected != nil {
			result.SelectedToolRef = decision.Selected.Tool.ToolRef
			result.Selected = selectedToolFromEntry(decision.Selected, l.schemaPreview(decision.Selected.Tool.InputSchema))
			result.Alternatives = l.alternatives(decision.RunnerUp)
			result.Reason = fmt.Sprintf("Multiple tools match %q — top two are too close to separate confidently.", query)
			result.NextAction = &contract.NextAction{
				Tool:      "describeTool",
				ToolRef:   decision.Selected.Tool.ToolRef,
				Arguments: map[string]any{"toolRef": decision.Selected.Tool.ToolRef},
				Reason:    "Inspect the close candidates' schemas with describeTool before choosing.",
			}
		}
		if decision.RunnerUp != nil {
			result.Candidates = []contract.Candidate{
				catalogToolToCandidate(decision.Selected),
				catalogToolToCandidate(decision.RunnerUp),
			}
		}
		result.AgentInstruction = "Two tools match closely. Use describeTool on both to inspect their schemas before choosing."

	case search.DecisionNoGoodMatch:
		if decision.Selected != nil {
			result.Reason = fmt.Sprintf("No indexed tool strongly matches %q.", query)
		}
		result.NextAction = &contract.NextAction{
			Tool:   "findTool",
			Reason: "Retry findTool with a more specific capability query.",
		}
		result.AgentInstruction = "Refine the query to be more specific, then retry findTool. If the tool should be available, run `ozy doctor` to check the catalog and then `ozy index` to refresh it. Do not infer that the capability is unavailable."

	case search.DecisionCatalogEmpty:
		result.AgentInstruction = "The catalog has no indexed tools. Run `ozy index` to populate it, or check configuration with `ozy doctor`. Do not infer that capabilities are unavailable."
	}

	if ranking.SemanticDegraded {
		note := "Semantic search was requested but is unavailable; results are lexical-only. Run `ozy doctor` to see why and `ozy index` to rebuild the semantic index."
		if result.AgentInstruction != "" {
			result.AgentInstruction = note + " " + result.AgentInstruction
		} else {
			result.AgentInstruction = note
		}
	}

	return result, nil
}

func (l *live) stats(ctx context.Context) (*contract.CatalogStats, error) {
	cs, err := l.skeleton.store.Stats(ctx)
	if err != nil {
		return nil, err
	}
	return &contract.CatalogStats{
		ConfiguredServers: cs.ConfiguredServers,
		IndexedTools:      cs.IndexedTools,
		FreshTools:        cs.FreshTools,
		StaleTools:        cs.StaleTools,
	}, nil
}

func (l *live) schemaPreview(schema map[string]any) *contract.SchemaPreview {
	if schema == nil {
		return nil
	}
	preview := &contract.SchemaPreview{}
	if props, ok := schema["properties"].(map[string]any); ok {
		for name := range props {
			preview.Properties = append(preview.Properties, name)
		}
	}
	if req, ok := schema["required"].([]any); ok {
		for _, r := range req {
			if s, ok := r.(string); ok {
				preview.Required = append(preview.Required, s)
			}
		}
	}
	if len(preview.Properties) == 0 && len(preview.Required) == 0 {
		return nil
	}
	return preview
}

func (l *live) alternatives(entry *search.RankedEntry) []contract.Alternative {
	if entry == nil {
		return nil
	}
	return []contract.Alternative{{
		ToolRef: entry.Tool.ToolRef,
		Reason:  entry.Reason,
	}}
}

func selectedToolFromEntry(entry *search.RankedEntry, preview *contract.SchemaPreview) *contract.SelectedTool {
	if entry == nil {
		return nil
	}
	return &contract.SelectedTool{
		ToolRef:       entry.Tool.ToolRef,
		Title:         entry.Tool.Title,
		CallableNow:   entry.Tool.CallableNow,
		ServerStatus:  string(entry.Tool.ServerStatus),
		SchemaPreview: preview,
	}
}

func catalogToolToCandidate(entry *search.RankedEntry) contract.Candidate {
	if entry == nil {
		return contract.Candidate{}
	}
	return contract.Candidate{
		ToolRef:            entry.Tool.ToolRef,
		ServerID:           entry.Tool.ServerID,
		DownstreamToolName: entry.Tool.DownstreamToolName,
		Title:              entry.Tool.Title,
		Description:        entry.Tool.Description,
		InputSchema:        entry.Tool.InputSchema,
	}
}

func mapDecision(d string) string {
	switch d {
	case search.DecisionUse:
		return contract.DecisionUse
	case search.DecisionAmbiguous:
		return contract.DecisionAmbiguous
	case search.DecisionNoGoodMatch:
		return contract.DecisionNoGoodMatch
	case search.DecisionCatalogEmpty:
		return contract.DecisionCatalogEmpty
	default:
		return contract.DecisionNoGoodMatch
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

	if verr := l.validateArgs(ctx, toolRef, args); verr != nil {
		return nil, verr
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
			Type:             contract.ErrTypeDownstreamCallFailed,
			ServerID:         serverID,
			ToolRef:          toolRef,
			Retryable:        true,
			Message:          fmt.Sprintf("tools/call failed on server %q: %v", serverID, downstream.Scrub(err.Error(), server)),
			AgentInstruction: "Check the downstream server health and the call arguments, then retry.",
		}
	}
	if callRes.IsError {
		return nil, &contract.Error{
			Type:             contract.ErrTypeDownstreamCallFailed,
			ServerID:         serverID,
			ToolRef:          toolRef,
			Retryable:        false,
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

// validateArgs checks args against the tool's cataloged input schema when one is
// available, so an agent that omits or mistypes an argument gets a structured
// ARGUMENT_VALIDATION_FAILED before any downstream contact. Validation is
// best-effort: a toolRef with no catalog entry or no schema (for example before
// `ozy index` has run) is allowed through so callTool stays index-free, leaving
// live correctness to the downstream server.
func (l *live) validateArgs(ctx context.Context, toolRef string, args map[string]any) *contract.Error {
	tool, ok, err := l.skeleton.store.GetTool(ctx, toolRef)
	if err != nil || !ok || len(tool.InputSchema) == 0 {
		return nil
	}
	problems := schema.Validate(tool.InputSchema, args)
	if len(problems) == 0 {
		return nil
	}
	return &contract.Error{
		Type:      contract.ErrTypeArgumentValidationFailed,
		ToolRef:   toolRef,
		ServerID:  tool.ServerID,
		Retryable: false,
		Message:   "arguments do not satisfy the tool schema: " + strings.Join(problems, "; "),
		AgentInstruction: "Correct the arguments named in the message and re-call callTool; call describeTool to confirm " +
			"the exact schema first. Do not retry the same arguments unchanged.",
	}
}

func (l *live) List(ctx context.Context) (*contract.ListResult, error) {
	return l.skeleton.List(ctx)
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
	truncated := make([]byte, 0, limit+len("...[truncated]"))
	truncated = append(truncated, encoded[:limit]...)
	truncated = append(truncated, "...[truncated]"...)
	marker := fmt.Sprintf("(result exceeded budgets.callTool.maxResultBytes=%d bytes; narrow the call for full output)", cfg.Budgets.CallTool.MaxResultBytes)
	return string(truncated), marker, true
}
