package installer

import (
	"fmt"
	"io"
)

// Progress renders per-step status to the terminal. On a colour-capable TTY it
// uses coloured status glyphs; otherwise it emits the same lines without ANSI,
// preserving the step semantics for non-TTY / CI / --no-color output.
//
// ponytail: static per-step lines, not an in-place repaint with an animated
// spinner. Chosen deliberately — consent prompts and copy-paste PATH
// instructions print mid-execution, and a repainting dashboard would corrupt
// scrollback and garble those. The plan already lists every step up front, so
// "every step visible" holds. Upgrade path: a repainting dashboard with all
// consent gathered up front, behind this same Start/Done/Skip/Fail interface.
type Progress struct {
	w     io.Writer
	color bool
}

// NewProgress renders to w. Colour is enabled only when the platform reports a
// colour-capable interactive terminal.
func NewProgress(w io.Writer, plat Platform) *Progress {
	return &Progress{w: w, color: !plat.Plain()}
}

// Start marks a step as now running.
func (p *Progress) Start(title string) { fmt.Fprintf(p.w, "%s %s\n", p.paint("→", dim), title) }

// Done marks a step as completed.
func (p *Progress) Done(title string) { fmt.Fprintf(p.w, "%s %s\n", p.paint("✓", green), title) }

// Skip marks a step that was already complete on a rerun.
func (p *Progress) Skip(title string) {
	fmt.Fprintf(p.w, "%s %s (already done)\n", p.paint("✓", green), title)
}

// Fail keeps the failed step's line visible; the actionable error is printed by
// the caller immediately beneath it.
func (p *Progress) Fail(title string) { fmt.Fprintf(p.w, "%s %s\n", p.paint("✗", red), title) }

const (
	dim   = "2"
	green = "32"
	red   = "31"
)

func (p *Progress) paint(s, code string) string {
	if !p.color {
		return s
	}
	return "\033[" + code + "m" + s + "\033[0m"
}
