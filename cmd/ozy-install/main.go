// Command ozy-install is the one-command bootstrap for Ozy. It inspects the
// machine, prints a plan, gets consent, and drives the install to a working
// state — or, as `ozy-install uninstall`, removes Ozy safely. It is a separate
// binary from `ozy` so it can run before `ozy` exists:
//
//	go run github.com/rokasklive/ozy/cmd/ozy-install@latest
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/rokasklive/ozy/internal/installer"
)

func main() {
	// `ozy-install uninstall [flags]` runs the removal flow.
	if len(os.Args) > 1 && os.Args[1] == "uninstall" {
		os.Exit(runUninstall(os.Args[2:]))
	}

	plan := flag.Bool("plan", false, "show the plan and exit without changes (alias of --dry-run)")
	dryRun := flag.Bool("dry-run", false, "show the plan and exit without changes")
	yes := flag.Bool("yes", false, "auto-accept ordinary confirmations (never risky actions)")
	manual := flag.Bool("manual", false, "print a guided checklist instead of installing")
	verbose := flag.Bool("verbose", false, "stream detailed log lines to the terminal")
	noColor := flag.Bool("no-color", false, "disable ANSI color")
	flag.Usage = usage
	flag.Parse()

	if err := installer.Run(installer.Options{
		DryRun:    *dryRun || *plan,
		AssumeYes: *yes,
		Manual:    *manual,
		Verbose:   *verbose,
		NoColor:   *noColor,
	}); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func runUninstall(args []string) int {
	fs := flag.NewFlagSet("ozy-install uninstall", flag.ExitOnError)
	dryRun := fs.Bool("dry-run", false, "show the removal plan and exit without changes")
	yes := fs.Bool("yes", false, "auto-accept ordinary removals (never config/purge)")
	keepConfig := fs.Bool("keep-config", false, "preserve config and MCP definitions even under --purge")
	keepData := fs.Bool("keep-data", false, "preserve data/models")
	purge := fs.Bool("purge", false, "also remove config (requires typing 'purge' to confirm)")
	noColor := fs.Bool("no-color", false, "disable ANSI color")
	_ = fs.Parse(args)

	if err := installer.Uninstall(installer.UninstallOptions{
		DryRun:     *dryRun,
		AssumeYes:  *yes,
		KeepConfig: *keepConfig,
		KeepData:   *keepData,
		Purge:      *purge,
		NoColor:    *noColor,
	}); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}

func usage() {
	fmt.Fprintln(os.Stderr, "Ozy installer — one-command setup for the Ozy MCP gateway.")
	fmt.Fprintln(os.Stderr, "\nUsage:")
	fmt.Fprintln(os.Stderr, "  go run github.com/rokasklive/ozy/cmd/ozy-install@latest [flags]")
	fmt.Fprintln(os.Stderr, "\nFlags:")
	flag.PrintDefaults()
}
