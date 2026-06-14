package cli

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"time"

	"github.com/rokasklive/ozy/internal/catalog"
	"github.com/rokasklive/ozy/internal/eval"
	"github.com/rokasklive/ozy/internal/search"
	"github.com/rokasklive/ozy/internal/sidecar"
)

// defaultEvalModel is the embedding model the semantic eval leg uses unless
// overridden. It matches the sidecar's documented default (SPEC.md §10.4).
const defaultEvalModel = "BAAI/bge-small-en-v1.5"

// sidecarSemanticBuilder returns an eval.SemanticBuilder that provisions the real
// embedding sidecar, embeds the eval corpus, and hands the harness a live
// semantic provider. It is the bridge that keeps internal/eval free of any
// sidecar dependency: the CLI supplies the real builder only on the --semantic
// path. Any failure returns an error, which eval.Run treats as "skip the
// semantic leg cleanly" (recorded, never fatal) per SPEC.md §10.1.
func sidecarSemanticBuilder(model string) eval.SemanticBuilder {
	if model == "" {
		model = defaultEvalModel
	}
	return func(ctx context.Context, _ catalog.Store, corpus *eval.Corpus) (search.Semantic, func(), error) {
		pctx, cancel := context.WithTimeout(ctx, sidecar.DefaultProvisionTimeout)
		defer cancel()
		resolved, err := sidecar.Provision(pctx, sidecar.ProvisionOptions{Backend: "turbovec", Model: model})
		if err != nil {
			return nil, nil, fmt.Errorf("provision sidecar: %w", err)
		}

		// Isolated, throwaway store so the embedded vectors are exactly this
		// corpus run — no stale state leaks between eval runs.
		dataDir, err := os.MkdirTemp("", "ozy-eval-sidecar-")
		if err != nil {
			return nil, nil, err
		}
		client, err := sidecar.NewClient(sidecar.Options{
			DataDir: dataDir,
			Backend: "turbovec",
			Model:   model,
			ProcessOptions: sidecar.ProcessOptions{
				PythonPath: resolved.PythonPath,
				SourceDir:  resolved.SourceDir,
				DataDir:    dataDir,
				Backend:    "turbovec",
				Model:      model,
			},
		})
		if err != nil {
			_ = os.RemoveAll(dataDir)
			return nil, nil, fmt.Errorf("start sidecar: %w", err)
		}
		cleanup := func() {
			_ = client.Close()
			_ = os.RemoveAll(dataDir)
		}

		hctx, hcancel := context.WithTimeout(ctx, 90*time.Second)
		defer hcancel()
		if hr := client.Health(hctx); !hr.OK {
			cleanup()
			return nil, nil, fmt.Errorf("sidecar health check failed: %w", hr.Err)
		}

		if err := embedCorpus(ctx, client, corpus); err != nil {
			cleanup()
			return nil, nil, err
		}
		return sidecar.NewSemanticAdapter(client), cleanup, nil
	}
}

// embedCorpus pushes every corpus tool to the sidecar to embed and store,
// mirroring production embed-on-index.
func embedCorpus(ctx context.Context, client *sidecar.Client, corpus *eval.Corpus) error {
	items := make([]sidecar.UpsertItem, 0, len(corpus.Catalog.Tools))
	for _, t := range corpus.Catalog.Tools {
		text := t.IndexedText()
		sum := sha256.Sum256([]byte(text))
		items = append(items, sidecar.UpsertItem{
			ToolRef:     t.ToolRef,
			Text:        text,
			ContentHash: hex.EncodeToString(sum[:]),
			ServerID:    t.ServerID,
		})
	}
	uctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	if _, err := client.Upsert(uctx, items); err != nil {
		return fmt.Errorf("embed corpus: %w", err)
	}
	return nil
}
