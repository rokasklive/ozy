package bench

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// inMemoryConnect captures from an in-process fixture toolset over an in-memory
// transport, so the capture→bake→replay loop is exercised with no subprocess or
// network (the real capture path uses a CommandTransport to a live MCP).
func inMemoryConnect(t *testing.T, fixtureDir string) captureConnect {
	t.Helper()
	return func(ctx context.Context, s CaptureServer) (*mcpsdk.ClientSession, func(), error) {
		srv, err := newMCPServer(s.Name, fixtureDir)
		if err != nil {
			return nil, nil, err
		}
		serverT, clientT := mcpsdk.NewInMemoryTransports()
		go func() { _ = srv.Run(ctx, serverT) }()
		client := mcpsdk.NewClient(&mcpsdk.Implementation{Name: "capture-test", Version: "0"}, nil)
		cs, err := client.Connect(ctx, clientT, nil)
		if err != nil {
			return nil, nil, err
		}
		return cs, func() { _ = cs.Close() }, nil
	}
}

// TestCaptureFixtureByteStable proves the capture step bakes a real MCP's schemas
// and responses into the fixture, that re-capturing identical responses is
// byte-stable, and that runtime replay reads only the baked file deterministically
// with no network (D4/5.1,5.2).
func TestCaptureFixtureByteStable(t *testing.T) {
	t.Parallel()

	// A source fixture the (stand-in "real") duckduckgo server ranks over.
	fixtureDir := t.TempDir()
	corpus := []map[string]any{
		{"title": "Vilnius", "url": "https://x/v", "snippet": "Vilnius is the capital of Lithuania on the Neris."},
		{"title": "Other", "url": "https://x/o", "snippet": "unrelated"},
	}
	data, _ := json.Marshal(corpus)
	if err := os.WriteFile(filepath.Join(fixtureDir, "search.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	spec := &CaptureSpec{Servers: []CaptureServer{{
		Name:   "duckduckgo",
		BakeTo: "search.json",
		Shape:  "search",
		Calls:  []CaptureCall{{Tool: "search", Arguments: map[string]any{"query": "Vilnius Neris"}}},
	}}}

	outA, outB := t.TempDir(), t.TempDir()
	if err := CaptureFixture(context.Background(), spec, outA, inMemoryConnect(t, fixtureDir)); err != nil {
		t.Fatalf("capture A: %v", err)
	}
	if err := CaptureFixture(context.Background(), spec, outB, inMemoryConnect(t, fixtureDir)); err != nil {
		t.Fatalf("capture B: %v", err)
	}

	bakedA := mustRead(t, filepath.Join(outA, "search.json"))
	bakedB := mustRead(t, filepath.Join(outB, "search.json"))
	if !bytes.Equal(bakedA, bakedB) {
		t.Errorf("capture not byte-stable:\nA=%s\nB=%s", bakedA, bakedB)
	}
	if !strings.Contains(string(bakedA), "Neris") {
		t.Errorf("baked fixture missing captured fact:\n%s", bakedA)
	}
	if _, err := os.Stat(filepath.Join(outA, "duckduckgo.schemas.json")); err != nil {
		t.Errorf("tool schemas not captured: %v", err)
	}

	// Replay reads only the baked file — deterministic, no network.
	got1, err := searchFixtureResults(outA, "Vilnius Neris", 0)
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	got2, _ := searchFixtureResults(outA, "Vilnius Neris", 0)
	j1, _ := json.Marshal(got1)
	j2, _ := json.Marshal(got2)
	if !bytes.Equal(j1, j2) {
		t.Error("replay over baked capture is not deterministic")
	}
}

// TestLoadCaptureSpecMissingIsOptIn asserts an absent capture.json is not an
// error — capture is opt-in.
func TestLoadCaptureSpecMissingIsOptIn(t *testing.T) {
	t.Parallel()

	spec, ok, err := LoadCaptureSpec(filepath.Join(t.TempDir(), "capture.json"))
	if err != nil {
		t.Fatalf("missing capture spec should not error: %v", err)
	}
	if ok || spec != nil {
		t.Errorf("missing capture spec = (%v, ok=%v), want (nil, false)", spec, ok)
	}
}
