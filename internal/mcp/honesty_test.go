package mcp

import (
	"context"
	"errors"
	"strings"
	"testing"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/rokasklive/ozy/internal/broker"
	"github.com/rokasklive/ozy/internal/contract"
)

// connectAdapter connects an in-memory client to the given adapter.
func connectAdapter(t *testing.T, adapter *Adapter) *mcpsdk.ClientSession {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	serverT, clientT := mcpsdk.NewInMemoryTransports()
	go func() { _ = adapter.Server().Run(ctx, serverT) }()

	client := mcpsdk.NewClient(&mcpsdk.Implementation{Name: "test", Version: "0"}, nil)
	cs, err := client.Connect(ctx, clientT, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { _ = cs.Close() })
	return cs
}

// failingBroker errors on FindTool; the other operations are unused.
type failingBroker struct{ err error }

func (f failingBroker) FindTool(context.Context, string) (*contract.FindResult, error) {
	return nil, f.err
}

func (f failingBroker) DescribeTool(context.Context, string) (*contract.DescribeResult, error) {
	return nil, f.err
}

func (f failingBroker) CallTool(context.Context, string, map[string]any) (*contract.CallResult, error) {
	return nil, f.err
}

func (f failingBroker) List(context.Context) (*contract.ListResult, error) {
	return nil, f.err
}

var _ broker.Broker = failingBroker{}

func TestAdapter_InitializeCarriesInstructionsAndBreadcrumb(t *testing.T) {
	breadcrumb := Breadcrumb([]string{"github", "searxng"})
	adapter := New(StaticProvider(failingBroker{}), "test", breadcrumb)
	cs := connectAdapter(t, adapter)

	init := cs.InitializeResult()
	if init == nil {
		t.Fatal("no initialize result")
	}
	if !strings.Contains(init.Instructions, "call findTool first") {
		t.Fatalf("instructions missing when-to-use guidance: %q", init.Instructions)
	}
	for _, srv := range []string{"github", "searxng"} {
		if !strings.Contains(init.Instructions, srv) {
			t.Fatalf("instructions missing breadcrumb server %q: %q", srv, init.Instructions)
		}
	}
}

func TestAdapter_InstructionsOmitBreadcrumbWhenDisabled(t *testing.T) {
	adapter := New(StaticProvider(failingBroker{}), "test", "")
	cs := connectAdapter(t, adapter)

	init := cs.InitializeResult()
	if init == nil || init.Instructions == "" {
		t.Fatal("when-to-use instructions must be present even without a breadcrumb")
	}
	if strings.Contains(init.Instructions, "Available downstream servers") {
		t.Fatalf("disabled breadcrumb leaked into instructions: %q", init.Instructions)
	}
}

func TestAdapter_FindFailureIsLabeledEnvelopeNeverNull(t *testing.T) {
	adapter := New(StaticProvider(failingBroker{err: errors.New("catalog unreadable")}), "test", "")
	cs := connectAdapter(t, adapter)

	res, err := cs.CallTool(context.Background(), &mcpsdk.CallToolParams{
		Name:      "findTool",
		Arguments: map[string]any{"query": "anything"},
	})
	if err != nil {
		t.Fatalf("CallTool transport error: %v", err)
	}
	if !res.IsError {
		t.Fatal("broker failure must set isError")
	}
	raw := textContent(t, res)
	if raw == "null" || raw == "" {
		t.Fatalf("find failure rendered as %q — the null-with-success bug", raw)
	}
	payload := textPayload(t, res)
	if ok, _ := payload["ok"].(bool); ok {
		t.Fatalf("envelope must be ok:false, got %v", payload)
	}
	errObj, _ := payload["error"].(map[string]any)
	if errObj["type"] != contract.ErrTypeConfigError {
		t.Fatalf("synthesized find error type = %v, want CONFIG_ERROR", errObj["type"])
	}
	if errObj["agentInstruction"] == "" {
		t.Fatal("find error must carry an agentInstruction")
	}
}
