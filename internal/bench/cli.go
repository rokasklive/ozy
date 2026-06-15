// Package bench implements the scenario benchmark harness: a controlled,
// local-first, reproducible comparison of direct-MCP vs Ozy-brokered agent
// performance against a frozen incident scenario.
package bench

import (
	"io"

	"github.com/spf13/cobra"
)

// Version is the build version reported by `ozy-bench --version`.
const Version = "0.1.0-dev"

// app holds shared CLI state: resolved flags and output writers.
type app struct {
	out    io.Writer
	errOut io.Writer
}

// Execute runs the CLI with the given args and writers, returning the process
// exit code.
func Execute(args []string, stdout, stderr io.Writer) int {
	a := &app{out: stdout, errOut: stderr}
	root := a.rootCmd()
	root.SetArgs(args)
	root.SetOut(stdout)
	root.SetErr(stderr)
	if err := root.Execute(); err != nil {
		return 1
	}
	return 0
}

func (a *app) rootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:           "ozy-bench",
		Short:         "Ozy scenario benchmark harness",
		Long:          "Runs a frozen incident scenario in direct and ozy modes and emits a side-by-side comparison of tool-surface, token-economy, and task-success metrics.",
		Version:       Version,
		SilenceErrors: false,
		SilenceUsage:  true,
	}
	root.AddCommand(
		a.runCmd(),
		a.mcpCmd(),
		a.fixtureCmd(),
	)
	return root
}
