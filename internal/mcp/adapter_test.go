package mcp

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/rokasklive/ozy/internal/broker"
	"github.com/rokasklive/ozy/internal/catalog"
	"github.com/rokasklive/ozy/internal/config"
	"github.com/rokasklive/ozy/internal/downstream"
)

func connect(t *testing.T) *mcpsdk.ClientSession {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	serverT, clientT := mcpsdk.NewInMemoryTransports()
	adapter := New(broker.NewSkeleton(catalog.NewMemory()), "test")

	go func() { _ = adapter.Server().Run(ctx, serverT) }()

	client := mcpsdk.NewClient(&mcpsdk.Implementation{Name: "test", Version: "0"}, nil)
	cs, err := client.Connect(ctx, clientT, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { _ = cs.Close() })
	return cs
}

func textPayload(t *testing.T, res *mcpsdk.CallToolResult) map[string]any {
	t.Helper()
	if len(res.Content) == 0 {
		t.Fatal("tool result had no content")
	}
	tc, ok := res.Content[0].(*mcpsdk.TextContent)
	if !ok {
		t.Fatalf("content[0] is %T, want *TextContent", res.Content[0])
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(tc.Text), &payload); err != nil {
		t.Fatalf("content is not valid JSON: %v", err)
	}
	return payload
}

func TestAdapter_AdvertisesExactlyThreeTools(t *testing.T) {
	cs := connect(t)
	list, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	got := make(map[string]bool)
	for _, tool := range list.Tools {
		got[tool.Name] = true
	}
	want := []string{"findTool", "describeTool", "callTool"}
	if len(list.Tools) != len(want) {
		t.Errorf("advertised %d tools, want %d: %v", len(list.Tools), len(want), got)
	}
	for _, name := range want {
		if !got[name] {
			t.Errorf("missing tool %q", name)
		}
	}
}

func TestAdapter_FindToolReturnsCatalogEmptyShape(t *testing.T) {
	cs := connect(t)
	res, err := cs.CallTool(context.Background(), &mcpsdk.CallToolParams{
		Name:      "findTool",
		Arguments: map[string]any{"query": "search confluence"},
	})
	if err != nil {
		t.Fatalf("CallTool(findTool): %v", err)
	}
	payload := textPayload(t, res)
	if payload["decision"] != "catalog_empty" {
		t.Errorf("decision = %v, want catalog_empty", payload["decision"])
	}
	if payload["agentInstruction"] == "" || payload["agentInstruction"] == nil {
		t.Error("findTool response must include an agentInstruction")
	}
}

type fakeLiveConnector struct {
	results []downstream.Result
}

func (f fakeLiveConnector) ConnectAll(context.Context, *config.Config) []downstream.Result {
	return f.results
}

func (fakeLiveConnector) Connect(_ context.Context, _ string, _ config.ServerConfig) downstream.Result {
	return downstream.Result{ServerID: "unused"}
}

type fakeLiveSession struct {
	tools []*mcpsdk.Tool
	err   error
}

func (f fakeLiveSession) ListTools(context.Context, *mcpsdk.ListToolsParams) (*mcpsdk.ListToolsResult, error) {
	return &mcpsdk.ListToolsResult{Tools: f.tools}, f.err
}

func (fakeLiveSession) CallTool(context.Context, *mcpsdk.CallToolParams) (*mcpsdk.CallToolResult, error) {
	return &mcpsdk.CallToolResult{}, nil
}

func (fakeLiveSession) Close() error { return nil }

func connectLive(t *testing.T) *mcpsdk.ClientSession {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	liveConnector := fakeLiveConnector{results: []downstream.Result{
		{
			ServerID: "atlassian",
			Session: fakeLiveSession{tools: []*mcpsdk.Tool{
				{
					Name:        "confluence_search",
					Title:       "Confluence Search",
					Description: "Search Confluence wiki",
					InputSchema: map[string]any{
						"type":       "object",
						"properties": map[string]any{"query": map[string]any{"type": "string"}},
					},
				},
			}},
		},
	}}

	serverT, clientT := mcpsdk.NewInMemoryTransports()
	adapter := New(broker.NewLive(catalog.NewMemory(), &config.Config{}, liveConnector), "test")

	go func() { _ = adapter.Server().Run(ctx, serverT) }()

	client := mcpsdk.NewClient(&mcpsdk.Implementation{Name: "test", Version: "0"}, nil)
	cs, err := client.Connect(ctx, clientT, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { _ = cs.Close() })
	return cs
}

func TestAdapter_FindToolReturnsLiveDiscoveredTools(t *testing.T) {
	cs := connectLive(t)
	res, err := cs.CallTool(context.Background(), &mcpsdk.CallToolParams{
		Name:      "findTool",
		Arguments: map[string]any{"query": "search"},
	})
	if err != nil {
		t.Fatalf("CallTool(findTool): %v", err)
	}
	payload := textPayload(t, res)
	if payload["decision"] != "choose_from_candidates" {
		t.Errorf("decision = %v, want choose_from_candidates", payload["decision"])
	}
	candidates, ok := payload["candidates"].([]any)
	if !ok {
		t.Fatalf("candidates = %v (%T), want array", payload["candidates"], payload["candidates"])
	}
	if len(candidates) != 1 {
		t.Fatalf("candidates len = %d, want 1", len(candidates))
	}
	c, ok := candidates[0].(map[string]any)
	if !ok {
		t.Fatalf("candidate[0] = %T, want object", candidates[0])
	}
	if c["toolRef"] != "atlassian.confluence_search" {
		t.Errorf("toolRef = %v, want atlassian.confluence_search", c["toolRef"])
	}
	if c["serverId"] != "atlassian" {
		t.Errorf("serverId = %v, want atlassian", c["serverId"])
	}
	if c["name"] != "confluence_search" {
		t.Errorf("name = %v, want confluence_search", c["name"])
	}
	if payload["agentInstruction"] == "" || payload["agentInstruction"] == nil {
		t.Error("findTool response must include an agentInstruction")
	}
}

func TestAdapter_FindToolReportsEmptyLiveDiscovery(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	emptyConnector := fakeLiveConnector{results: []downstream.Result{
		{
			ServerID: "atlassian",
			Session:  fakeLiveSession{}, // zero tools
		},
	}}

	serverT, clientT := mcpsdk.NewInMemoryTransports()
	adapter := New(broker.NewLive(catalog.NewMemory(), &config.Config{}, emptyConnector), "test")

	go func() { _ = adapter.Server().Run(ctx, serverT) }()

	client := mcpsdk.NewClient(&mcpsdk.Implementation{Name: "test", Version: "0"}, nil)
	cs, err := client.Connect(ctx, clientT, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { _ = cs.Close() })

	res, err := cs.CallTool(context.Background(), &mcpsdk.CallToolParams{
		Name:      "findTool",
		Arguments: map[string]any{"query": "search"},
	})
	if err != nil {
		t.Fatalf("CallTool(findTool): %v", err)
	}
	payload := textPayload(t, res)
	if payload["decision"] != "no_good_match" {
		t.Errorf("decision = %v, want no_good_match", payload["decision"])
	}
	if payload["agentInstruction"] == "" || payload["agentInstruction"] == nil {
		t.Error("zero-tools response must include an agentInstruction")
	}
}

func TestAdapter_IntegrationWithFixtureDownstreamServer(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	// Create a fixture downstream MCP server with real tools.
	dsServer := mcpsdk.NewServer(&mcpsdk.Implementation{Name: "fixture-downstream", Version: "0"}, nil)
	mcpsdk.AddTool(dsServer, &mcpsdk.Tool{
		Name:        "fixture_search",
		Title:       "Fixture Search",
		Description: "Search fixture data",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{"query": map[string]any{"type": "string"}},
		},
	}, func(context.Context, *mcpsdk.CallToolRequest, map[string]any) (*mcpsdk.CallToolResult, any, error) {
		return &mcpsdk.CallToolResult{}, nil, nil
	})
	mcpsdk.AddTool(dsServer, &mcpsdk.Tool{
		Name:        "fixture_read",
		Title:       "Fixture Read",
		Description: "Read fixture data",
	}, func(context.Context, *mcpsdk.CallToolRequest, map[string]any) (*mcpsdk.CallToolResult, any, error) {
		return &mcpsdk.CallToolResult{}, nil, nil
	})
	dsServerT, dsClientT := mcpsdk.NewInMemoryTransports()
	go func() { _ = dsServer.Run(ctx, dsServerT) }()

	// Build a Connector that routes to the fixture server.
	connector := downstream.New(downstream.WithTransportFactory(
		func(_ string, _ config.ServerConfig) (mcpsdk.Transport, error) {
			return dsClientT, nil
		},
	))

	// Create the live broker backed by the fixture connector.
	liveBroker := broker.NewLive(catalog.NewMemory(), &config.Config{
		MCP: map[string]config.ServerConfig{
			"fixture": {Type: "local", Enabled: true},
		},
	}, connector)

	// Wire the MCP adapter.
	ozyServerT, ozyClientT := mcpsdk.NewInMemoryTransports()
	adapter := New(liveBroker, "test")
	go func() { _ = adapter.Server().Run(ctx, ozyServerT) }()

	client := mcpsdk.NewClient(&mcpsdk.Implementation{Name: "test", Version: "0"}, nil)
	cs, err := client.Connect(ctx, ozyClientT, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { _ = cs.Close() })

	// Verify Ozy advertises exactly its three tools.
	list, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	got := make(map[string]bool)
	for _, tool := range list.Tools {
		got[tool.Name] = true
	}
	for _, name := range []string{"findTool", "describeTool", "callTool"} {
		if !got[name] {
			t.Errorf("missing Ozy tool %q", name)
		}
	}

	// Call findTool and verify live-discovered fixture tools are returned.
	res, err := cs.CallTool(context.Background(), &mcpsdk.CallToolParams{
		Name:      "findTool",
		Arguments: map[string]any{"query": "search"},
	})
	if err != nil {
		t.Fatalf("CallTool(findTool): %v", err)
	}
	payload := textPayload(t, res)
	if payload["decision"] != "choose_from_candidates" {
		t.Fatalf("decision = %v, want choose_from_candidates", payload["decision"])
	}
	candidates, ok := payload["candidates"].([]any)
	if !ok || len(candidates) != 2 {
		t.Fatalf("candidates = %v (len=%d), want 2 tools", payload["candidates"], len(candidates))
	}
	candidateNames := make(map[string]bool)
	for _, c := range candidates {
		cm, ok := c.(map[string]any)
		if !ok {
			t.Fatalf("candidate is %T, want object", c)
		}
		candidateNames[cm["toolRef"].(string)] = true
	}
	if !candidateNames["fixture.fixture_search"] {
		t.Error("missing fixture.fixture_search candidate")
	}
	if !candidateNames["fixture.fixture_read"] {
		t.Error("missing fixture.fixture_read candidate")
	}
}

func TestAdapter_CallToolReturnsStructuredFailure(t *testing.T) {
	cs := connect(t)
	res, err := cs.CallTool(context.Background(), &mcpsdk.CallToolParams{
		Name:      "callTool",
		Arguments: map[string]any{"toolRef": "atlassian.confluence_search", "arguments": map[string]any{}},
	})
	if err != nil {
		t.Fatalf("CallTool(callTool): %v", err)
	}
	if !res.IsError {
		t.Error("callTool failure should set IsError")
	}
	payload := textPayload(t, res)
	if payload["ok"] != false {
		t.Errorf("ok = %v, want false", payload["ok"])
	}
	errObj, ok := payload["error"].(map[string]any)
	if !ok {
		t.Fatalf("error field = %v, want object", payload["error"])
	}
	if errObj["type"] == "" || errObj["type"] == nil {
		t.Error("failure must carry an error.type")
	}
	if errObj["agentInstruction"] == "" || errObj["agentInstruction"] == nil {
		t.Error("failure must carry an agentInstruction")
	}
}

func TestAdapter_CallToolInvokesFixtureDownstreamAndNormalizesResult(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	// Factory: spawns a fresh fixture downstream server with one echo tool on
	// each connect. Each connect gets a fresh in-memory transport pair because
	// net.Pipe-backed transports are single-session.
	factory := func(_ string, _ config.ServerConfig) (mcpsdk.Transport, error) {
		dsServerT, dsClientT := mcpsdk.NewInMemoryTransports()
		dsServer := mcpsdk.NewServer(&mcpsdk.Implementation{Name: "fixture-downstream", Version: "0"}, nil)
		mcpsdk.AddTool(dsServer, &mcpsdk.Tool{
			Name:        "fixture_echo",
			Title:       "Fixture Echo",
			Description: "Echo the supplied text argument back",
			InputSchema: map[string]any{
				"type":       "object",
				"properties": map[string]any{"text": map[string]any{"type": "string"}},
			},
		}, func(_ context.Context, _ *mcpsdk.CallToolRequest, args map[string]any) (*mcpsdk.CallToolResult, any, error) {
			text, _ := args["text"].(string)
			return &mcpsdk.CallToolResult{
				Content: []mcpsdk.Content{&mcpsdk.TextContent{Text: "echo: " + text}},
			}, nil, nil
		})
		go func() { _ = dsServer.Run(ctx, dsServerT) }()
		return dsClientT, nil
	}

	connector := downstream.New(downstream.WithTransportFactory(factory))
	liveBroker := broker.NewLive(catalog.NewMemory(), &config.Config{
		MCP: map[string]config.ServerConfig{
			"fixture": {Type: "local", Enabled: true},
		},
	}, connector)

	ozyServerT, ozyClientT := mcpsdk.NewInMemoryTransports()
	adapter := New(liveBroker, "test")
	go func() { _ = adapter.Server().Run(ctx, ozyServerT) }()

	client := mcpsdk.NewClient(&mcpsdk.Implementation{Name: "test", Version: "0"}, nil)
	cs, err := client.Connect(ctx, ozyClientT, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { _ = cs.Close() })

	// Downstream tools must not leak through the adapter surface.
	list, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	if len(list.Tools) != 3 {
		t.Errorf("advertised %d tools, want 3: %+v", len(list.Tools), list.Tools)
	}
	advertised := make(map[string]bool, len(list.Tools))
	for _, tool := range list.Tools {
		advertised[tool.Name] = true
	}
	for _, name := range []string{"findTool", "describeTool", "callTool"} {
		if !advertised[name] {
			t.Errorf("missing advertised tool %q", name)
		}
	}
	for _, name := range []string{"fixture_echo", "fixture_read", "fixture_search"} {
		if advertised[name] {
			t.Errorf("downstream tool %q leaked into the advertised set", name)
		}
	}

	res, err := cs.CallTool(context.Background(), &mcpsdk.CallToolParams{
		Name: "callTool",
		Arguments: map[string]any{
			"toolRef":   "fixture.fixture_echo",
			"arguments": map[string]any{"text": "hello"},
		},
	})
	if err != nil {
		t.Fatalf("CallTool(callTool): %v", err)
	}
	if res.IsError {
		t.Fatalf("callTool returned IsError=true; content=%+v", res.Content)
	}
	payload := textPayload(t, res)
	if payload["ok"] != true {
		t.Errorf("ok = %v, want true", payload["ok"])
	}
	if payload["toolRef"] != "fixture.fixture_echo" {
		t.Errorf("toolRef = %v, want fixture.fixture_echo", payload["toolRef"])
	}
	got, _ := payload["result"].(string)
	if !strings.Contains(got, "hello") {
		t.Errorf("result = %q, want substring %q", got, "hello")
	}
	summary, _ := payload["resultSummary"].(string)
	if summary == "" {
		t.Error("resultSummary is empty, want non-empty")
	}
}

func TestAdapter_FindToolThenCallToolEndToEndWithoutIndex(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	factory := func(_ string, _ config.ServerConfig) (mcpsdk.Transport, error) {
		dsServerT, dsClientT := mcpsdk.NewInMemoryTransports()
		dsServer := mcpsdk.NewServer(&mcpsdk.Implementation{Name: "fixture-downstream", Version: "0"}, nil)
		mcpsdk.AddTool(dsServer, &mcpsdk.Tool{
			Name:        "fixture_echo",
			Title:       "Fixture Echo",
			Description: "Echo the supplied text argument back",
			InputSchema: map[string]any{
				"type":       "object",
				"properties": map[string]any{"text": map[string]any{"type": "string"}},
			},
		}, func(_ context.Context, _ *mcpsdk.CallToolRequest, args map[string]any) (*mcpsdk.CallToolResult, any, error) {
			text, _ := args["text"].(string)
			return &mcpsdk.CallToolResult{
				Content: []mcpsdk.Content{&mcpsdk.TextContent{Text: "echo: " + text}},
			}, nil, nil
		})
		mcpsdk.AddTool(dsServer, &mcpsdk.Tool{
			Name:        "fixture_read",
			Title:       "Fixture Read",
			Description: "Return a fixed fixture string",
		}, func(context.Context, *mcpsdk.CallToolRequest, map[string]any) (*mcpsdk.CallToolResult, any, error) {
			return &mcpsdk.CallToolResult{
				Content: []mcpsdk.Content{&mcpsdk.TextContent{Text: "fixture_read_payload"}},
			}, nil, nil
		})
		go func() { _ = dsServer.Run(ctx, dsServerT) }()
		return dsClientT, nil
	}

	connector := downstream.New(downstream.WithTransportFactory(factory))
	liveBroker := broker.NewLive(catalog.NewMemory(), &config.Config{
		MCP: map[string]config.ServerConfig{
			"fixture": {Type: "local", Enabled: true},
		},
	}, connector)

	ozyServerT, ozyClientT := mcpsdk.NewInMemoryTransports()
	adapter := New(liveBroker, "test")
	go func() { _ = adapter.Server().Run(ctx, ozyServerT) }()

	client := mcpsdk.NewClient(&mcpsdk.Implementation{Name: "test", Version: "0"}, nil)
	cs, err := client.Connect(ctx, ozyClientT, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { _ = cs.Close() })

	// Live discovery through findTool — no ozy index, no catalog priming.
	res, err := cs.CallTool(context.Background(), &mcpsdk.CallToolParams{
		Name:      "findTool",
		Arguments: map[string]any{"query": "read"},
	})
	if err != nil {
		t.Fatalf("CallTool(findTool): %v", err)
	}
	payload := textPayload(t, res)
	if payload["decision"] != "choose_from_candidates" {
		t.Fatalf("decision = %v, want choose_from_candidates", payload["decision"])
	}
	candidates, ok := payload["candidates"].([]any)
	if !ok || len(candidates) == 0 {
		t.Fatalf("candidates = %v, want non-empty array", payload["candidates"])
	}

	var picked string
	for _, c := range candidates {
		cm, ok := c.(map[string]any)
		if !ok {
			continue
		}
		if name, _ := cm["name"].(string); name == "fixture_read" {
			ref, _ := cm["toolRef"].(string)
			if !strings.HasPrefix(ref, "fixture.") {
				t.Fatalf("picked toolRef %q does not start with %q", ref, "fixture.")
			}
			picked = ref
			break
		}
	}
	if picked == "" {
		t.Fatalf("no candidate named %q among %v", "fixture_read", candidates)
	}

	// Live invocation through callTool — no catalog, no pre-warmed session.
	res, err = cs.CallTool(context.Background(), &mcpsdk.CallToolParams{
		Name: "callTool",
		Arguments: map[string]any{
			"toolRef":   picked,
			"arguments": map[string]any{},
		},
	})
	if err != nil {
		t.Fatalf("CallTool(callTool): %v", err)
	}
	if res.IsError {
		t.Fatalf("callTool returned IsError=true; content=%+v", res.Content)
	}
	payload = textPayload(t, res)
	if payload["ok"] != true {
		t.Errorf("ok = %v, want true", payload["ok"])
	}
	if payload["toolRef"] != picked {
		t.Errorf("toolRef = %v, want %q", payload["toolRef"], picked)
	}
	got, _ := payload["result"].(string)
	if !strings.Contains(got, "fixture_read") {
		t.Errorf("result = %q, want substring %q", got, "fixture_read")
	}
	if summary, _ := payload["resultSummary"].(string); summary == "" {
		t.Error("resultSummary is empty, want non-empty")
	}
}
