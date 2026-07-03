// Package contract defines Ozy's agent-facing response and error models.
//
// These types are the wire shapes for the findTool, describeTool, and callTool
// contracts described in SPEC.md §9. They are shared by the broker, the CLI, and
// the MCP adapter so that every adapter emits the same instructional responses.
package contract

import "fmt"

// Error type values returned in structured failures (SPEC.md §9.3).
const (
	ErrTypeToolNotFound             = "TOOL_NOT_FOUND"
	ErrTypeDownstreamServerOffline  = "DOWNSTREAM_SERVER_OFFLINE"
	ErrTypeArgumentValidationFailed = "ARGUMENT_VALIDATION_FAILED"
	// ErrTypeToolSchemaChanged is reserved: the live broker does not emit it
	// yet. It names the planned schema-drift failure (cataloged schema no
	// longer matching the live tool) that the eval corpus already exercises.
	ErrTypeToolSchemaChanged         = "TOOL_SCHEMA_CHANGED"
	ErrTypeDownstreamCallFailed      = "DOWNSTREAM_CALL_FAILED"
	ErrTypeAuthUnavailable           = "AUTH_UNAVAILABLE"
	ErrTypeSemanticSearchUnavailable = "SEMANTIC_SEARCH_UNAVAILABLE"
	ErrTypeConfigError               = "CONFIG_ERROR"

	// ErrTypeNotImplemented is a skeleton-only marker for operations whose broker
	// behavior is deferred to a later change. It is not part of the durable §9.3
	// error set and is expected to disappear as behavior is implemented.
	ErrTypeNotImplemented = "NOT_IMPLEMENTED"
)

// Error is a structured, repair-oriented failure (SPEC.md §9.3). AgentInstruction
// must state, in grounded terms, whether the agent should retry, choose an
// alternative, ask the user, refresh, or report the failure.
type Error struct {
	Type             string `json:"type"`
	ToolRef          string `json:"toolRef,omitempty"`
	ServerID         string `json:"serverId,omitempty"`
	Retryable        bool   `json:"retryable"`
	Message          string `json:"message"`
	AgentInstruction string `json:"agentInstruction"`
}

// Error implements the error interface so structured failures can flow through
// ordinary Go error returns.
func (e *Error) Error() string { return fmt.Sprintf("%s: %s", e.Type, e.Message) }

// ErrorEnvelope is the {ok:false,error} failure response shape (SPEC.md §9.3).
type ErrorEnvelope struct {
	OK    bool   `json:"ok"`
	Error *Error `json:"error"`
}

// NewErrorEnvelope wraps a structured error in its failure envelope.
func NewErrorEnvelope(e *Error) *ErrorEnvelope { return &ErrorEnvelope{OK: false, Error: e} }

// NotImplemented builds a skeleton-only structured error for an operation that
// is not yet wired to real behavior.
func NotImplemented(operation string) *Error {
	return &Error{
		Type:             ErrTypeNotImplemented,
		Retryable:        false,
		Message:          fmt.Sprintf("%q is not implemented in this build.", operation),
		AgentInstruction: "Do not retry. This capability is pending a future change; report this to the user or choose an available alternative.",
	}
}

// Render produces the human/concise text form of a failure envelope.
func (env *ErrorEnvelope) Render(format string) string {
	e := env.Error
	if format == FormatConcise {
		return fmt.Sprintf("error %s: %s", e.Type, e.Message)
	}
	out := fmt.Sprintf("✗ %s\n  %s", e.Type, e.Message)
	if e.ToolRef != "" {
		out += "\n  toolRef: " + e.ToolRef
	}
	out += fmt.Sprintf("\n  retryable: %t\n  → %s", e.Retryable, e.AgentInstruction)
	return out
}
