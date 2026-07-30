package bench

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"
)

// outputDirEnv names the per-run agent workspace where functional tools (the
// pdf-toolkit) resolve relative output paths, so the grader finds deliverables.
const outputDirEnv = "OZY_BENCH_OUTPUT_DIR"

// jsonResult marshals v into a CallToolResult containing JSON text content.
// Compact (not indented): these tool results are agent-facing context, so
// indentation whitespace would only spend the agent's tokens. Unlike Ozy's own
// responses, this string payload is passed through verbatim — OpenCode does not
// re-serialize it — so the formatting here reaches the model.
func jsonResult(v any) *mcpsdk.CallToolResult {
	data, err := json.Marshal(v)
	if err != nil {
		data = []byte(`{"error":"failed to encode response"}`)
	}
	return &mcpsdk.CallToolResult{
		Content: []mcpsdk.Content{&mcpsdk.TextContent{Text: string(data)}},
	}
}

// ServeMCP creates an MCP server for the given toolset and serves it over
// stdio. fixtureDir is used by toolsets that read baked fixture data
// (weather, duckduckgo, wikipedia).
func ServeMCP(toolset, fixtureDir string) error {
	srv, err := newMCPServer(toolset, fixtureDir)
	if err != nil {
		return fmt.Errorf("create mcp server: %w", err)
	}
	return serveStdio(srv)
}

// ServeCorpus serves a data-defined corpus server over stdio. fixtureDir feeds
// the `functional:search` backend (baked search corpus); functional:pdf writes to
// the agent workspace (OZY_BENCH_OUTPUT_DIR) and needs no fixtureDir.
func ServeCorpus(name, corpusDir, fixtureDir string) error {
	srv, err := newCorpusServer(name, corpusDir, fixtureDir)
	if err != nil {
		return fmt.Errorf("create corpus server: %w", err)
	}
	return serveStdio(srv)
}

// serveStdio runs srv over stdio until SIGINT/SIGTERM.
func serveStdio(srv *mcpsdk.Server) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		cancel()
	}()

	return srv.Run(ctx, &mcpsdk.StdioTransport{})
}

// newMCPServer creates and configures an *mcpsdk.Server for the given
// toolset, ready to run with any transport (stdio or in-memory for tests).
func newMCPServer(toolset, fixtureDir string) (*mcpsdk.Server, error) {
	name := "ozy-bench-" + toolset
	title := fmt.Sprintf("Ozy Bench %s Fixture MCP Server", toolset)
	srv := mcpsdk.NewServer(&mcpsdk.Implementation{
		Name:    name,
		Title:   title,
		Version: "0.1.0",
	}, nil)

	switch toolset {
	case "weather":
		registerWeather(srv, fixtureDir)
	case "duckduckgo":
		registerDuckDuckGo(srv, fixtureDir)
	case "wikipedia":
		registerWikipedia(srv, fixtureDir)
	case "pdf-toolkit":
		registerPDFToolkit(srv)
	default:
		return nil, fmt.Errorf("unknown toolset: %s (valid: weather, duckduckgo, wikipedia, pdf-toolkit)", toolset)
	}

	// Server-side invocation logging covers this toolset's tools (D4).
	srv.AddReceivingMiddleware(callLogMiddleware(toolset))
	return srv, nil
}

// newBenchServer builds the in-process MCP server for a scenario server spec: a
// functional toolset, or a corpus server.
func newBenchServer(bs benchServer, fixtureDir, corpusDir string) (*mcpsdk.Server, error) {
	if bs.Corpus {
		return newCorpusServer(bs.Name, corpusDir, fixtureDir)
	}
	return newMCPServer(bs.Name, fixtureDir)
}

// newCorpusServer builds an MCP server for a data-defined corpus server: it
// loads <corpusDir>/<name>.json and serves each tool's raw schema, routing calls
// by the tool's Behavior (D3): a deterministic stub (default), a realistic
// authentication-required error (auth-gated rivals), or real data via a shared
// functional backend (no-auth same-capability rivals). Adding a corpus server is
// a data change.
func newCorpusServer(name, corpusDir, fixtureDir string) (*mcpsdk.Server, error) {
	cs, err := LoadCorpusServer(filepath.Join(corpusDir, name+".json"))
	if err != nil {
		return nil, fmt.Errorf("load corpus server %q: %w", name, err)
	}
	srv := mcpsdk.NewServer(&mcpsdk.Implementation{
		Name:    "ozy-bench-" + name,
		Title:   fmt.Sprintf("Ozy Bench %s Corpus MCP Server", name),
		Version: "0.1.0",
	}, nil)

	for _, tool := range cs.Tools {
		t := &mcpsdk.Tool{
			Name:        tool.Name,
			Description: tool.Description,
			// Served verbatim: the SDK accepts any value marshaling to valid
			// JSON schema for a server-only tool (no auto-validation).
			InputSchema: tool.InputSchema,
		}
		srv.AddTool(t, corpusHandler(cs.Server, tool, fixtureDir))
	}

	srv.AddReceivingMiddleware(callLogMiddleware(name))
	return srv, nil
}

// corpusHandler returns the tool handler for a corpus tool's behavior tier: a
// legible auth-required error, a shared functional backend, or the deterministic
// stub. Handlers are byte-stable per tool (the stub and auth error ignore
// arguments; functional backends are deterministic over the baked fixture).
func corpusHandler(server string, tool CorpusTool, fixtureDir string) mcpsdk.ToolHandler {
	switch {
	case tool.Behavior == "auth_error":
		errResult := corpusAuthError(server)
		return func(_ context.Context, _ *mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
			return errResult, nil
		}
	case strings.HasPrefix(tool.Behavior, "functional:"):
		backend := strings.TrimPrefix(tool.Behavior, "functional:")
		return corpusFunctionalHandler(backend, fixtureDir)
	default:
		resp := corpusResponse(tool)
		return func(_ context.Context, _ *mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
			return &mcpsdk.CallToolResult{Content: []mcpsdk.Content{&mcpsdk.TextContent{Text: resp}}}, nil
		}
	}
}

// corpusAuthError builds the deterministic authentication-required error an
// auth-gated rival returns for a keyless agent, so the pick fails legibly and the
// agent routes to a working tool (D3).
func corpusAuthError(server string) *mcpsdk.CallToolResult {
	msg, _ := json.Marshal(map[string]any{
		"error":  "authentication required",
		"code":   401,
		"detail": server + " requires an API key or credentials that are not configured in this environment",
	})
	return &mcpsdk.CallToolResult{
		IsError: true,
		Content: []mcpsdk.Content{&mcpsdk.TextContent{Text: string(msg)}},
	}
}

// corpusFunctionalHandler routes a no-auth same-capability rival to a shared
// backend over the baked fixture: `search` ranks the fixture corpus; `pdf` writes
// a PDF to the agent workspace. Args are read generically (rivals use varying
// field names), so the same handler serves whichever schema the rival declares.
func corpusFunctionalHandler(backend, fixtureDir string) mcpsdk.ToolHandler {
	return func(_ context.Context, req *mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
		args := map[string]any{}
		if req.Params != nil && len(req.Params.Arguments) > 0 {
			_ = json.Unmarshal(req.Params.Arguments, &args)
		}
		switch backend {
		case "search":
			res, err := searchFixtureResults(fixtureDir, firstStringArg(args, "query", "q", "search"), intArg(args, "count", "max_results", "limit"))
			if err != nil {
				return jsonResult(map[string]any{"error": "no baked search corpus: " + err.Error()}), nil
			}
			return jsonResult(res), nil
		case "pdf":
			out := firstStringArg(args, "output_path", "outputPath", "path")
			body := firstStringArg(args, "content", "html", "text", "markdown")
			full, err := writeFixturePDF(out, "", body)
			if err != nil {
				return jsonResult(map[string]any{"error": err.Error()}), nil
			}
			return jsonResult(map[string]any{"outputPath": out, "path": full, "ok": true}), nil
		default:
			return jsonResult(map[string]any{"error": "unknown functional backend: " + backend}), nil
		}
	}
}

// firstStringArg returns the first key in keys whose value is a string.
func firstStringArg(args map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := args[k].(string); ok {
			return v
		}
	}
	return ""
}

// intArg returns the first key in keys whose value is a number, as an int (0 if
// none). JSON numbers decode to float64.
func intArg(args map[string]any, keys ...string) int {
	for _, k := range keys {
		if v, ok := args[k].(float64); ok {
			return int(v)
		}
	}
	return 0
}

// corpusResponse returns the deterministic stub response for a corpus tool: the
// authored override compacted to one line, else a stable envelope naming the
// tool. It never depends on arguments (so it is byte-identical across calls)
// and never carries a scenario ground-truth fact.
func corpusResponse(tool CorpusTool) string {
	if len(tool.CannedResponse) > 0 {
		var buf bytes.Buffer
		if err := json.Compact(&buf, tool.CannedResponse); err == nil {
			return buf.String()
		}
		return string(tool.CannedResponse)
	}
	b, _ := json.Marshal(map[string]any{
		"ok":     true,
		"tool":   tool.Name,
		"detail": "stub response from corpus fixture server",
	})
	return string(b)
}

// ---------------------------------------------------------------------------
// CLI command
// ---------------------------------------------------------------------------

func (a *app) mcpCmd() *cobra.Command {
	var toolset string
	var fixtureDir string
	var server string
	var corpusDir string
	cmd := &cobra.Command{
		Use:   "mcp",
		Short: "Serve a fixture MCP server",
		Long:  "Serve a parameterized fixture toolset (--toolset) or a data-defined corpus server (--server --corpus-dir) over stdio.",
		RunE: func(_ *cobra.Command, _ []string) error {
			// Corpus server mode: serve a data-defined server from the corpus dir.
			if server != "" {
				if corpusDir == "" {
					corpusDir = os.Getenv("OZY_BENCH_CORPUS_DIR")
				}
				if corpusDir == "" {
					return fmt.Errorf("--corpus-dir is required with --server (or set OZY_BENCH_CORPUS_DIR)")
				}
				if fixtureDir == "" {
					fixtureDir = os.Getenv("OZY_BENCH_FIXTURE_DIR")
				}
				return ServeCorpus(server, corpusDir, fixtureDir)
			}
			if toolset == "" {
				return fmt.Errorf("--toolset or --server is required")
			}
			if fixtureDir == "" {
				fixtureDir = os.Getenv("OZY_BENCH_FIXTURE_DIR")
			}
			return ServeMCP(toolset, fixtureDir)
		},
	}
	cmd.Flags().StringVar(&toolset, "toolset", "", "functional toolset to serve: code-search, git, incident-db, filesystem, time, memory, notes, weather, duckduckgo, wikipedia, pdf-toolkit")
	cmd.Flags().StringVar(&fixtureDir, "fixture-dir", "", "path to the fixture directory (required for code-search, git, incident-db, filesystem, and the functional mirrored toolsets)")
	cmd.Flags().StringVar(&server, "server", "", "corpus server name to serve from --corpus-dir")
	cmd.Flags().StringVar(&corpusDir, "corpus-dir", "", "directory of corpus server data files (default: OZY_BENCH_CORPUS_DIR)")
	return cmd
}
