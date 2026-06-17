package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/rokasklive/ozy/internal/config"
	"github.com/rokasklive/ozy/internal/contract"
	"github.com/rokasklive/ozy/internal/eval"
	ozymcp "github.com/rokasklive/ozy/internal/mcp"
)

func (a *app) initCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Scaffold a starter configuration file",
		RunE: func(*cobra.Command, []string) error {
			if err := config.WriteStarter(a.configPath); err != nil {
				a.emit(&contract.Message{OK: false, Message: err.Error()})
				a.exitCode = 1
				return nil
			}
			a.emit(&contract.Message{OK: true, Message: "Wrote starter configuration to " + a.configPath})
			return nil
		},
	}
}

func (a *app) daemonCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "daemon",
		Short: "Run the Ozy daemon",
		RunE: func(cmd *cobra.Command, _ []string) error {
			d, ok := a.load()
			if !ok {
				return nil
			}
			ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
			defer stop()
			return d.Run(ctx, a.errOut)
		},
	}
}

func (a *app) mcpCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "mcp",
		Short: "Serve the Ozy MCP adapter over stdio",
		RunE: func(*cobra.Command, []string) error {
			d, ok := a.load()
			if !ok {
				return nil
			}
			ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
			defer stop()
			adapter := ozymcp.New(d.Broker(), Version, mcpBreadcrumb(d.Config()))
			// stdout carries the JSON-RPC stream, so diagnostics go to stderr only.
			// A cancelled context (signal) or client disconnect (EOF) is a clean
			// shutdown, not a failure.
			err := adapter.Serve(ctx)
			if err != nil && ctx.Err() == nil && !errors.Is(err, io.EOF) {
				fmt.Fprintln(a.errOut, "ozy mcp: "+err.Error())
				a.exitCode = 1
			}
			return nil
		},
	}
}

// mcpBreadcrumb builds the findTool capability breadcrumb from the enabled
// downstream servers, honoring surface.capabilityBreadcrumb. It returns "" when
// the breadcrumb is disabled or no servers are configured.
func mcpBreadcrumb(cfg *config.Loaded) string {
	if cfg == nil || cfg.Resolved == nil || !cfg.Resolved.Surface.CapabilityBreadcrumb {
		return ""
	}
	var ids []string
	for id, s := range cfg.Resolved.MCP {
		if s.IsEnabled() {
			ids = append(ids, id)
		}
	}
	return ozymcp.Breadcrumb(ids)
}

func (a *app) indexCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "index",
		Short: "Refresh and index downstream tool catalogs",
		RunE: func(*cobra.Command, []string) error {
			d, ok := a.load()
			if !ok {
				return nil
			}
			// Index provisions the sidecar and embeds when semantic is enabled —
			// the same path the daemon uses — so the CLI never silently runs
			// lexical-only. Shut the one-shot sidecar down before exiting.
			summary := d.Index(context.Background(), a.errOut)
			d.Shutdown()
			a.emit(summary)
			if !summary.OK {
				a.exitCode = 1
			}
			return nil
		},
	}
}

func (a *app) listCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List indexed tools in the catalog",
		RunE: func(*cobra.Command, []string) error {
			d, ok := a.load()
			if !ok {
				return nil
			}
			res, err := d.Broker().List(context.Background())
			if err != nil {
				a.emitError(asError(err))
				return nil
			}
			a.emit(res)
			return nil
		},
	}
}

func (a *app) searchCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "search <query>",
		Short: "Find the best known tool for a capability query",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			d, ok := a.load()
			if !ok {
				return nil
			}
			// FindTool encodes outcomes (including catalog_empty) as decisions,
			// never Go errors, so this is always an exit-0 instructional result.
			res, _ := d.Broker().FindTool(context.Background(), args[0])
			a.emit(res)
			return nil
		},
	}
}

func (a *app) describeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "describe <toolRef>",
		Short: "Show schema and usage for one known toolRef",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			d, ok := a.load()
			if !ok {
				return nil
			}
			res, err := d.Broker().DescribeTool(context.Background(), args[0])
			if err != nil {
				a.emitError(asError(err))
				return nil
			}
			a.emit(res)
			return nil
		},
	}
}

func (a *app) callCmd() *cobra.Command {
	var jsonArgs string
	cmd := &cobra.Command{
		Use:   "call <toolRef> --json '<arguments>'",
		Short: "Invoke a downstream tool through Ozy",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			d, ok := a.load()
			if !ok {
				return nil
			}
			var parsed map[string]any
			if jsonArgs != "" {
				if err := json.Unmarshal([]byte(jsonArgs), &parsed); err != nil {
					a.emitError(&contract.Error{
						Type:             contract.ErrTypeArgumentValidationFailed,
						ToolRef:          args[0],
						Retryable:        false,
						Message:          fmt.Sprintf("--json is not a valid JSON object: %v", err),
						AgentInstruction: "Pass arguments as a JSON object, e.g. --json '{\"query\":\"...\"}'.",
					})
					return nil
				}
			}
			res, err := d.Broker().CallTool(context.Background(), args[0], parsed)
			if err != nil {
				a.emitError(asError(err))
				return nil
			}
			a.emit(res)
			return nil
		},
	}
	cmd.Flags().StringVar(&jsonArgs, "json", "", "tool arguments as a JSON object")
	return cmd
}

func (a *app) evalCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "eval",
		Short: "Run Ozy evaluation scenarios",
	}
	cmd.AddCommand(a.evalRunCmd(), a.evalReportCmd())
	return cmd
}

// evalRunCmd runs the eval harness over the committed corpus. It does not touch
// the user's config or catalog — the corpus is embedded — so it never calls load().
func (a *app) evalRunCmd() *cobra.Command {
	var out string
	var semantic bool
	cmd := &cobra.Command{
		Use:   "run [family]",
		Short: "Run the eval suite (optionally scoped to one family) over the committed corpus",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			opts := eval.Options{OutDir: out, Semantic: semantic}
			if len(args) == 1 {
				opts.Families = []string{args[0]}
			}
			// Supply the real-model semantic builder whenever the leg may run
			// (flag or env). eval.Run skips cleanly if provisioning fails.
			if semantic || os.Getenv("OZY_EVAL_SEMANTIC") == "1" {
				opts.SemanticBuilder = sidecarSemanticBuilder("")
			}
			res, err := eval.Run(context.Background(), opts)
			if err != nil {
				a.emitError(evalError(err))
				return nil
			}
			a.emit(res)
			if res.Failed() {
				a.exitCode = 1
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&out, "out", "evals", "directory to write the snapshot and BENCHMARKS.md (empty to skip writing)")
	cmd.Flags().BoolVar(&semantic, "semantic", false, "run the real-model semantic leg (also enabled via OZY_EVAL_SEMANTIC=1)")
	return cmd
}

// evalReportCmd shows the latest committed benchmark snapshot.
func (a *app) evalReportCmd() *cobra.Command {
	var out string
	cmd := &cobra.Command{
		Use:   "report",
		Short: "Show the latest benchmark snapshot",
		Args:  cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			res, err := eval.LoadSnapshot(out)
			if err != nil {
				a.emitError(&contract.Error{
					Type:             contract.ErrTypeConfigError,
					Retryable:        false,
					Message:          err.Error(),
					AgentInstruction: "Run `ozy eval run --out evals` to produce a benchmark snapshot first.",
				})
				return nil
			}
			a.emit(res)
			return nil
		},
	}
	cmd.Flags().StringVar(&out, "out", "evals", "directory containing snapshots/baseline.json")
	return cmd
}

// evalError wraps a harness failure as a structured CONFIG_ERROR (the eval suite
// is dev/CI tooling; a failure is a setup/corpus problem, not a broker call).
func evalError(err error) *contract.Error {
	return &contract.Error{
		Type:             contract.ErrTypeConfigError,
		Retryable:        false,
		Message:          err.Error(),
		AgentInstruction: "Fix the eval corpus or arguments named in the message, then re-run `ozy eval run`.",
	}
}

// asError unwraps a broker error into its structured contract form, synthesizing
// one if a non-contract error ever surfaces.
func asError(err error) *contract.Error {
	var ce *contract.Error
	if errors.As(err, &ce) {
		return ce
	}
	return &contract.Error{
		Type:             contract.ErrTypeDownstreamCallFailed,
		Retryable:        false,
		Message:          err.Error(),
		AgentInstruction: "Report this unexpected failure; do not retry blindly.",
	}
}
