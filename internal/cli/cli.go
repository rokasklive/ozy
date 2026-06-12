// Package cli implements the `ozy` command surface (SPEC.md §15). Every
// broker-backed command routes through the same shared broker the MCP adapter
// uses, so the two adapters cannot diverge. Output honors a global --format flag
// (human, json, concise); operations whose behavior is deferred return a
// structured NOT_IMPLEMENTED result and a non-zero exit code.
package cli

import (
	"io"

	"github.com/spf13/cobra"

	"github.com/rokask/ozy/internal/config"
	"github.com/rokask/ozy/internal/contract"
	"github.com/rokask/ozy/internal/daemon"
	"github.com/rokask/ozy/internal/render"
)

// Version is the build version reported by `ozy --version` and the MCP adapter.
const Version = "0.1.0-dev"

// app holds shared CLI state: resolved flags and output writers.
type app struct {
	out        io.Writer
	errOut     io.Writer
	configPath string
	format     string
	exitCode   int
}

// Execute runs the CLI with the given args and writers, returning the process
// exit code. Cobra usage errors yield exit code 1; structured failures rendered
// by commands set the code via the app.
func Execute(args []string, stdout, stderr io.Writer) int {
	a := &app{out: stdout, errOut: stderr, format: contract.FormatHuman}
	root := a.rootCmd()
	root.SetArgs(args)
	root.SetOut(stdout)
	root.SetErr(stderr)
	if err := root.Execute(); err != nil {
		return 1
	}
	return a.exitCode
}

func (a *app) rootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:           "ozy",
		Short:         "Ozy is a local agent tool broker",
		Long:          "Ozy discovers downstream MCP tools into a searchable catalog and brokers their invocation through a small, stable agent interface.",
		Version:       Version,
		SilenceErrors: false,
		SilenceUsage:  true,
		PersistentPreRun: func(*cobra.Command, []string) {
			a.format, _ = render.Normalize(a.format)
		},
	}
	root.PersistentFlags().StringVar(&a.configPath, "config", config.DefaultPath(),
		"path to the Ozy configuration file")
	root.PersistentFlags().StringVar(&a.format, "format", contract.FormatHuman,
		"output format: human, json, or concise")

	root.AddCommand(
		a.initCmd(),
		a.daemonCmd(),
		a.mcpCmd(),
		a.indexCmd(),
		a.doctorCmd(),
		a.listCmd(),
		a.searchCmd(),
		a.describeCmd(),
		a.callCmd(),
		a.evalCmd(),
	)
	return root
}

// emit renders a successful/instructional result in the selected format.
func (a *app) emit(v any) {
	_ = render.Output(a.out, a.format, v)
}

// emitError renders a structured failure envelope and marks a non-zero exit.
func (a *app) emitError(e *contract.Error) {
	a.emit(contract.NewErrorEnvelope(e))
	a.exitCode = 1
}

// load resolves configuration and builds the runtime, or renders a CONFIG_ERROR.
func (a *app) load() (*daemon.Daemon, bool) {
	cfg, cerr := config.Load(a.configPath)
	if cerr != nil {
		a.emitError(cerr)
		return nil, false
	}
	d, err := daemon.New(cfg)
	if err != nil {
		a.emitError(&contract.Error{
			Type:             contract.ErrTypeConfigError,
			Retryable:        true,
			Message:          err.Error(),
			AgentInstruction: "Check catalog storage permissions, then retry.",
		})
		return nil, false
	}
	return d, true
}
