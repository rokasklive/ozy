package bench

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"
)

// CaptureSpec is the opt-in, generation-time record-and-replay spec for a
// scenario: which real MCP servers to contact and which representative calls to
// bake. It is read from <fixtureSrc>/capture.json; an absent file means the
// scenario's fixtures are hand-authored and capture is a no-op (D4).
type CaptureSpec struct {
	Servers []CaptureServer `json:"servers"`
}

// CaptureServer names one real MCP server to capture from. Command is the
// server's launch command (network is allowed at generation time, outside the
// hermetic runtime). BakeTo is the fixture file the shaped responses land in;
// Shape selects how the raw responses are reduced ("search" → the ranked search
// corpus; "" / "raw" → the raw tool-result JSON verbatim).
type CaptureServer struct {
	Name    string        `json:"name"`
	Command []string      `json:"command"`
	BakeTo  string        `json:"bakeTo"`
	Shape   string        `json:"shape,omitempty"`
	Calls   []CaptureCall `json:"calls"`
}

// CaptureCall is one representative tool invocation whose response is baked.
type CaptureCall struct {
	Tool      string         `json:"tool"`
	Arguments map[string]any `json:"arguments"`
}

// captureConnect opens a client session to a capture server and returns a close
// func. It is injectable so tests can capture from an in-memory fixture server
// without a subprocess or network.
type captureConnect func(context.Context, CaptureServer) (*mcpsdk.ClientSession, func(), error)

// LoadCaptureSpec reads capture.json from path. A missing file yields ok=false
// (capture is opt-in), never an error.
//
//nolint:gosec // G304: path is a controlled scenario fixture path.
func LoadCaptureSpec(path string) (*CaptureSpec, bool, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("read capture spec: %w", err)
	}
	var spec CaptureSpec
	if err := json.Unmarshal(data, &spec); err != nil {
		return nil, false, fmt.Errorf("unmarshal capture spec: %w", err)
	}
	return &spec, true, nil
}

// defaultCaptureConnect launches a real MCP server via stdio and connects a
// client. This is the network-touching path used at generation time only.
func defaultCaptureConnect(ctx context.Context, s CaptureServer) (*mcpsdk.ClientSession, func(), error) {
	if len(s.Command) == 0 {
		return nil, nil, fmt.Errorf("capture server %q: empty command", s.Name)
	}
	//nolint:gosec // G204: command comes from the trusted scenario capture spec, run only at generation time.
	cmd := exec.CommandContext(ctx, s.Command[0], s.Command[1:]...)
	client := mcpsdk.NewClient(&mcpsdk.Implementation{Name: "ozy-bench-capture", Version: Version}, nil)
	cs, err := client.Connect(ctx, &mcpsdk.CommandTransport{Command: cmd}, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("connect to %q: %w", s.Name, err)
	}
	return cs, func() { _ = cs.Close() }, nil
}

// CaptureFixture contacts each real MCP server in the spec, records its tool
// schemas (<name>.schemas.json) and the representative call responses (shaped
// into BakeTo), and writes them into outDir. Runtime never re-contacts the
// server — it replays the baked files hermetically. Output is deterministic
// (indented JSON over the recorded responses), so re-capturing identical
// responses is byte-stable.
func CaptureFixture(ctx context.Context, spec *CaptureSpec, outDir string, connect captureConnect) error {
	if connect == nil {
		connect = defaultCaptureConnect
	}
	//nolint:gosec // G301: 0755 is intentional for bench fixture output.
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}
	for _, s := range spec.Servers {
		cs, closeFn, err := connect(ctx, s)
		if err != nil {
			return err
		}
		schemas, calls, err := captureServer(ctx, cs, s)
		closeFn()
		if err != nil {
			return err
		}
		if err := writeJSON(filepath.Join(outDir, s.Name+".schemas.json"), schemas); err != nil {
			return err
		}
		baked, err := shapeCapture(s.Shape, calls)
		if err != nil {
			return fmt.Errorf("shape %q: %w", s.Name, err)
		}
		if err := writeJSON(filepath.Join(outDir, s.BakeTo), baked); err != nil {
			return err
		}
	}
	return nil
}

// captureServer records a server's advertised tool schemas and the raw text of
// each representative call's result.
func captureServer(ctx context.Context, cs *mcpsdk.ClientSession, s CaptureServer) (schemas []map[string]any, calls []json.RawMessage, err error) {
	lt, err := cs.ListTools(ctx, &mcpsdk.ListToolsParams{})
	if err != nil {
		return nil, nil, fmt.Errorf("list tools %q: %w", s.Name, err)
	}
	for _, t := range lt.Tools {
		schemas = append(schemas, map[string]any{"name": t.Name, "description": t.Description, "inputSchema": t.InputSchema})
	}
	for _, call := range s.Calls {
		res, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{Name: call.Tool, Arguments: call.Arguments})
		if err != nil {
			return nil, nil, fmt.Errorf("call %s/%s: %w", s.Name, call.Tool, err)
		}
		calls = append(calls, json.RawMessage(toolResultText(res)))
	}
	return schemas, calls, nil
}

// toolResultText returns the first text-content payload of a tool result, or an
// empty JSON object when none is present.
func toolResultText(res *mcpsdk.CallToolResult) string {
	for _, c := range res.Content {
		if tc, ok := c.(*mcpsdk.TextContent); ok {
			return tc.Text
		}
	}
	return "{}"
}

// shapeCapture reduces the raw call responses into the baked fixture shape.
// "search" flattens each result's {results:[…]} (or a bare array) into the
// ranked search corpus the duckduckgo toolset replays; anything else bakes the
// raw responses verbatim.
func shapeCapture(shape string, calls []json.RawMessage) (any, error) {
	switch shape {
	case "search":
		out := []searchEntry{}
		for _, r := range calls {
			var env struct {
				Results []searchEntry `json:"results"`
			}
			if err := json.Unmarshal(r, &env); err == nil && len(env.Results) > 0 {
				out = append(out, env.Results...)
				continue
			}
			var arr []searchEntry
			if err := json.Unmarshal(r, &arr); err == nil {
				out = append(out, arr...)
			}
		}
		return out, nil
	default:
		return calls, nil
	}
}

func (a *app) captureCmd() *cobra.Command {
	var (
		scenario string
		out      string
	)
	cmd := &cobra.Command{
		Use:   "capture",
		Short: "Capture fixtures from real MCP servers (generation-time, network-allowed)",
		Long: "Opt-in record-and-replay: contacts the real no-auth MCP servers named in a " +
			"scenario's fixture/capture.json, records their tool schemas and representative " +
			"responses, and bakes them into the fixture for hermetic replay. Requires network at " +
			"generation time; it is never run during a benchmark.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if scenario == "" {
				fmt.Fprintln(a.errOut, "ozy-bench capture: --scenario is required")
				return nil
			}
			src := filepath.Join("scenarios", scenario, "fixture")
			spec, ok, err := LoadCaptureSpec(filepath.Join(src, "capture.json"))
			if err != nil {
				fmt.Fprintf(a.errOut, "ozy-bench capture: %v\n", err)
				return nil
			}
			if !ok {
				fmt.Fprintf(a.out, "No capture.json for scenario %q — fixtures are hand-authored, nothing to capture.\n", scenario)
				return nil
			}
			if out == "" {
				out = src
			}
			if err := CaptureFixture(cmd.Context(), spec, out, nil); err != nil {
				fmt.Fprintf(a.errOut, "ozy-bench capture: %v\n", err)
				return nil
			}
			fmt.Fprintf(a.out, "Captured %d server(s) into %s\n", len(spec.Servers), out)
			return nil
		},
	}
	cmd.Flags().StringVar(&scenario, "scenario", "", "scenario whose fixture/capture.json to run")
	cmd.Flags().StringVar(&out, "out", "", "output fixture dir (default: scenarios/<scenario>/fixture)")
	return cmd
}
