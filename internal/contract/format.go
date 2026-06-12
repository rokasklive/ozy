package contract

// Output format identifiers shared by every adapter (SPEC.md §15). Human is the
// default; JSON is a single machine-readable document for agents and evals;
// concise is a terse mode for token-sensitive use.
const (
	FormatHuman   = "human"
	FormatJSON    = "json"
	FormatConcise = "concise"
)

// Renderable is implemented by every contract result so the render layer can
// produce human and concise text. JSON output is handled by marshaling the value
// directly and does not go through Render.
type Renderable interface {
	Render(format string) string
}
