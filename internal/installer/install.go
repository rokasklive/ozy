// Package installer drives Ozy's setup and teardown as resumable, idempotent
// step state machines. Run is the install entrypoint called by cmd/ozy-install.
package installer

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/rokasklive/ozy/internal/paths"
)

// Options are the parsed bootstrap flags.
type Options struct {
	DryRun    bool // --dry-run / --plan: inspect and print only, never mutate
	AssumeYes bool // --yes: auto-accept ordinary confirmations (never risky)
	Manual    bool // --manual: print a guided checklist instead of installing
	Verbose   bool // --verbose: stream detailed log lines to the terminal
	NoColor   bool // --no-color: disable ANSI color
}

// Run executes the dry-run-first install flow: inspect, print the plan, and —
// unless this is a plan/dry-run — confirm before performing any mutation.
func Run(opts Options) error {
	p := paths.Resolve()
	plat := DetectPlatform(os.Stdout, opts.NoColor)
	plan := BuildPlan(plat, p, NewDepChecker())

	printIntro(os.Stdout)

	// Manual/guided mode is a read-only checklist: print and exit, no prompts.
	if opts.Manual {
		RenderPlan(os.Stdout, plan)
		printManualChecklist(os.Stdout)
		return nil
	}

	RenderPlan(os.Stdout, plan)

	// --dry-run / --plan short-circuit before any consent or mutation.
	if opts.DryRun {
		fmt.Fprintf(os.Stdout, "\nDry run: no changes made. Re-run without --dry-run to install.\n")
		return nil
	}

	consent := ConsentPolicy{
		AssumeYes:   opts.AssumeYes,
		Interactive: isTerminal(os.Stdin),
	}

	// Dry-run-first confirmation. The overall go-ahead is an ordinary action, so
	// --yes accepts it; interactive prompts; non-interactive without --yes stops
	// before any mutation.
	switch consent.Decide(Ordinary) {
	case AskUser:
		fmt.Fprint(os.Stdout, "\nProceed with this installation? [Y/n] ")
		if !Confirm(os.Stdin, true) {
			fmt.Fprintln(os.Stdout, "No changes made.")
			return nil
		}
	case SkipNoConsent:
		fmt.Fprintln(os.Stdout, "\nNon-interactive: re-run with --yes to proceed, or --dry-run to preview.")
		return nil
	case Proceed:
		// --yes: continue without prompting.
	}

	return execute(opts, plan, p, consent)
}

// execute opens the log and runs the install state machine after consent.
func execute(opts Options, plan Plan, p paths.Paths, consent ConsentPolicy) error {
	log, err := NewLogger(p.LogDir, "install", os.Stdout)
	if err != nil {
		return fmt.Errorf("open install log: %w", err)
	}
	defer func() { _ = log.Close() }()
	log.Verbose = opts.Verbose
	log.Logf("ozy-install %s starting on %s/%s", plan.OzyVersion, plan.Platform.OS, plan.Platform.Arch)

	store := NewStateStore(filepath.Join(p.StateDir, "install-state.json"))
	state, err := store.Load()
	if err != nil {
		return fmt.Errorf("load install state: %w", err)
	}
	state.OzyVersion = plan.OzyVersion

	c := &execContext{
		ctx:     context.Background(),
		plan:    &plan,
		paths:   p,
		log:     log,
		consent: consent,
		stdin:   os.Stdin,
		term:    os.Stdout,
		run:     execRunner,
		prog:    NewProgress(os.Stdout, plan.Platform),
		state:   state,
		store:   store,
		retry:   retryCommand(),
	}

	if err := c.execute(installSteps()); err != nil {
		// The actionable StepError is returned and printed by main (to stderr,
		// beneath the failed step's line); record it in the durable log too.
		log.Logf("install failed: %v", err)
		return err
	}
	return nil
}

func printIntro(w io.Writer) {
	fmt.Fprintln(w, "Ozy installer")
	fmt.Fprintln(w, "Boring, transparent, safe to rerun. Dry-run first — nothing changes without your say-so.")
	fmt.Fprintln(w)
}

func printManualChecklist(w io.Writer) {
	fmt.Fprintln(w, "\nManual / guided mode — perform these steps yourself:")
	for i, s := range installSteps() {
		fmt.Fprintf(w, "\n%d. %s\n", i+1, s.title)
		fmt.Fprintf(w, "   what:   %s\n", s.what)
		fmt.Fprintf(w, "   why:    %s\n", s.why)
		fmt.Fprintf(w, "   verify: %s\n", s.verify)
		fmt.Fprintf(w, "   next:   %s\n", s.next)
	}
}

// retryCommand is the exact, safe command to re-run after a failure. State makes
// the rerun resume from the failed step regardless of flags.
func retryCommand() string {
	if v, ok := releaseVersion(); ok {
		return "go run github.com/rokasklive/ozy/cmd/ozy-install@" + v
	}
	return "go run ./cmd/ozy-install"
}
