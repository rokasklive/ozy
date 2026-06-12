// Package render writes typed broker results to an io.Writer in the format the
// caller requested: human-readable (default), a single JSON document for agents
// and evals, or a terse concise mode (SPEC.md §15). Keeping formatting here means
// the broker and adapters never format output themselves.
package render

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/rokask/ozy/internal/contract"
)

// Normalize maps a raw format flag to a known format, defaulting unknown or empty
// values to human and reporting whether the input was recognized.
func Normalize(format string) (string, bool) {
	switch format {
	case contract.FormatHuman, contract.FormatJSON, contract.FormatConcise:
		return format, true
	case "":
		return contract.FormatHuman, true
	default:
		return contract.FormatHuman, false
	}
}

// Output writes v to w. In JSON mode it emits a single indented document so the
// whole response is machine-consumable; otherwise it uses the value's Render
// method, falling back to JSON if the value is not Renderable.
func Output(w io.Writer, format string, v any) error {
	if format == contract.FormatJSON {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(v)
	}
	if r, ok := v.(contract.Renderable); ok {
		_, err := fmt.Fprintln(w, r.Render(format))
		return err
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}
