// Command ozy is the single Ozy binary: a local agent tool broker that discovers
// downstream MCP tools into a searchable catalog and brokers their invocation
// through a small, stable agent interface (see SPEC.md).
package main

import (
	"os"

	"github.com/rokasklive/ozy/internal/cli"
)

func main() {
	os.Exit(cli.Execute(os.Args[1:], os.Stdout, os.Stderr))
}
