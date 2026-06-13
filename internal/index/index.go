// Package index discovers tools from connected downstream servers and persists
// normalized catalog entries.
package index

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/rokasklive/ozy/internal/catalog"
	"github.com/rokasklive/ozy/internal/config"
	"github.com/rokasklive/ozy/internal/contract"
	"github.com/rokasklive/ozy/internal/downstream"
)

// Connector is the downstream dependency used by the indexer.
type Connector interface {
	ConnectAll(ctx context.Context, cfg *config.Config) []downstream.Result
}

// Summary is the structured `ozy index` result.
type Summary struct {
	OK               bool             `json:"ok"`
	ServersReached   int              `json:"serversReached"`
	ServersSkipped   int              `json:"serversSkipped"`
	ServersFailed    int              `json:"serversFailed"`
	ToolsIndexed     int              `json:"toolsIndexed"`
	Errors           []contract.Error `json:"errors,omitempty"`
	AgentInstruction string           `json:"agentInstruction,omitempty"`
}

// Render produces the human/concise form of an index summary.
func (s *Summary) Render(format string) string {
	if format == contract.FormatConcise {
		return fmt.Sprintf("index servers=%d tools=%d errors=%d", s.ServersReached, s.ToolsIndexed, len(s.Errors))
	}
	var b strings.Builder
	fmt.Fprintf(&b, "indexed %d tools from %d reachable servers", s.ToolsIndexed, s.ServersReached)
	if s.ServersSkipped > 0 {
		fmt.Fprintf(&b, "\nskipped servers: %d", s.ServersSkipped)
	}
	if s.ServersFailed > 0 {
		fmt.Fprintf(&b, "\nfailed servers: %d", s.ServersFailed)
		for _, e := range s.Errors {
			fmt.Fprintf(&b, "\n  - %s: %s", e.ServerID, e.Message)
		}
	}
	if s.AgentInstruction != "" {
		fmt.Fprintf(&b, "\n→ %s", s.AgentInstruction)
	}
	return b.String()
}

// Indexer coordinates connect -> tools/list -> normalize -> persist.
type Indexer struct {
	store     catalog.Store
	connector Connector
	now       func() time.Time
}

// Option customizes an Indexer.
type Option func(*Indexer)

// WithClock injects time for deterministic tests.
func WithClock(now func() time.Time) Option {
	return func(i *Indexer) {
		if now != nil {
			i.now = now
		}
	}
}

// New constructs an Indexer.
func New(store catalog.Store, connector Connector, opts ...Option) *Indexer {
	if connector == nil {
		connector = downstream.New()
	}
	i := &Indexer{
		store:     store,
		connector: connector,
		now:       time.Now,
	}
	for _, opt := range opts {
		opt(i)
	}
	return i
}

// Run connects to configured downstream servers, discovers tools, and persists
// normalized catalog entries.
func (i *Indexer) Run(ctx context.Context, cfg *config.Config) *Summary {
	summary := &Summary{OK: true}
	for _, result := range i.connector.ConnectAll(ctx, cfg) {
		if result.Skipped {
			summary.ServersSkipped++
			i.recordServer(ctx, summary, result.ServerID, catalog.ServerUnknown)
			continue
		}
		if result.Error != nil {
			summary.ServersFailed++
			summary.Errors = append(summary.Errors, *result.Error)
			i.recordServer(ctx, summary, result.ServerID, catalog.ServerOffline)
			continue
		}
		if result.Session == nil {
			err := contract.Error{
				Type:             contract.ErrTypeDownstreamServerOffline,
				ServerID:         result.ServerID,
				Retryable:        true,
				Message:          "downstream connector returned no session",
				AgentInstruction: "Retry after checking the server connection.",
			}
			summary.ServersFailed++
			summary.Errors = append(summary.Errors, err)
			i.recordServer(ctx, summary, result.ServerID, catalog.ServerOffline)
			continue
		}

		server := config.ServerConfig{}
		if cfg != nil {
			server = cfg.MCP[result.ServerID]
		}
		summary.ServersReached++
		i.recordServer(ctx, summary, result.ServerID, catalog.ServerOnline)
		i.indexSession(ctx, summary, result.ServerID, server, result.Session)
		_ = result.Session.Close()
	}
	switch {
	case summary.ServersReached == 0:
		summary.OK = false
		summary.AgentInstruction = "No configured downstream server was reachable. Review the per-server errors, run `ozy doctor`, then retry `ozy index` after repairing configuration or connectivity."
	case summary.ToolsIndexed == 0 && len(summary.Errors) > 0:
		summary.OK = false
		summary.AgentInstruction = "No downstream tools were indexed. Review the per-server errors, run `ozy doctor`, then retry `ozy index` after repairing configuration or connectivity."
	case len(summary.Errors) > 0:
		summary.AgentInstruction = "Some servers failed, but reachable servers were indexed. Use `ozy list` or `ozy describe` for indexed tools and repair failed servers separately."
	}
	if summary.ServersReached > 0 {
		if err := i.store.SetLastIndexedAt(ctx, i.now()); err != nil {
			summary.OK = false
			summary.Errors = append(summary.Errors, contract.Error{
				Type:             contract.ErrTypeConfigError,
				Retryable:        true,
				Message:          fmt.Sprintf("failed to persist last-indexed time: %v", err),
				AgentInstruction: "Check catalog storage permissions, then retry indexing.",
			})
		}
	}
	return summary
}

func (i *Indexer) recordServer(ctx context.Context, summary *Summary, serverID string, status catalog.ServerStatus) {
	if err := i.store.PutServer(ctx, catalog.Server{ID: serverID, Status: status}); err != nil {
		summary.OK = false
		summary.Errors = append(summary.Errors, contract.Error{
			Type:             contract.ErrTypeConfigError,
			ServerID:         serverID,
			Retryable:        true,
			Message:          fmt.Sprintf("failed to persist server status: %v", err),
			AgentInstruction: "Check catalog storage permissions, then retry indexing.",
		})
	}
}

func (i *Indexer) indexSession(ctx context.Context, summary *Summary, serverID string, server config.ServerConfig, session downstream.Session) {
	listCtx, cancel := context.WithTimeout(ctx, server.DiscoveryTimeout())
	defer cancel()
	list, err := session.ListTools(listCtx, nil)
	if err != nil {
		summary.ServersFailed++
		summary.Errors = append(summary.Errors, contract.Error{
			Type:             contract.ErrTypeDownstreamCallFailed,
			ServerID:         serverID,
			Retryable:        true,
			Message:          fmt.Sprintf("tools/list failed: %v", scrub(err.Error(), server)),
			AgentInstruction: "Check the downstream server health and retry indexing.",
		})
		return
	}
	for _, tool := range list.Tools {
		catalogTool, err := normalizeTool(serverID, tool, i.now())
		if err != nil {
			summary.Errors = append(summary.Errors, contract.Error{
				Type:             contract.ErrTypeDownstreamCallFailed,
				ServerID:         serverID,
				Retryable:        false,
				Message:          fmt.Sprintf("tool %q has invalid schema: %v", tool.Name, err),
				AgentInstruction: "Report the invalid downstream tool schema to the server owner.",
			})
			continue
		}
		if err := i.store.PutTool(ctx, catalogTool); err != nil {
			summary.OK = false
			summary.Errors = append(summary.Errors, contract.Error{
				Type:             contract.ErrTypeConfigError,
				ServerID:         serverID,
				Retryable:        true,
				Message:          fmt.Sprintf("failed to persist tool %q: %v", catalogTool.ToolRef, err),
				AgentInstruction: "Check catalog storage permissions, then retry indexing.",
			})
			continue
		}
		summary.ToolsIndexed++
	}
}

func scrub(msg string, server config.ServerConfig) string {
	for _, secret := range secretValues(server) {
		if secret == "" || strings.Contains(secret, "{env:") {
			continue
		}
		msg = strings.ReplaceAll(msg, secret, "****")
	}
	return msg
}

func secretValues(server config.ServerConfig) []string {
	values := make([]string, 0, len(server.Headers)+len(server.Environment))
	for _, v := range server.Headers {
		values = append(values, v)
	}
	for _, v := range server.Environment {
		values = append(values, v)
	}
	return values
}

func normalizeTool(serverID string, tool *mcpsdk.Tool, now time.Time) (catalog.Tool, error) {
	inputSchema, schemaHash, err := normalizeSchema(tool.InputSchema)
	if err != nil {
		return catalog.Tool{}, err
	}
	return catalog.Tool{
		ToolRef:            serverID + "." + tool.Name,
		ServerID:           serverID,
		DownstreamToolName: tool.Name,
		Title:              tool.Title,
		Description:        tool.Description,
		InputSchema:        inputSchema,
		CapabilityText:     []string{tool.Title, tool.Description},
		ServerStatus:       catalog.ServerOnline,
		CallableNow:        true,
		LastIndexedAt:      now,
		SchemaHash:         schemaHash,
		Freshness:          catalog.FreshnessFresh,
	}, nil
}

func normalizeSchema(schema any) (map[string]any, string, error) {
	if schema == nil {
		schema = map[string]any{"type": "object"}
	}
	data, err := json.Marshal(schema)
	if err != nil {
		return nil, "", err
	}
	var out map[string]any
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, "", err
	}
	canonical, err := json.Marshal(out)
	if err != nil {
		return nil, "", err
	}
	sum := sha256.Sum256(canonical)
	return out, hex.EncodeToString(sum[:]), nil
}
