package catalog

import (
	"context"
	"path/filepath"
	"testing"
)

func TestMemory_DeleteToolsIgnoresUnknownRefs(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	m := NewMemory()
	if err := m.PutTool(ctx, Tool{ToolRef: "a.one"}); err != nil {
		t.Fatal(err)
	}
	if err := m.PutTool(ctx, Tool{ToolRef: "a.two"}); err != nil {
		t.Fatal(err)
	}
	if err := m.DeleteTools(ctx, []string{"a.one", "a.never-existed"}); err != nil {
		t.Fatalf("DeleteTools: %v", err)
	}
	if _, ok, _ := m.GetTool(ctx, "a.one"); ok {
		t.Fatal("a.one should be deleted")
	}
	if _, ok, _ := m.GetTool(ctx, "a.two"); !ok {
		t.Fatal("a.two should survive")
	}
}

func TestFile_DeleteToolsSurvivesRestart(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "catalog.json")

	store, err := NewFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.PutTool(ctx, Tool{ToolRef: "a.one"}); err != nil {
		t.Fatal(err)
	}
	if err := store.PutTool(ctx, Tool{ToolRef: "a.two"}); err != nil {
		t.Fatal(err)
	}
	if err := store.DeleteTools(ctx, []string{"a.one"}); err != nil {
		t.Fatalf("DeleteTools: %v", err)
	}

	reopened, err := NewFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := reopened.GetTool(ctx, "a.one"); ok {
		t.Fatal("a.one should stay deleted after restart")
	}
	tools, err := reopened.Tools(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(tools) != 1 || tools[0].ToolRef != "a.two" {
		t.Fatalf("unexpected tools after restart: %+v", tools)
	}
}
