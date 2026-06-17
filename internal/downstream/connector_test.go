package downstream

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/rokasklive/ozy/internal/config"
	"github.com/rokasklive/ozy/internal/contract"
)

type fakeTransport struct {
	err error
}

func (f fakeTransport) Connect(context.Context) (mcpsdk.Connection, error) {
	return nil, f.err
}

type blockingTransport struct{}

func (blockingTransport) Connect(ctx context.Context) (mcpsdk.Connection, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

func testServerWithTool(name string) *mcpsdk.Server {
	server := mcpsdk.NewServer(&mcpsdk.Implementation{Name: "test-server", Version: "0"}, nil)
	mcpsdk.AddTool(server, &mcpsdk.Tool{
		Name:        name,
		Title:       "Test Tool",
		Description: "A test tool",
	}, func(context.Context, *mcpsdk.CallToolRequest, map[string]any) (*mcpsdk.CallToolResult, any, error) {
		return &mcpsdk.CallToolResult{}, nil, nil
	})
	return server
}

func inMemoryFactory(t *testing.T, toolName string) TransportFactory {
	t.Helper()
	return func(string, config.ServerConfig) (mcpsdk.Transport, error) {
		serverT, clientT := mcpsdk.NewInMemoryTransports()
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)
		go func() { _ = testServerWithTool(toolName).Run(ctx, serverT) }()
		return clientT, nil
	}
}

func TestConnector_ConnectsInMemoryServerAndListsTools(t *testing.T) {
	t.Parallel()
	connector := New(WithTransportFactory(inMemoryFactory(t, "search")))

	results := connector.ConnectAll(context.Background(), &config.Config{
		MCP: map[string]config.ServerConfig{
			"atlassian": {Type: "remote", URL: "memory", Enabled: true},
		},
	})

	if len(results) != 1 {
		t.Fatalf("ConnectAll() results len = %d, want 1", len(results))
	}
	result := results[0]
	if result.Error != nil {
		t.Fatalf("ConnectAll() error = %+v", result.Error)
	}
	t.Cleanup(func() { _ = result.Session.Close() })
	tools, err := result.Session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListTools() error = %v", err)
	}
	if len(tools.Tools) != 1 || tools.Tools[0].Name != "search" {
		t.Fatalf("ListTools() = %+v, want search tool", tools.Tools)
	}
}

func TestConnector_OneUnreachableServerDoesNotAbortOthers(t *testing.T) {
	t.Parallel()
	connector := New(WithTransportFactory(func(id string, cfg config.ServerConfig) (mcpsdk.Transport, error) {
		if id == "bad" {
			return fakeTransport{err: errors.New("dial failed")}, nil
		}
		return inMemoryFactory(t, "search")(id, cfg)
	}))

	results := connector.ConnectAll(context.Background(), &config.Config{
		MCP: map[string]config.ServerConfig{
			"bad":  {Type: "remote", URL: "bad", Enabled: true},
			"good": {Type: "remote", URL: "memory", Enabled: true},
		},
	})

	byID := resultsByID(results)
	if byID["good"].Error != nil || byID["good"].Session == nil {
		t.Fatalf("good result = %+v, want connected session", byID["good"])
	}
	t.Cleanup(func() { _ = byID["good"].Session.Close() })
	if byID["bad"].Error == nil || byID["bad"].Error.Type != contract.ErrTypeDownstreamServerOffline {
		t.Fatalf("bad result error = %+v, want DOWNSTREAM_SERVER_OFFLINE", byID["bad"].Error)
	}
}

func TestConnector_DisabledServersAreSkipped(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	connector := New(WithTransportFactory(func(id string, cfg config.ServerConfig) (mcpsdk.Transport, error) {
		calls.Add(1)
		return inMemoryFactory(t, "search")(id, cfg)
	}))

	results := connector.ConnectAll(context.Background(), &config.Config{
		MCP: map[string]config.ServerConfig{
			"disabled": {Type: "remote", URL: "memory", Enabled: false},
			"enabled":  {Type: "remote", URL: "memory", Enabled: true},
		},
	})

	byID := resultsByID(results)
	if !byID["disabled"].Skipped || byID["disabled"].Session != nil {
		t.Fatalf("disabled result = %+v, want skipped without session", byID["disabled"])
	}
	if calls.Load() != 1 {
		t.Fatalf("transport factory calls = %d, want 1", calls.Load())
	}
	if byID["enabled"].Session != nil {
		t.Cleanup(func() { _ = byID["enabled"].Session.Close() })
	}
}

func TestConnector_ConnectionErrorExcludesSecretValues(t *testing.T) {
	t.Parallel()
	const secret = "Bearer supersecretvalue"
	connector := New(WithTransportFactory(func(string, config.ServerConfig) (mcpsdk.Transport, error) {
		return fakeTransport{err: errors.New("connect failed with header " + secret)}, nil
	}))

	results := connector.ConnectAll(context.Background(), &config.Config{
		MCP: map[string]config.ServerConfig{
			"atlassian": {
				Type:    "remote",
				URL:     "memory",
				Headers: map[string]string{"Authorization": secret},
				Enabled: true,
			},
		},
	})

	got := results[0].Error
	if got == nil {
		t.Fatal("ConnectAll() error = nil, want structured error")
		return
	}
	if strings.Contains(got.Message, "supersecretvalue") || strings.Contains(got.Message, secret) {
		t.Fatalf("connection error leaked secret: %+v", got)
	}
	if got.ServerID != "atlassian" {
		t.Fatalf("ServerID = %q, want atlassian", got.ServerID)
	}
}

func TestConnector_LocalTransportUsesCommandAndEnvironment(t *testing.T) {
	t.Parallel()
	connector := New()

	transport, err := connector.transportForServer("filesystem", config.ServerConfig{
		Type:        "local",
		Command:     []string{"filesystem-mcp", "--root", "."},
		CWD:         "/tmp/workspace",
		Environment: map[string]string{"OZY_ROOT": "/tmp/ozy"},
		Enabled:     true,
	})
	if err != nil {
		t.Fatalf("transportForServer() error = %v", err)
	}
	commandTransport, ok := transport.(*mcpsdk.CommandTransport)
	if !ok {
		t.Fatalf("transport = %T, want *mcp.CommandTransport", transport)
	}
	cmd := commandTransport.Command
	if cmd.Path != "filesystem-mcp" || strings.Join(cmd.Args, " ") != "filesystem-mcp --root ." {
		t.Fatalf("command = %v %v, want filesystem-mcp --root .", cmd.Path, cmd.Args)
	}
	if cmd.Dir != "/tmp/workspace" {
		t.Fatalf("command dir = %q, want /tmp/workspace", cmd.Dir)
	}
	if !hasEnv(cmd, "OZY_ROOT=/tmp/ozy") {
		t.Fatalf("command env missing OZY_ROOT: %v", cmd.Env)
	}
}

func TestConnector_PerServerTimeoutCancelsConnect(t *testing.T) {
	t.Parallel()
	connector := New(WithTransportFactory(func(string, config.ServerConfig) (mcpsdk.Transport, error) {
		return blockingTransport{}, nil
	}))

	results := connector.ConnectAll(context.Background(), &config.Config{
		MCP: map[string]config.ServerConfig{
			"slow": {
				Type:    "remote",
				URL:     "memory",
				Enabled: true,
				Timeout: 1,
			},
		},
	})

	if len(results) != 1 {
		t.Fatalf("ConnectAll() results len = %d, want 1", len(results))
	}
	got := results[0].Error
	if got == nil || got.Type != contract.ErrTypeDownstreamServerOffline {
		t.Fatalf("ConnectAll() error = %+v, want timeout as DOWNSTREAM_SERVER_OFFLINE", got)
	}
	if !strings.Contains(got.Message, context.DeadlineExceeded.Error()) {
		t.Fatalf("timeout message = %q, want deadline exceeded", got.Message)
	}
}

func TestConnector_OAuthConnectionFailureIsAuthUnavailableAndRedacted(t *testing.T) {
	t.Parallel()
	const secret = "supersecretvalue"
	err := connectionError("sentry", config.ServerConfig{
		Type:    "remote",
		URL:     "https://mcp.example.com",
		Headers: map[string]string{"Authorization": "Bearer " + secret},
		OAuth:   json.RawMessage(`{"clientId":"{env:CLIENT_ID}"}`),
	}, errors.New("401 unauthorized with Bearer "+secret))

	if err.Type != contract.ErrTypeAuthUnavailable {
		t.Fatalf("connectionError() type = %q, want AUTH_UNAVAILABLE", err.Type)
	}
	if strings.Contains(err.Message, secret) {
		t.Fatalf("auth error leaked secret: %+v", err)
	}
	if err.Retryable {
		t.Fatal("auth unavailable should not be blindly retryable")
	}
}

func TestConnector_RemoteTransportInjectsHeaders(t *testing.T) {
	t.Parallel()
	const secret = "Bearer test-token"
	var sawHeader atomic.Bool
	server := testServerWithTool("search")
	handler := mcpsdk.NewStreamableHTTPHandler(func(*http.Request) *mcpsdk.Server {
		return server
	}, nil)
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == secret {
			sawHeader.Store(true)
		}
		handler.ServeHTTP(w, r)
	}))
	t.Cleanup(httpServer.Close)

	connector := New()
	results := connector.ConnectAll(context.Background(), &config.Config{
		MCP: map[string]config.ServerConfig{
			"remote": {
				Type:    "remote",
				URL:     httpServer.URL,
				Headers: map[string]string{"Authorization": secret},
				Enabled: true,
			},
		},
	})

	if results[0].Error != nil {
		t.Fatalf("ConnectAll() error = %+v", results[0].Error)
	}
	t.Cleanup(func() { _ = results[0].Session.Close() })
	if !sawHeader.Load() {
		t.Fatal("remote server did not receive Authorization header")
	}
}

func resultsByID(results []Result) map[string]Result {
	out := make(map[string]Result, len(results))
	for _, r := range results {
		out[r.ServerID] = r
	}
	return out
}

func TestServerConfigDiscoveryTimeoutDuration(t *testing.T) {
	t.Parallel()
	if got := (config.ServerConfig{}).DiscoveryTimeout(); got != 5*time.Second {
		t.Fatalf("zero timeout duration = %v, want 5s default", got)
	}
	if got := (config.ServerConfig{Timeout: 180000}).DiscoveryTimeout(); got != 180*time.Second {
		t.Fatalf("configured timeout duration = %v, want 180s", got)
	}
}

func hasEnv(cmd *exec.Cmd, entry string) bool {
	for _, got := range cmd.Env {
		if got == entry {
			return true
		}
	}
	return false
}
