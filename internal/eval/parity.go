package eval

import (
	"context"
	"encoding/json"
	"fmt"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/rokasklive/ozy/internal/broker"
	ozymcp "github.com/rokasklive/ozy/internal/mcp"
)

// observed is the surface-independent projection of a broker response used for
// CLI↔MCP parity: the decision (findTool), the selected/target toolRef, and the
// structured error type (failures). Two surfaces agree when these are equal.
type observed struct {
	Decision  string
	ToolRef   string
	ErrorType string
}

// observedFromJSON projects a decoded response document into an observed value,
// reading the fields the CLI JSON and the MCP structured content share.
func observedFromJSON(doc map[string]any) observed {
	var o observed
	if s, ok := doc["decision"].(string); ok {
		o.Decision = s
	}
	if s, ok := doc["selectedToolRef"].(string); ok && s != "" {
		o.ToolRef = s
	} else if s, ok := doc["toolRef"].(string); ok {
		o.ToolRef = s
	}
	if errObj, ok := doc["error"].(map[string]any); ok {
		if t, ok := errObj["type"].(string); ok {
			o.ErrorType = t
		}
	}
	return o
}

// mcpProbe drives the real in-process MCP adapter over an in-memory transport so
// parity is measured against the actual adapter code, not a reimplementation.
type mcpProbe struct {
	session *mcpsdk.ClientSession
	cancel  context.CancelFunc
}

// newMCPProbe stands up the Ozy MCP adapter for bk and connects an in-memory
// client to it. Close must be called to release the session and server.
func newMCPProbe(ctx context.Context, bk broker.Broker) (*mcpProbe, error) {
	runCtx, cancel := context.WithCancel(ctx)
	serverT, clientT := mcpsdk.NewInMemoryTransports()

	srv := ozymcp.New(bk, "eval").Server()
	go func() { _ = srv.Run(runCtx, serverT) }()

	client := mcpsdk.NewClient(&mcpsdk.Implementation{Name: "ozy-eval", Version: "eval"}, nil)
	session, err := client.Connect(runCtx, clientT, nil)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("connect in-memory MCP client: %w", err)
	}
	return &mcpProbe{session: session, cancel: cancel}, nil
}

// call invokes one Ozy tool through the MCP adapter and returns the decoded
// response document (parsed from the result's JSON text content).
func (p *mcpProbe) call(ctx context.Context, name string, args map[string]any) (map[string]any, error) {
	res, err := p.session.CallTool(ctx, &mcpsdk.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		return nil, fmt.Errorf("mcp %s: %w", name, err)
	}
	text := resultText(res)
	// A successful callTool surfaces the downstream payload directly (raw text
	// or structured content) with Ozy's metadata in _meta — not a JSON envelope.
	// Project that into the shared doc shape so parity reads toolRef/ok the same
	// way it does for findTool/describeTool. Errors stay JSON envelopes below.
	if name == "callTool" && !res.IsError {
		doc := map[string]any{"ok": true}
		if res.StructuredContent != nil {
			doc["result"] = res.StructuredContent
		} else {
			doc["result"] = text
		}
		for k, v := range res.Meta {
			doc[k] = v
		}
		return doc, nil
	}
	if text == "" {
		return nil, fmt.Errorf("mcp %s: empty result content", name)
	}
	var doc map[string]any
	if err := json.Unmarshal([]byte(text), &doc); err != nil {
		return nil, fmt.Errorf("mcp %s: response is not JSON: %w", name, err)
	}
	return doc, nil
}

// Close ends the client session and stops the in-process server.
func (p *mcpProbe) Close() {
	if p.session != nil {
		_ = p.session.Close()
	}
	if p.cancel != nil {
		p.cancel()
	}
}

// resultText returns the first text-content fragment of an MCP tool result; the
// Ozy adapter always emits its response as a single JSON text block.
func resultText(res *mcpsdk.CallToolResult) string {
	if res == nil {
		return ""
	}
	for _, c := range res.Content {
		if tc, ok := c.(*mcpsdk.TextContent); ok {
			return tc.Text
		}
	}
	return ""
}
