package broker

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/rokasklive/ozy/internal/config"
	"github.com/rokasklive/ozy/internal/contract"
	"github.com/rokasklive/ozy/internal/downstream"
)

// slowSession delays CallTool by delay, honoring context cancellation the way
// a real transport would.
type slowSession struct {
	delay time.Duration
	res   *mcpsdk.CallToolResult
}

func (s *slowSession) ListTools(context.Context, *mcpsdk.ListToolsParams) (*mcpsdk.ListToolsResult, error) {
	return &mcpsdk.ListToolsResult{}, nil
}

func (s *slowSession) CallTool(ctx context.Context, _ *mcpsdk.CallToolParams) (*mcpsdk.CallToolResult, error) {
	select {
	case <-time.After(s.delay):
		return s.res, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (s *slowSession) Close() error { return nil }

func timeoutCfg(discoveryMs, callMs int) *config.Config {
	return &config.Config{MCP: map[string]config.ServerConfig{
		"srv": {Type: "local", Command: []string{"fake"}, Enabled: true, Timeout: discoveryMs, CallTimeout: callMs},
	}}
}

func TestCallTool_SlowCallSurvivesDiscoveryTimeout(t *testing.T) {
	t.Parallel()
	session := &slowSession{
		delay: 200 * time.Millisecond,
		res:   &mcpsdk.CallToolResult{Content: []mcpsdk.Content{&mcpsdk.TextContent{Text: "ok"}}},
	}
	cfg := timeoutCfg(50, 2000) // discovery would kill this call; callTimeout must not
	b := newCallBroker(t, cfg, &callConnector{connectResult: downstream.Result{ServerID: "srv", Session: session}})

	res, err := b.CallTool(context.Background(), "srv.tool", nil)
	if err != nil {
		t.Fatalf("CallTool: expected success past the discovery timeout, got %v", err)
	}
	if !res.OK || res.Result != "ok" {
		t.Fatalf("CallTool: unexpected result %+v", res)
	}
}

func TestCallTool_OwnDeadlineIsNonRetryableAndNamed(t *testing.T) {
	t.Parallel()
	session := &slowSession{delay: 5 * time.Second}
	cfg := timeoutCfg(5000, 50)
	b := newCallBroker(t, cfg, &callConnector{connectResult: downstream.Result{ServerID: "srv", Session: session}})

	_, err := b.CallTool(context.Background(), "srv.tool", nil)
	var ce *contract.Error
	if !errors.As(err, &ce) {
		t.Fatalf("CallTool: expected *contract.Error, got %v", err)
	}
	if ce.Retryable {
		t.Fatalf("own callTimeout deadline must be non-retryable, got %+v", ce)
	}
	if !strings.Contains(ce.Message, "callTimeout") {
		t.Fatalf("deadline error must name callTimeout, got %q", ce.Message)
	}
	if !strings.Contains(ce.AgentInstruction, "Do not retry the same call unchanged") {
		t.Fatalf("deadline instruction must forbid identical retry, got %q", ce.AgentInstruction)
	}
}

func TestServerConfig_InvocationTimeoutDefaults(t *testing.T) {
	t.Parallel()
	var s config.ServerConfig
	if got := s.InvocationTimeout(); got != 60*time.Second {
		t.Fatalf("default InvocationTimeout = %v, want 60s", got)
	}
	s.CallTimeout = 180000
	if got := s.InvocationTimeout(); got != 180*time.Second {
		t.Fatalf("explicit InvocationTimeout = %v, want 180s", got)
	}
}
