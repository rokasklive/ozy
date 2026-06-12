package broker

import (
	"context"
	"fmt"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/rokasklive/ozy/internal/catalog"
	"github.com/rokasklive/ozy/internal/config"
	"github.com/rokasklive/ozy/internal/contract"
	"github.com/rokasklive/ozy/internal/downstream"
)

// Connector is the downstream discovery seam used by the live broker.
type Connector interface {
	ConnectAll(ctx context.Context, cfg *config.Config) []downstream.Result
}

// live is the broker that performs live downstream tool discovery for FindTool
// while delegating describeTool, callTool, and List to the skeleton backed by
// the catalog store.
type live struct {
	skeleton  *skeleton
	cfg       *config.Config
	connector Connector
}

// NewLive returns a Broker that discovers tools live from configured downstream
// MCP servers when FindTool is called. describeTool and callTool remain
// placeholder (skeleton-backed) per the deferred scope of this change.
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
	return l.skeleton.CallTool(ctx, toolRef, args)
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
