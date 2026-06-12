package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/rokask/ozy/internal/config"
	"github.com/rokask/ozy/internal/contract"
	ozyindex "github.com/rokask/ozy/internal/index"
	ozymcp "github.com/rokask/ozy/internal/mcp"
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
			adapter := ozymcp.New(d.Broker(), Version)
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

func (a *app) indexCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "index",
		Short: "Refresh and index downstream tool catalogs",
		RunE: func(*cobra.Command, []string) error {
			d, ok := a.load()
			if !ok {
				return nil
			}
			summary := ozyindex.New(d.Store(), nil).Run(context.Background(), d.Config().Resolved)
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
	cmd.AddCommand(&cobra.Command{
		Use:   "run <scenario>",
		Short: "Run a single evaluation scenario",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(*cobra.Command, []string) error {
			a.emitError(contract.NotImplemented("ozy eval run"))
			return nil
		},
	})
	return cmd
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
