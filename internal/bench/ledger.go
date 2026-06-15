package bench

import (
	"encoding/json"
	"fmt"
	"os"
)

// LedgerItem is a single entry in the JSONL context ledger.
type LedgerItem struct {
	RunID                string `json:"run_id"`
	Mode                 string `json:"mode"`
	Phase                string `json:"phase"`
	Source               string `json:"source"`
	Kind                 string `json:"kind"`
	Server               string `json:"server,omitempty"`
	Tool                 string `json:"tool,omitempty"`
	Bytes                int    `json:"bytes"`
	TokenCount           int    `json:"token_count"`
	TokenSource          string `json:"token_source"`
	IncludedInModelContext bool `json:"included_in_model_context"`
}

// LedgerKind values categorize ledger entries by their role in the context.
const (
	LedgerKindSystemPrompt      = "system_prompt"
	LedgerKindAgentInstruction  = "agent_instruction"
	LedgerKindToolSchema        = "tool_schema"
	LedgerKindToolCall          = "tool_call"
	LedgerKindToolResult        = "tool_result"
	LedgerKindAssistantMessage  = "assistant_message"
	LedgerKindUserMessage       = "user_message"
	LedgerKindError             = "error"
	LedgerKindFinalAnswer       = "final_answer"
)

// TokenSource values label how token counts were obtained.
const (
	TokenSourceMeasured   = "measured"
	TokenSourceEstimated  = "estimated"
)

// LedgerWriter appends LedgerItems to a JSONL file.
type LedgerWriter struct {
	f    *os.File
	enc  *json.Encoder
}

// NewLedgerWriter creates a ledger writer that appends JSON lines to path.
func NewLedgerWriter(path string) (*LedgerWriter, error) {
	f, err := os.Create(path)
	if err != nil {
		return nil, fmt.Errorf("create ledger file: %w", err)
	}
	return &LedgerWriter{f: f, enc: json.NewEncoder(f)}, nil
}

// Append writes one ledger item as a JSON line.
func (w *LedgerWriter) Append(item *LedgerItem) error {
	if err := w.enc.Encode(item); err != nil {
		return fmt.Errorf("write ledger item: %w", err)
	}
	return nil
}

// Close flushes and closes the underlying file.
func (w *LedgerWriter) Close() error {
	return w.f.Close()
}
