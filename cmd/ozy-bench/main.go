// Command ozy-bench is the scenario benchmark harness for Ozy. It runs a frozen
// incident scenario in direct and ozy modes and emits a side-by-side comparison
// of tool-surface, token-economy, and task-success metrics.
package main

import (
	"os"

	"github.com/rokasklive/ozy/internal/bench"
)

func main() {
	os.Exit(bench.Execute(os.Args[1:], os.Stdout, os.Stderr))
}
