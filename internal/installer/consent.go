package installer

import (
	"bufio"
	"fmt"
	"io"
	"strings"
)

// Risk classifies how much consent an action needs.
type Risk int

const (
	// Ordinary actions (create a Ozy-managed directory, write a fresh config)
	// may be auto-accepted by --yes.
	Ordinary Risk = iota
	// Risky actions (downloads, dependency installs, PATH/shell-profile edits,
	// creating managed runtimes, deleting user config/data, purge, or anything
	// outside the Ozy-managed directory) always require an explicit answer.
	// --yes never auto-accepts them.
	Risky
)

// Decision is the resolved consent outcome for an action, computed before any IO.
type Decision int

const (
	// Proceed means the action is allowed without prompting (ordinary + --yes).
	Proceed Decision = iota
	// AskUser means the run is interactive and must prompt before acting.
	AskUser
	// SkipNoConsent means consent is required but cannot be obtained
	// (non-interactive), so the action is skipped and the caller prints a manual
	// instruction.
	SkipNoConsent
)

// ConsentPolicy decides whether an action needs a prompt given the run flags.
// It is pure so the consent boundary is testable without IO.
type ConsentPolicy struct {
	AssumeYes   bool // --yes was passed
	Interactive bool // stdin is a TTY we can prompt on
}

// NeedsPrompt reports whether the user must be asked before an action of the
// given risk. Risky actions always need a prompt, even under --yes.
func (p ConsentPolicy) NeedsPrompt(r Risk) bool {
	if r == Risky {
		return true
	}
	return !p.AssumeYes
}

// Decide resolves the consent outcome for an action of the given risk.
func (p ConsentPolicy) Decide(r Risk) Decision {
	if !p.NeedsPrompt(r) {
		return Proceed
	}
	if p.Interactive {
		return AskUser
	}
	return SkipNoConsent
}

// askConsent resolves consent for an action of the given risk and performs the
// IO: it prompts when interactive and skips (logging a manual note) when consent
// cannot be obtained. Both the install and uninstall flows share it.
func askConsent(consent ConsentPolicy, r Risk, in io.Reader, out io.Writer, log *Logger, what string) bool {
	switch consent.Decide(r) {
	case Proceed:
		return true
	case AskUser:
		fmt.Fprintf(out, "%s\n  Proceed? [y/N] ", what)
		if Confirm(in, false) {
			return true
		}
		if log != nil {
			log.Sayf("Declined: %s", what)
		}
		return false
	default: // SkipNoConsent
		if log != nil {
			log.Sayf("Skipped (needs consent, non-interactive): %s", what)
		}
		return false
	}
}

// Confirm reads a yes/no answer from r, returning def for an empty or
// unrecognized line. It is the thin IO layer used when Decide returns AskUser.
func Confirm(r io.Reader, def bool) bool {
	line, _ := bufio.NewReader(r).ReadString('\n')
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes":
		return true
	case "n", "no":
		return false
	default:
		return def
	}
}
