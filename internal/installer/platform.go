package installer

import (
	"os"
	"runtime"
	"strconv"

	"golang.org/x/term"
)

// Platform captures the host OS/arch and the terminal capabilities the plan
// renderer, progress dashboard, and prompts depend on. It is detected once at
// startup and is otherwise inert (no mutation).
type Platform struct {
	OS    string // runtime.GOOS
	Arch  string // runtime.GOARCH
	TTY   bool   // stdout is an interactive terminal
	Width int    // terminal width in columns (best-effort; 80 default)
	Color bool   // ANSI color is safe to emit
}

// DetectPlatform inspects the host and the given stdout stream. noColor forces
// color off regardless of the terminal (the --no-color flag).
func DetectPlatform(stdout *os.File, noColor bool) Platform {
	tty := isTerminal(stdout)
	return Platform{
		OS:    runtime.GOOS,
		Arch:  runtime.GOARCH,
		TTY:   tty,
		Width: termWidth(stdout),
		Color: tty && !noColor && os.Getenv("NO_COLOR") == "",
	}
}

// Plain reports whether output should use the static, no-ANSI fallback: a
// non-terminal, a CI environment, or color explicitly disabled.
func (p Platform) Plain() bool {
	return !p.TTY || !p.Color || os.Getenv("CI") != ""
}

// isTerminal reports whether f is a real interactive terminal — not a pipe and
// not a character device like /dev/null. Correctness here is load-bearing: it
// gates whether the installer may prompt for consent.
func isTerminal(f *os.File) bool {
	return f != nil && term.IsTerminal(int(f.Fd()))
}

// termWidth returns the real terminal width, falling back to COLUMNS and then a
// conservative default for non-terminals.
func termWidth(f *os.File) int {
	if f != nil {
		if w, _, err := term.GetSize(int(f.Fd())); err == nil && w > 0 {
			return w
		}
	}
	if c := os.Getenv("COLUMNS"); c != "" {
		if n, err := strconv.Atoi(c); err == nil && n > 0 {
			return n
		}
	}
	return 80
}
