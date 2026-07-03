package broker

import (
	"context"
	"time"

	"github.com/rokasklive/ozy/internal/catalog"
	"github.com/rokasklive/ozy/internal/contract"
)

// skeleton is the placeholder broker. It performs no ranking or downstream
// invocation yet; instead it returns contract-shaped, instructional responses
// grounded only in catalog state. Later changes replace the bodies without
// changing the response contracts.
type skeleton struct {
	store catalog.Store
}

// NewSkeleton returns a Broker backed by the given catalog store.
func NewSkeleton(store catalog.Store) Broker {
	return &skeleton{store: store}
}

func (s *skeleton) stats(ctx context.Context) *contract.CatalogStats {
	st, err := s.store.Stats(ctx)
	if err != nil {
		return nil
	}
	out := &contract.CatalogStats{
		ConfiguredServers: st.ConfiguredServers,
		IndexedTools:      st.IndexedTools,
		FreshTools:        st.FreshTools,
		StaleTools:        st.StaleTools,
	}
	out.CatalogAgeSeconds = catalogAge(ctx, s.store)
	return out
}

// catalogAge derives seconds-since-last-index for responses, nil when the
// catalog has never been indexed (distinct from a just-indexed age of 0).
func catalogAge(ctx context.Context, store catalog.Store) *int64 {
	t, ok, err := store.LastIndexedAt(ctx)
	if err != nil || !ok {
		return nil
	}
	age := int64(time.Since(t).Seconds())
	if age < 0 {
		age = 0
	}
	return &age
}

// FindTool returns catalog_empty when nothing is indexed, otherwise no_good_match
// (ranking is not implemented yet). Both outcomes are explicit decisions, never
// Go errors, and both are instructional per SPEC.md §9.1.
func (s *skeleton) FindTool(ctx context.Context, query string) (*contract.FindResult, error) {
	stats := s.stats(ctx)
	if stats == nil || stats.IndexedTools == 0 {
		return &contract.FindResult{
			Query:        query,
			Decision:     contract.DecisionCatalogEmpty,
			CatalogStats: stats,
			AgentInstruction: "The catalog has no indexed tools. Do not infer that the capability is unavailable. " +
				"Run `ozy doctor` to diagnose configuration, then `ozy index` to populate the catalog before searching again.",
		}, nil
	}
	return &contract.FindResult{
		Query:        query,
		Decision:     contract.DecisionNoGoodMatch,
		CatalogStats: stats,
		AgentInstruction: "Tool ranking is not implemented in this build, so no match can be selected yet. " +
			"Use `ozy list` to inspect indexed tools, or report that search is pending.",
	}, nil
}

// DescribeTool resolves the toolRef against the catalog. Unknown tools yield a
// TOOL_NOT_FOUND structured error directing the agent to discover first.
func (s *skeleton) DescribeTool(ctx context.Context, toolRef string) (*contract.DescribeResult, error) {
	tool, ok, err := s.store.GetTool(ctx, toolRef)
	if err != nil || !ok {
		return nil, &contract.Error{
			Type:      contract.ErrTypeToolNotFound,
			ToolRef:   toolRef,
			Retryable: false,
			Message:   "No tool with this toolRef is in the catalog.",
			AgentInstruction: "Do not retry with this toolRef. Call findTool to discover a valid tool, " +
				"or run `ozy index` if the catalog is empty.",
		}
	}
	return &contract.DescribeResult{
		ToolRef:     tool.ToolRef,
		Title:       tool.Title,
		Description: tool.Description,
		InputSchema: tool.InputSchema,
		Status: &contract.ToolStatus{
			CallableNow:      tool.CallableNow,
			ServerStatus:     string(tool.ServerStatus),
			CatalogFreshness: string(tool.Freshness),
		},
	}, nil
}

// CallTool is live-gated: it first resolves the toolRef. Unknown tools yield
// TOOL_NOT_FOUND; known tools yield NOT_IMPLEMENTED because brokered invocation
// is deferred. Either way the response is a §9.3-shaped structured failure with a
// grounded, conditional agentInstruction (it never fabricates execution).
func (s *skeleton) CallTool(ctx context.Context, toolRef string, _ map[string]any) (*contract.CallResult, error) {
	if _, ok, err := s.store.GetTool(ctx, toolRef); err != nil || !ok {
		return nil, &contract.Error{
			Type:      contract.ErrTypeToolNotFound,
			ToolRef:   toolRef,
			Retryable: false,
			Message:   "No tool with this toolRef is in the catalog.",
			AgentInstruction: "Do not retry with this toolRef. Call findTool to discover a valid tool before invoking, " +
				"or run `ozy index` if the catalog is empty.",
		}
	}
	return nil, &contract.Error{
		Type:      contract.ErrTypeNotImplemented,
		ToolRef:   toolRef,
		Retryable: false,
		Message:   "Brokered invocation is not implemented in this build.",
		AgentInstruction: "Do not retry. Invocation is pending a future change; report this to the user or " +
			"choose another approach. Ozy will not fabricate a downstream call.",
	}
}

// List returns the current catalog contents, or an instructional empty listing.
func (s *skeleton) List(ctx context.Context) (*contract.ListResult, error) {
	tools, err := s.store.Tools(ctx)
	if err != nil {
		return nil, &contract.Error{
			Type:             contract.ErrTypeConfigError,
			Retryable:        true,
			Message:          "Failed to read the catalog store.",
			AgentInstruction: "Run `ozy doctor` to inspect catalog storage status.",
		}
	}
	out := &contract.ListResult{CatalogStats: s.stats(ctx)}
	for _, t := range tools {
		out.Tools = append(out.Tools, contract.ListedTool{
			ToolRef:     t.ToolRef,
			Title:       t.Title,
			ServerID:    t.ServerID,
			Freshness:   string(t.Freshness),
			CallableNow: t.CallableNow,
		})
	}
	if len(out.Tools) == 0 {
		out.AgentInstruction = "No tools are indexed. Configure downstream servers, then run `ozy index`."
	}
	return out, nil
}
