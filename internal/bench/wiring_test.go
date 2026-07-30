package bench

import (
	"encoding/json"
	"os"
	"sort"
	"testing"

	"github.com/tailscale/hujson"
)

// weatherToolsets mirrors the historical-weather-report scenario's declared
// functional toolsets.
var weatherToolsets = []string{"weather", "duckduckgo", "wikipedia", "pdf-toolkit"}

// templateServerKeys parses a checked-in JSONC config template and returns its
// sorted MCP server names.
func templateServerKeys(t *testing.T, path string) []string {
	t.Helper()
	//nolint:gosec // G304: test reads a checked-in template path.
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	std, err := hujson.Standardize(raw)
	if err != nil {
		t.Fatalf("standardize %s: %v", path, err)
	}
	var doc struct {
		MCP map[string]json.RawMessage `json:"mcp"`
	}
	if err := json.Unmarshal(std, &doc); err != nil {
		t.Fatalf("unmarshal %s: %v", path, err)
	}
	keys := make([]string, 0, len(doc.MCP))
	for k := range doc.MCP {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func runnerServerKeys(t *testing.T, mode string) []string {
	t.Helper()
	// The runner's functional wiring for the weather scenario (corpus disabled
	// here so we compare the functional set the vestigial template documents).
	servers, err := scenarioServers(weatherToolsets, false, "{env:OZY_BENCH_FIXTURE_DIR}", "")
	if err != nil {
		t.Fatalf("scenarioServers: %v", err)
	}
	m := mcpServersFor(mode, servers)
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// TestDirectLeanWiringDropsCorpus asserts direct-lean wires only the functional
// toolsets while direct wires the full estate — the sole wiring difference.
func TestDirectLeanWiringDropsCorpus(t *testing.T) {
	t.Parallel()

	servers := []benchServer{
		{Name: "weather", Args: []string{"--toolset", "weather"}},
		{Name: "duckduckgo", Args: []string{"--toolset", "duckduckgo"}},
		{Name: "corpus-a", Args: []string{"--server", "corpus-a"}, Corpus: true},
		{Name: "corpus-b", Args: []string{"--server", "corpus-b"}, Corpus: true},
	}

	if got := len(mcpServersFor("direct", servers)); got != 4 {
		t.Fatalf("direct wired %d servers, want all 4", got)
	}

	lean := mcpServersFor("direct-lean", servers)
	got := make([]string, 0, len(lean))
	for k := range lean {
		got = append(got, k)
	}
	sort.Strings(got)
	if want := []string{"duckduckgo", "weather"}; !equalStrings(got, want) {
		t.Errorf("direct-lean wired %v, want functional-only %v", got, want)
	}
}

// TestModeTemplatesMatchRunner asserts the checked-in reference templates wire
// the same functional server set the runner generates, so they cannot drift.
func TestModeTemplatesMatchRunner(t *testing.T) {
	t.Parallel()

	direct := runnerServerKeys(t, "direct")
	if got := templateServerKeys(t, "../../bench/configs/opencode.direct.jsonc"); !equalStrings(got, direct) {
		t.Errorf("opencode.direct.jsonc server set %v != runner %v", got, direct)
	}
	if got := templateServerKeys(t, "../../bench/configs/ozy.downstream.jsonc"); !equalStrings(got, direct) {
		t.Errorf("ozy.downstream.jsonc server set %v != runner direct %v", got, direct)
	}

	// direct-lean wires the same functional set as direct (the test uses
	// corpus=false, so there is no corpus to drop) — the template documents it.
	lean := runnerServerKeys(t, "direct-lean")
	if !equalStrings(lean, direct) {
		t.Errorf("direct-lean functional wiring %v != direct %v", lean, direct)
	}
	if got := templateServerKeys(t, "../../bench/configs/opencode.direct-lean.jsonc"); !equalStrings(got, lean) {
		t.Errorf("opencode.direct-lean.jsonc server set %v != runner %v", got, lean)
	}

	// The ozy-mode agent config exposes only the broker.
	ozy := runnerServerKeys(t, "ozy")
	if !equalStrings(ozy, []string{"ozy"}) {
		t.Errorf("ozy mode wired %v, want [ozy]", ozy)
	}
	if got := templateServerKeys(t, "../../bench/configs/opencode.ozy.jsonc"); !equalStrings(got, []string{"ozy"}) {
		t.Errorf("opencode.ozy.jsonc server set %v, want [ozy]", got)
	}
}
