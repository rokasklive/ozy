// Package downstream connects to configured downstream MCP servers and returns
// initialized client sessions isolated per server.
package downstream

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"sort"
	"strings"
	"sync"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/rokask/ozy/internal/config"
	"github.com/rokask/ozy/internal/contract"
)

const (
	defaultMaxConcurrency = 4
	redacted              = "****"
)

// Session is the downstream MCP client surface used by discovery.
type Session interface {
	ListTools(ctx context.Context, params *mcpsdk.ListToolsParams) (*mcpsdk.ListToolsResult, error)
	Close() error
}

// Result is the per-server outcome of a connection attempt.
type Result struct {
	ServerID string
	Session  Session
	Error    *contract.Error
	Skipped  bool
}

// TransportFactory builds a fresh MCP transport for one configured server.
type TransportFactory func(serverID string, server config.ServerConfig) (mcpsdk.Transport, error)

// Connector opens MCP client sessions for enabled downstream servers.
type Connector struct {
	client           *mcpsdk.Client
	transportFactory TransportFactory
	maxConcurrency   int
}

// Option customizes a Connector.
type Option func(*Connector)

// WithTransportFactory replaces default local/remote transport construction.
// Tests use this to connect to in-memory MCP servers.
func WithTransportFactory(factory TransportFactory) Option {
	return func(c *Connector) {
		if factory != nil {
			c.transportFactory = factory
		}
	}
}

// WithMaxConcurrency bounds concurrent connection attempts.
func WithMaxConcurrency(limit int) Option {
	return func(c *Connector) {
		if limit > 0 {
			c.maxConcurrency = limit
		}
	}
}

// New constructs a Connector backed by the official MCP Go SDK client.
func New(opts ...Option) *Connector {
	c := &Connector{
		client: mcpsdk.NewClient(&mcpsdk.Implementation{
			Name:    "ozy",
			Version: "0.1.0-dev",
		}, nil),
		maxConcurrency: defaultMaxConcurrency,
	}
	c.transportFactory = c.defaultTransport
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// ConnectAll connects every enabled server in stable order. Each server receives
// an independent result; one failure does not abort the rest.
func (c *Connector) ConnectAll(ctx context.Context, cfg *config.Config) []Result {
	if cfg == nil || len(cfg.MCP) == 0 {
		return nil
	}
	ids := make([]string, 0, len(cfg.MCP))
	for id := range cfg.MCP {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	results := make([]Result, len(ids))
	sem := make(chan struct{}, c.maxConcurrency)
	var wg sync.WaitGroup
	for i, id := range ids {
		server := cfg.MCP[id]
		results[i] = Result{ServerID: id}
		if !server.IsEnabled() {
			results[i].Skipped = true
			continue
		}

		wg.Add(1)
		go func(i int, id string, server config.ServerConfig) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				results[i].Error = connectionError(id, server, ctx.Err())
				return
			}
			serverCtx, cancel := context.WithTimeout(ctx, server.DiscoveryTimeout())
			defer cancel()
			results[i] = c.Connect(serverCtx, id, server)
		}(i, id, server)
	}
	wg.Wait()
	return results
}

// Connect opens a single initialized MCP client session.
func (c *Connector) Connect(ctx context.Context, serverID string, server config.ServerConfig) Result {
	result := Result{ServerID: serverID}
	transport, err := c.transportForServer(serverID, server)
	if err != nil {
		result.Error = configError(serverID, server, err)
		return result
	}
	session, err := c.client.Connect(ctx, transport, nil)
	if err != nil {
		result.Error = connectionError(serverID, server, err)
		return result
	}
	result.Session = session
	return result
}

func (c *Connector) transportForServer(serverID string, server config.ServerConfig) (mcpsdk.Transport, error) {
	return c.transportFactory(serverID, server)
}

func (c *Connector) defaultTransport(_ string, server config.ServerConfig) (mcpsdk.Transport, error) {
	switch server.Type {
	case "local":
		if len(server.Command) == 0 || server.Command[0] == "" {
			return nil, fmt.Errorf("local server missing command")
		}
		// #nosec G204 -- local MCP commands are explicitly user-configured.
		cmd := exec.Command(server.Command[0], server.Command[1:]...)
		cmd.Dir = server.CWD
		if len(server.Environment) > 0 {
			cmd.Env = os.Environ()
			keys := make([]string, 0, len(server.Environment))
			for k := range server.Environment {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				cmd.Env = append(cmd.Env, k+"="+server.Environment[k])
			}
		}
		return &mcpsdk.CommandTransport{Command: cmd}, nil
	case "remote":
		if server.URL == "" {
			return nil, fmt.Errorf("remote server missing url")
		}
		return &mcpsdk.StreamableClientTransport{
			Endpoint:             server.URL,
			HTTPClient:           &http.Client{Transport: headerRoundTripper{headers: server.Headers, base: http.DefaultTransport}},
			MaxRetries:           -1,
			DisableStandaloneSSE: true,
		}, nil
	default:
		return nil, fmt.Errorf("unknown server type %q", server.Type)
	}
}

type headerRoundTripper struct {
	headers map[string]string
	base    http.RoundTripper
}

func (h headerRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	base := h.base
	if base == nil {
		base = http.DefaultTransport
	}
	clone := req.Clone(req.Context())
	clone.Header = req.Header.Clone()
	for k, v := range h.headers {
		clone.Header.Set(k, v)
	}
	return base.RoundTrip(clone)
}

func configError(serverID string, server config.ServerConfig, err error) *contract.Error {
	return &contract.Error{
		Type:             contract.ErrTypeConfigError,
		ServerID:         serverID,
		Retryable:        false,
		Message:          scrub(err.Error(), server),
		AgentInstruction: "Fix this server's configuration, then retry discovery.",
	}
}

func connectionError(serverID string, server config.ServerConfig, err error) *contract.Error {
	if isOAuthAuthFailure(server, err) {
		return &contract.Error{
			Type:             contract.ErrTypeAuthUnavailable,
			ServerID:         serverID,
			Retryable:        false,
			Message:          fmt.Sprintf("oauth authentication is required for server %q but Ozy does not implement the OAuth flow in this build: %s", serverID, scrub(err.Error(), server)),
			AgentInstruction: "Do not retry blindly. Ask the user to configure a non-OAuth credential path, disable this server, or wait for Ozy OAuth support.",
		}
	}
	return &contract.Error{
		Type:             contract.ErrTypeDownstreamServerOffline,
		ServerID:         serverID,
		Retryable:        true,
		Message:          fmt.Sprintf("could not connect to server %q: %s", serverID, scrub(err.Error(), server)),
		AgentInstruction: "Keep using other reachable servers. Check this server's command, URL, credentials, and network reachability before retrying.",
	}
}

func isOAuthAuthFailure(server config.ServerConfig, err error) bool {
	if len(server.OAuth) == 0 || strings.TrimSpace(string(server.OAuth)) == "false" || err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "401") ||
		strings.Contains(msg, "unauthorized") ||
		strings.Contains(msg, "oauth") ||
		strings.Contains(msg, "auth")
}

func scrub(msg string, server config.ServerConfig) string {
	for _, secret := range secretValues(server) {
		if secret == "" || strings.Contains(secret, "{env:") {
			continue
		}
		msg = strings.ReplaceAll(msg, secret, redacted)
	}
	return msg
}

func secretValues(server config.ServerConfig) []string {
	values := make([]string, 0, len(server.Headers)+len(server.Environment))
	for _, v := range server.Headers {
		values = append(values, v)
	}
	for _, v := range server.Environment {
		values = append(values, v)
	}
	return values
}
