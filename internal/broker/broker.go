// Package broker defines the single in-process seam through which every adapter
// performs Ozy's findTool, describeTool, and callTool operations.
//
// Both the CLI and the MCP adapter depend only on the Broker interface, so the
// two paths cannot drift (SPEC.md §4.9, §14.1). This package also ships the
// skeleton implementation whose responses already conform to the §9 contracts
// while real search and invocation are deferred to later changes.
package broker

import (
	"context"

	"github.com/rokasklive/ozy/internal/contract"
)

// Broker is the capability-brokerage seam. FindTool encodes its outcome in the
// returned FindResult (including no-match and empty-catalog decisions), so it
// returns a nil error for ordinary decisions. DescribeTool and CallTool return a
// *contract.Error on failure so callers can render the §9.3 failure envelope.
type Broker interface {
	// FindTool finds the best known tool for a capability query (SPEC.md §9.1).
	FindTool(ctx context.Context, query string) (*contract.FindResult, error)
	// DescribeTool returns exact schema and usage guidance for one toolRef
	// (SPEC.md §9.2).
	DescribeTool(ctx context.Context, toolRef string) (*contract.DescribeResult, error)
	// CallTool invokes a downstream tool through Ozy (SPEC.md §9.3).
	CallTool(ctx context.Context, toolRef string, args map[string]any) (*contract.CallResult, error)
	// List returns the current catalog listing for `ozy list`.
	List(ctx context.Context) (*contract.ListResult, error)
}
