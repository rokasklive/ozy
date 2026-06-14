// Package evals embeds the committed eval corpus (the synthetic downstream MCP
// catalog, the labeled gold/scenario sets, and the gate thresholds) so the eval
// harness can load it regardless of the process working directory.
//
// The data files live under evals/data/ and are the source of truth for every
// eval; see evals/README.md for the dataset schema and contribution rules and
// evals/METHODOLOGY.md for the metric mathematics. Reports (BENCHMARKS.md and
// snapshots) are written back to the on-disk evals/ tree, not to this FS.
package evals

import (
	"embed"
	"io/fs"
)

//go:embed data
var dataRoot embed.FS

// Data returns the embedded corpus rooted at the data/ directory, so callers
// reference paths like "catalog/world.json" rather than "data/catalog/world.json".
func Data() fs.FS {
	sub, err := fs.Sub(dataRoot, "data")
	if err != nil {
		// dataRoot always contains data/ (enforced by the embed directive), so a
		// failure here is a build-time impossibility; panic rather than return a
		// broken FS that would surface as confusing load errors downstream.
		panic("evals: embedded data/ directory missing: " + err.Error())
	}
	return sub
}
