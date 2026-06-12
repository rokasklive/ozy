package contract

import (
	"fmt"
	"strings"
)

// Decision values for findTool (SPEC.md §9.1). They are explicit so an agent can
// branch on intent rather than parsing prose.
const (
	DecisionUse                  = "use"
	DecisionChooseFromCandidates = "choose_from_candidates"
	DecisionKnownButUnavailable  = "known_but_unavailable"
	DecisionNoGoodMatch          = "no_good_match"
	DecisionAmbiguous            = "ambiguous"
	DecisionCatalogEmpty         = "catalog_empty"
)

// CatalogStats is lightweight catalog health surfaced when it affects confidence.
type CatalogStats struct {
	ConfiguredServers int `json:"configuredServers"`
	IndexedTools      int `json:"indexedTools"`
	FreshTools        int `json:"freshTools"`
	StaleTools        int `json:"staleTools"`
}

// SchemaPreview is the field-name preview returned by findTool instead of a full
// schema, to keep response size bounded (SPEC.md §13).
type SchemaPreview struct {
	Required   []string `json:"required,omitempty"`
	Properties []string `json:"properties,omitempty"`
}

// SelectedTool is the best-match summary embedded in a findTool response.
type SelectedTool struct {
	ToolRef       string         `json:"toolRef"`
	Title         string         `json:"title,omitempty"`
	CallableNow   bool           `json:"callableNow"`
	ServerStatus  string         `json:"serverStatus,omitempty"`
	SchemaPreview *SchemaPreview `json:"schemaPreview,omitempty"`
}

// NextAction tells the agent the next concrete Ozy call to make.
type NextAction struct {
	Tool      string         `json:"tool,omitempty"`
	ToolRef   string         `json:"toolRef,omitempty"`
	Arguments map[string]any `json:"arguments,omitempty"`
	Reason    string         `json:"reason,omitempty"`
}

// FollowupTool is an obvious follow-up suggestion (e.g. read after search).
type FollowupTool struct {
	ToolRef string `json:"toolRef"`
	When    string `json:"when,omitempty"`
}

// Alternative is a secondary candidate tool, included only when useful.
type Alternative struct {
	ToolRef string `json:"toolRef"`
	Reason  string `json:"reason,omitempty"`
}

// FindResult is the findTool response (SPEC.md §9.1). It is a decision, not just
// a list, and is always instructional.
type FindResult struct {
	Query               string         `json:"query"`
	Decision            string         `json:"decision"`
	SelectedToolRef     string         `json:"selectedToolRef,omitempty"`
	Confidence          string         `json:"confidence,omitempty"`
	Reason              string         `json:"reason,omitempty"`
	Selected            *SelectedTool  `json:"selected,omitempty"`
	CatalogStats        *CatalogStats  `json:"catalogStats,omitempty"`
	NextAction          *NextAction    `json:"nextAction,omitempty"`
	LikelyFollowupTools []FollowupTool `json:"likelyFollowupTools,omitempty"`
	Alternatives        []Alternative  `json:"alternatives,omitempty"`
	Avoid               []string       `json:"avoid,omitempty"`
	AgentInstruction    string         `json:"agentInstruction,omitempty"`
}

// Render produces the human/concise form of a findTool result.
func (r *FindResult) Render(format string) string {
	if format == FormatConcise {
		if r.SelectedToolRef != "" {
			return fmt.Sprintf("%s -> %s", r.Decision, r.SelectedToolRef)
		}
		return r.Decision
	}
	var b strings.Builder
	fmt.Fprintf(&b, "query: %s\ndecision: %s", r.Query, r.Decision)
	if r.SelectedToolRef != "" {
		fmt.Fprintf(&b, "\nselected: %s (%s)", r.SelectedToolRef, r.Confidence)
	}
	if r.Reason != "" {
		fmt.Fprintf(&b, "\nreason: %s", r.Reason)
	}
	if r.AgentInstruction != "" {
		fmt.Fprintf(&b, "\n→ %s", r.AgentInstruction)
	}
	return b.String()
}

// Example is a worked invocation example for describeTool.
type Example struct {
	Request   string         `json:"request,omitempty"`
	Arguments map[string]any `json:"arguments,omitempty"`
}

// RecommendedCall is the suggested callTool shape for a tool.
type RecommendedCall struct {
	Tool      string         `json:"tool"`
	Arguments map[string]any `json:"arguments"`
}

// RelatedTool links a tool to one commonly used alongside it.
type RelatedTool struct {
	ToolRef      string `json:"toolRef"`
	Relationship string `json:"relationship,omitempty"`
}

// ToolStatus reports live/freshness state for a described tool.
type ToolStatus struct {
	CallableNow      bool   `json:"callableNow"`
	ServerStatus     string `json:"serverStatus,omitempty"`
	CatalogFreshness string `json:"catalogFreshness,omitempty"`
}

// DescribeResult is the describeTool response (SPEC.md §9.2): one tool's exact
// schema, usage guidance, examples, and recommended call shape.
type DescribeResult struct {
	ToolRef         string           `json:"toolRef"`
	Title           string           `json:"title,omitempty"`
	Description     string           `json:"description,omitempty"`
	InputSchema     map[string]any   `json:"inputSchema,omitempty"`
	UsageHints      []string         `json:"usageHints,omitempty"`
	Examples        []Example        `json:"examples,omitempty"`
	RecommendedCall *RecommendedCall `json:"recommendedCall,omitempty"`
	RelatedTools    []RelatedTool    `json:"relatedTools,omitempty"`
	Status          *ToolStatus      `json:"status,omitempty"`
}

// Render produces the human/concise form of a describeTool result.
func (r *DescribeResult) Render(format string) string {
	if format == FormatConcise {
		return r.ToolRef
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s — %s\n%s", r.ToolRef, r.Title, r.Description)
	for _, h := range r.UsageHints {
		fmt.Fprintf(&b, "\n  - %s", h)
	}
	return b.String()
}

// CallNextAction is a recommended follow-up after a successful call.
type CallNextAction struct {
	Recommended bool             `json:"recommended,omitempty"`
	ToolRef     string           `json:"toolRef,omitempty"`
	Reason      string           `json:"reason,omitempty"`
	ExampleCall *RecommendedCall `json:"exampleCall,omitempty"`
}

// CallResult is the callTool success response (SPEC.md §9.3).
type CallResult struct {
	OK            bool             `json:"ok"`
	ToolRef       string           `json:"toolRef"`
	Result        any              `json:"result,omitempty"`
	ResultSummary string           `json:"resultSummary,omitempty"`
	NextActions   []CallNextAction `json:"nextActions,omitempty"`
}

// Render produces the human/concise form of a callTool success result.
func (r *CallResult) Render(format string) string {
	if format == FormatConcise {
		return fmt.Sprintf("ok %s", r.ToolRef)
	}
	return fmt.Sprintf("✓ %s\n  %s", r.ToolRef, r.ResultSummary)
}

// ListedTool is one row of the catalog listing.
type ListedTool struct {
	ToolRef     string `json:"toolRef"`
	Title       string `json:"title,omitempty"`
	ServerID    string `json:"serverId"`
	Freshness   string `json:"freshness,omitempty"`
	CallableNow bool   `json:"callableNow"`
}

// ListResult is the catalog listing returned by `ozy list`.
type ListResult struct {
	Tools            []ListedTool  `json:"tools"`
	CatalogStats     *CatalogStats `json:"catalogStats,omitempty"`
	AgentInstruction string        `json:"agentInstruction,omitempty"`
}

// Render produces the human/concise form of a catalog listing.
func (r *ListResult) Render(format string) string {
	if len(r.Tools) == 0 {
		if format == FormatConcise {
			return "0 tools"
		}
		out := "No indexed tools."
		if r.AgentInstruction != "" {
			out += "\n→ " + r.AgentInstruction
		}
		return out
	}
	var b strings.Builder
	for _, t := range r.Tools {
		fmt.Fprintf(&b, "%s\t%s\t%s\n", t.ToolRef, t.Freshness, t.Title)
	}
	return strings.TrimRight(b.String(), "\n")
}

// DoctorCheck is a single diagnostic result.
type DoctorCheck struct {
	Name   string `json:"name"`
	Status string `json:"status"` // ok | warn | error
	Detail string `json:"detail,omitempty"`
}

// Doctor check status values.
const (
	CheckOK    = "ok"
	CheckWarn  = "warn"
	CheckError = "error"
)

// DoctorResult is the aggregate diagnostics report (SPEC.md §17).
type DoctorResult struct {
	OK               bool          `json:"ok"`
	Checks           []DoctorCheck `json:"checks"`
	AgentInstruction string        `json:"agentInstruction,omitempty"`
}

// Render produces the human/concise form of a doctor report.
func (r *DoctorResult) Render(format string) string {
	if format == FormatConcise {
		return fmt.Sprintf("doctor ok=%t checks=%d", r.OK, len(r.Checks))
	}
	var b strings.Builder
	for _, c := range r.Checks {
		mark := "✓"
		switch c.Status {
		case CheckWarn:
			mark = "!"
		case CheckError:
			mark = "✗"
		}
		fmt.Fprintf(&b, "%s %s: %s\n", mark, c.Name, c.Detail)
	}
	if r.AgentInstruction != "" {
		fmt.Fprintf(&b, "→ %s", r.AgentInstruction)
	}
	return strings.TrimRight(b.String(), "\n")
}

// Message is a simple ok/message result for commands like init and version.
type Message struct {
	OK      bool   `json:"ok"`
	Message string `json:"message"`
}

// Render produces the human/concise form of a simple message.
func (r *Message) Render(string) string { return r.Message }
