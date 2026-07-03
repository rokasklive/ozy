package mcp

import (
	"strings"
	"testing"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/rokasklive/ozy/internal/contract"
)

func textBlocks(t *testing.T, res *mcpsdk.CallToolResult) []string {
	t.Helper()
	out := make([]string, 0, len(res.Content))
	for _, c := range res.Content {
		tc, ok := c.(*mcpsdk.TextContent)
		if !ok {
			t.Fatalf("unexpected non-text content %T", c)
		}
		out = append(out, tc.Text)
	}
	return out
}

func TestCallResult_NoticesBecomeSeparateTrailerBlock(t *testing.T) {
	t.Parallel()
	payload := `{"rows":[1,2,3]}`
	res := callResult(&contract.CallResult{
		OK:            true,
		ToolRef:       "srv.tool",
		Result:        payload,
		ResultSummary: "summary (truncated)",
		Notices:       []string{"result truncated: showing 3 of 9 items; narrow the call for the rest"},
	})
	blocks := textBlocks(t, res)
	if len(blocks) != 2 {
		t.Fatalf("content blocks = %d, want payload + trailer", len(blocks))
	}
	if blocks[0] != payload {
		t.Fatalf("payload block must stay byte-identical, got %q", blocks[0])
	}
	if !strings.HasPrefix(blocks[1], "[ozy] ") || !strings.Contains(blocks[1], "narrow the call") {
		t.Fatalf("trailer block must carry the marked notice, got %q", blocks[1])
	}
}

func TestCallResult_CachedAgeRendersInBandStamp(t *testing.T) {
	t.Parallel()
	age := int64(42)
	res := callResult(&contract.CallResult{
		OK:               true,
		ToolRef:          "srv.tool",
		Result:           "live-looking payload",
		CachedAgeSeconds: &age,
	})
	blocks := textBlocks(t, res)
	if len(blocks) != 2 {
		t.Fatalf("content blocks = %d, want payload + cache stamp", len(blocks))
	}
	if !strings.Contains(blocks[1], "cached result from 42s ago") {
		t.Fatalf("cache stamp missing from trailer, got %q", blocks[1])
	}
}

func TestCallResult_NoNoticesMeansNoTrailer(t *testing.T) {
	t.Parallel()
	res := callResult(&contract.CallResult{OK: true, ToolRef: "srv.tool", Result: "payload"})
	blocks := textBlocks(t, res)
	if len(blocks) != 1 || blocks[0] != "payload" {
		t.Fatalf("un-noticed result must be a single pristine block, got %v", blocks)
	}
}
