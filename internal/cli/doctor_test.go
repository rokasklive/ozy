package cli

import (
	"context"
	"strings"
	"testing"

	"github.com/rokasklive/ozy/internal/contract"
)

func TestEmbeddingCheck_AvailableOnlyWhenReadinessSucceeds(t *testing.T) {
	t.Parallel()
	ready := func(context.Context) SidecarStatus {
		return SidecarStatus{
			Available: true,
			Backend:   "turbovec",
			Model:     "BAAI/bge-small-en-v1.5",
			Dim:       384,
			ToolCount: 5,
		}
	}
	chk := embeddingCheck(context.Background(), ready)
	if chk.Status != contract.CheckOK {
		t.Fatalf("status = %v, want OK when the readiness probe succeeds", chk.Status)
	}
	if !strings.Contains(chk.Detail, "turbovec") || !strings.Contains(chk.Detail, "384") {
		t.Errorf("detail = %q, want it to report backend and dim", chk.Detail)
	}
}

func TestEmbeddingCheck_DegradedSurfacesTheReason(t *testing.T) {
	t.Parallel()
	degraded := func(context.Context) SidecarStatus {
		return SidecarStatus{
			Available: false,
			Reason:    "warm-up: model download incomplete",
		}
	}
	chk := embeddingCheck(context.Background(), degraded)
	if chk.Status != contract.CheckWarn {
		t.Fatalf("status = %v, want WARN when the readiness probe fails", chk.Status)
	}
	if !strings.Contains(chk.Detail, "model download incomplete") {
		t.Errorf("detail = %q, want it to name the specific degraded reason", chk.Detail)
	}
}
