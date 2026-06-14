package installer

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/rokasklive/ozy/internal/paths"
)

// UninstallOptions are the parsed uninstall flags.
type UninstallOptions struct {
	DryRun     bool // print the removal plan and exit
	AssumeYes  bool // auto-accept ordinary removals (never config/purge)
	KeepConfig bool // preserve config even under --purge
	KeepData   bool // preserve data/models
	Purge      bool // also remove config (requires a distinct confirmation)
	NoColor    bool
}

// removalCategory groups removals so conservative defaults and --keep-* flags can
// drop whole categories from the plan.
type removalCategory string

const (
	catBinary  removalCategory = "binary"
	catRuntime removalCategory = "managed-runtime (venv, logs, catalog, state)"
	catCache   removalCategory = "cache"
	catData    removalCategory = "data/models"
	catConfig  removalCategory = "config + downstream MCP definitions"
)

// PATH-block cleanup is handled by cleanupPath (not a filesystem removal entry),
// so it has no removalCategory.

type removal struct {
	cat   removalCategory
	paths []string
	risky bool // needs its own explicit consent even after the overall go-ahead
}

// Uninstall runs the plan-first, consent-based removal flow. It mirrors the
// installer: detect, show the plan, confirm, then remove. Every removal is
// idempotent (a missing target is a no-op), so an interrupted run is safe to
// rerun.
//
// ponytail: no persisted uninstall-state file. RemoveAll idempotency already
// makes reruns safe, and the state would otherwise live inside the very
// directory the run deletes. Add one only if a non-idempotent removal appears.
func Uninstall(opts UninstallOptions) error {
	p := paths.Resolve()
	plat := DetectPlatform(os.Stdout, opts.NoColor)
	removals, preserved := planRemovals(p, opts)

	printIntroUninstall(os.Stdout)
	printUninstallPlan(os.Stdout, removals, preserved)

	if opts.DryRun {
		fmt.Fprintln(os.Stdout, "\nDry run: nothing was removed.")
		return nil
	}

	consent := ConsentPolicy{AssumeYes: opts.AssumeYes, Interactive: isTerminal(os.Stdin)}

	switch consent.Decide(Ordinary) {
	case AskUser:
		fmt.Fprint(os.Stdout, "\nProceed with uninstall? [y/N] ")
		if !Confirm(os.Stdin, false) {
			fmt.Fprintln(os.Stdout, "Nothing removed.")
			return nil
		}
	case SkipNoConsent:
		fmt.Fprintln(os.Stdout, "\nNon-interactive: re-run with --yes (and --purge for config), or --dry-run.")
		return nil
	case Proceed:
	}

	// Purge of the config needs a distinct, explicit confirmation: --yes alone
	// never deletes config or downstream MCP definitions.
	if hasCategory(removals, catConfig) && !confirmPurge(consent, os.Stdin, os.Stdout) {
		fmt.Fprintln(os.Stdout, "Purge not confirmed — keeping config and MCP definitions.")
		removals = dropCategory(removals, catConfig)
		preserved = append(preserved, catConfig)
	}

	// The uninstall log lives outside the state tree it removes, so the durable
	// record (and the path printed in the summary) survives the run.
	log, err := NewLogger(filepath.Join(os.TempDir(), "ozy-uninstall-logs"), "uninstall", os.Stdout)
	if err != nil {
		return fmt.Errorf("open uninstall log: %w", err)
	}
	defer func() { _ = log.Close() }()
	prog := NewProgress(os.Stdout, plat)

	for _, r := range removals {
		if r.risky && !askConsent(consent, Risky, os.Stdin, os.Stdout, log, "Remove "+string(r.cat)) {
			prog.Skip(string(r.cat))
			continue
		}
		prog.Start(string(r.cat))
		if err := removeAll(r.paths, log); err != nil {
			prog.Fail(string(r.cat))
			return &StepError{
				Step: "Remove " + string(r.cat), Cause: err,
				Impact: "uninstall left some files behind", SafeRetry: true,
				Next: "go run github.com/rokasklive/ozy/cmd/ozy-install@latest uninstall", LogPath: log.Path(),
			}
		}
		prog.Done(string(r.cat))
	}

	cleanupPath(p, plat, consent, os.Stdin, os.Stdout, log)
	printUninstallSummary(os.Stdout, log, preserved)
	return nil
}

// planRemovals categorises what to remove given the flags. Config is preserved
// unless --purge; data is removed unless --keep-data; the rest are always
// removed. Only categories with at least one present target are included.
func planRemovals(p paths.Paths, opts UninstallOptions) (remove []removal, preserved []removalCategory) {
	add := func(cat removalCategory, risky bool, targets ...string) {
		var present []string
		for _, t := range targets {
			if pathPresent(t) {
				present = append(present, t)
			}
		}
		if len(present) > 0 {
			remove = append(remove, removal{cat: cat, paths: present, risky: risky})
		}
	}

	add(catBinary, false, p.BinaryPath)
	add(catRuntime, false, p.StateDir)
	add(catCache, false, p.CacheDir)

	if opts.KeepData {
		preserved = append(preserved, catData)
	} else {
		add(catData, false, p.DataDir)
	}

	if opts.Purge && !opts.KeepConfig {
		add(catConfig, true, p.ConfigFile)
	} else {
		preserved = append(preserved, catConfig)
	}
	return remove, preserved
}

func printIntroUninstall(w io.Writer) {
	fmt.Fprintln(w, "Ozy uninstaller")
	fmt.Fprintln(w, "Plan-first and conservative. Your config and MCP definitions are kept unless you --purge.")
	fmt.Fprintln(w)
}

func printUninstallPlan(w io.Writer, removals []removal, preserved []removalCategory) {
	if len(removals) == 0 {
		fmt.Fprintln(w, "Nothing to remove — no Ozy-managed files were found.")
		return
	}
	fmt.Fprintln(w, "Will remove:")
	for _, r := range removals {
		for _, path := range r.paths {
			fmt.Fprintf(w, "  - %-50s [%s]\n", path, r.cat)
		}
	}
	if len(preserved) > 0 {
		fmt.Fprintln(w, "\nWill preserve:")
		for _, c := range preserved {
			fmt.Fprintf(w, "  - %s\n", c)
		}
	}
	fmt.Fprintln(w, "\nNothing has been removed yet.")
}

func printUninstallSummary(w io.Writer, log *Logger, preserved []removalCategory) {
	fmt.Fprintln(w, "\nOzy has been uninstalled.")
	if len(preserved) > 0 {
		fmt.Fprintln(w, "Preserved:")
		for _, c := range preserved {
			fmt.Fprintf(w, "  - %s\n", c)
		}
	}
	fmt.Fprintf(w, "Log: %s\n", log.Path())
}

// confirmPurge requires the literal word "purge" — a distinct, explicit
// confirmation that --yes does not satisfy. Non-interactive runs cannot confirm.
func confirmPurge(consent ConsentPolicy, in io.Reader, out io.Writer) bool {
	if !consent.Interactive {
		return false
	}
	fmt.Fprint(out, "\nPurge will DELETE your config and downstream MCP definitions.\nType 'purge' to confirm: ")
	line, _ := bufio.NewReader(in).ReadString('\n')
	return strings.TrimSpace(line) == "purge"
}

// cleanupPath removes only the installer-added marked block from the shell rc,
// with consent. It never edits anything else.
func cleanupPath(p paths.Paths, plat Platform, consent ConsentPolicy, in io.Reader, out io.Writer, log *Logger) {
	if plat.OS == "windows" {
		log.Sayf("If you added %s to your user PATH, remove it via System Settings > Environment Variables.", p.UserBinDir)
		return
	}
	rc, _ := rcFileFor(os.Getenv("SHELL"), homeDir())
	data, err := os.ReadFile(rc) //nolint:gosec // user rc path
	if err != nil || !strings.Contains(string(data), pathBlockBegin) {
		return // no installer block to clean up
	}
	if !askConsent(consent, Risky, in, out, log, fmt.Sprintf("Remove the Ozy PATH block from %s", rc)) {
		log.Sayf("Left the PATH block in %s — remove it manually if you wish.", rc)
		return
	}
	if _, err := removePathBlock(rc); err != nil {
		log.Sayf("Could not edit %s: %v", rc, err)
		return
	}
	log.Sayf("Removed the Ozy PATH block from %s.", rc)
}

func removeAll(targets []string, log *Logger) error {
	for _, t := range targets {
		if err := os.RemoveAll(t); err != nil {
			return fmt.Errorf("remove %s: %w", t, err)
		}
		log.Logf("removed %s", t)
	}
	return nil
}

func pathPresent(path string) bool {
	if path == "" {
		return false
	}
	_, err := os.Stat(path)
	return err == nil
}

func hasCategory(rs []removal, cat removalCategory) bool {
	for _, r := range rs {
		if r.cat == cat {
			return true
		}
	}
	return false
}

func dropCategory(rs []removal, cat removalCategory) []removal {
	out := rs[:0:0]
	for _, r := range rs {
		if r.cat != cat {
			out = append(out, r)
		}
	}
	return out
}
