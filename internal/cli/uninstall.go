package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/rokasklive/ozy/internal/installer"
)

// uninstallCmd delegates to the installer's plan-first, consent-based removal
// flow. The bootstrap (`go run …/cmd/ozy-install uninstall`) and this command
// share the same code, so removal behavior cannot diverge.
func (a *app) uninstallCmd() *cobra.Command {
	var opts installer.UninstallOptions
	cmd := &cobra.Command{
		Use:   "uninstall",
		Short: "Remove Ozy (plan-first; keeps config and MCP definitions unless --purge)",
		Long: "Remove Ozy-managed files. Conservative by default: the binary, managed " +
			"runtime, cache, and data are removed, but your config and downstream MCP " +
			"definitions are preserved unless you pass --purge and confirm. Destructive " +
			"removals are never performed by --yes alone.",
		RunE: func(*cobra.Command, []string) error {
			if err := installer.Uninstall(opts); err != nil {
				fmt.Fprintln(a.errOut, err)
				a.exitCode = 1
			}
			return nil
		},
	}
	f := cmd.Flags()
	f.BoolVar(&opts.DryRun, "dry-run", false, "show the removal plan and exit without changes")
	f.BoolVar(&opts.AssumeYes, "yes", false, "auto-accept ordinary removals (never config/purge)")
	f.BoolVar(&opts.KeepConfig, "keep-config", false, "preserve config and MCP definitions even under --purge")
	f.BoolVar(&opts.KeepData, "keep-data", false, "preserve data/models")
	f.BoolVar(&opts.Purge, "purge", false, "also remove config (requires typing 'purge' to confirm)")
	f.BoolVar(&opts.NoColor, "no-color", false, "disable ANSI color")
	return cmd
}
