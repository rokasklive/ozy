package cli

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/rokasklive/ozy/internal/catalog"
	"github.com/rokasklive/ozy/internal/config"
	"github.com/rokasklive/ozy/internal/contract"
	"github.com/rokasklive/ozy/internal/downstream"
	"github.com/rokasklive/ozy/internal/sidecar"
)

// SidecarStatus is the embedding-subsystem health snapshot rendered by
// `ozy doctor`. Available=true means the Python sidecar is up and answered
// health/stats. Available=false means lexical-only mode is in effect (the
// Reason field carries the cause).
type SidecarStatus struct {
	Available   bool
	Model       string
	Dim         int
	Backend     string
	ToolCount   int
	VectorCount int
	Reason      string
}

// SidecarInspector returns the current embedding-subsystem status. The default
// (unset) inspector always reports Unavailable so `ozy doctor` renders the
// lexical-only notice without depending on the sidecar package. The sidecar
// package overrides this in an init() to wire the real probe.
type SidecarInspector func(ctx context.Context) SidecarStatus

// sidecarInspector is the active inspector. Tests override it via
// OverrideSidecarInspector; production code uses the sentinel default and
// runDoctor wires the real probe when semantic search is enabled.
var sidecarInspector SidecarInspector = func(_ context.Context) SidecarStatus {
	return SidecarStatus{Available: false, Reason: "semantic unavailable (lexical-only)"}
}

// sidecarInspectorOverridden is set true when a test overrides the inspector.
// runDoctor uses it to decide whether to wire the real sidecar probe.
var sidecarInspectorOverridden bool

// OverrideSidecarInspector replaces the default inspector. Tests use this to
// inject fakes. Production code should never call this.
func OverrideSidecarInspector(f SidecarInspector) {
	sidecarInspector = f
	sidecarInspectorOverridden = true
}

// newSidecarProbe returns a SidecarInspector that provisions and
// health-checks the Python embedding sidecar, returning the real status
// that `ozy doctor` reports. It uses the provisioned venv directory as
// the data directory so real vector and tool counts are visible.
// Provisioning honours the marker-based cache: a previously-provisioned
// venv skips the venv-creation step and returns in milliseconds.
func newSidecarProbe(emb config.EmbeddingConfig) SidecarInspector {
	return func(ctx context.Context) SidecarStatus {
		resolved, err := sidecar.Provision(ctx, sidecar.ProvisionOptions{
			Backend: emb.VectorBackend,
			Model:   emb.Model,
		})
		if err != nil {
			return SidecarStatus{Available: false, Reason: err.Error()}
		}

		client, err := sidecar.NewClient(sidecar.Options{
			DataDir: resolved.VenvDir,
			Backend: emb.VectorBackend,
			Model:   emb.Model,
			ProcessOptions: sidecar.ProcessOptions{
				PythonPath: resolved.PythonPath,
				SourceDir:  resolved.SourceDir,
				DataDir:    resolved.VenvDir,
				Backend:    emb.VectorBackend,
				Model:      emb.Model,
			},
		})
		if err != nil {
			return SidecarStatus{Available: false, Reason: "start: " + err.Error()}
		}
		defer func() { _ = client.Close() }()

		// Liveness first (fast): a dead sidecar fails here in seconds rather
		// than holding the warm-up's generous deadline.
		lctx, lcancel := context.WithTimeout(ctx, sidecarLivenessTimeout)
		hr := client.Health(lctx)
		lcancel()
		if !hr.OK {
			return SidecarStatus{Available: false, Reason: "health: " + errText(hr.Err)}
		}
		// Readiness warm-up: load the model and run a probe query. "Available"
		// is true only when this succeeds, so doctor reports semantic available
		// only when vectors are actually queryable — the same predicate the
		// daemon and `ozy index` use.
		rr := client.Ready(ctx)
		if !rr.OK {
			return SidecarStatus{Available: false, Reason: "warm-up: " + errText(rr.Err)}
		}

		stats, _ := client.Stats(ctx)
		return SidecarStatus{
			Available:   true,
			Model:       rr.Model,
			Dim:         rr.Dim,
			Backend:     rr.Backend,
			VectorCount: stats.VectorCount,
			ToolCount:   stats.ToolCount,
		}
	}
}

// sidecarLivenessTimeout bounds the fast liveness probe so a wedged sidecar
// fails quickly; the readiness warm-up gets its own generous deadline.
const sidecarLivenessTimeout = 10 * time.Second

// errText renders an error for a status reason, tolerating a nil error.
func errText(err error) string {
	if err == nil {
		return "unknown error"
	}
	return err.Error()
}

// inlineSecretKind names the secret pattern a config value matches, or "" when
// the value is clean or uses an {env:...} reference. Matching is per
// whitespace-separated field so short prefixes like sk- cannot fire inside
// ordinary words (desk-lamp).
func inlineSecretKind(value string) string {
	if value == "" || strings.Contains(value, "{env:") {
		return ""
	}
	for _, f := range strings.Fields(value) {
		switch {
		case strings.HasPrefix(f, "ghp_"), strings.HasPrefix(f, "gho_"), strings.HasPrefix(f, "ghs_"):
			return "GitHub token"
		case strings.HasPrefix(f, "github_pat_"):
			return "GitHub fine-grained token"
		case strings.HasPrefix(f, "sk-"):
			return "secret API key (sk-…)"
		case strings.HasPrefix(f, "AKIA"):
			return "AWS access key ID"
		case strings.HasPrefix(f, "xoxb-"), strings.HasPrefix(f, "xoxp-"),
			strings.HasPrefix(f, "xoxa-"), strings.HasPrefix(f, "xoxs-"):
			return "Slack token"
		}
	}
	if strings.HasPrefix(value, "Bearer ") {
		return "bearer credential"
	}
	return ""
}

// secretHygieneChecks scans raw server headers and environment values for
// inline secret-shaped literals and returns one WARN per finding, naming the
// server, field, and key — never any part of the value.
func secretHygieneChecks(raw *config.Config) []contract.DoctorCheck {
	if raw == nil {
		return nil
	}
	var checks []contract.DoctorCheck
	serverIDs := make([]string, 0, len(raw.MCP))
	for id := range raw.MCP {
		serverIDs = append(serverIDs, id)
	}
	sort.Strings(serverIDs)
	for _, id := range serverIDs {
		server := raw.MCP[id]
		for _, field := range []struct {
			name   string
			values map[string]string
		}{{"headers", server.Headers}, {"environment", server.Environment}} {
			keys := make([]string, 0, len(field.values))
			for k := range field.values {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				if kind := inlineSecretKind(field.values[k]); kind != "" {
					checks = append(checks, contract.DoctorCheck{
						Name:   "secrets",
						Status: contract.CheckWarn,
						Detail: fmt.Sprintf("server %q %s %q holds an inline %s; move it to an {env:NAME} reference and rotate the credential",
							id, field.name, k, kind),
					})
				}
			}
		}
	}
	return checks
}

func (a *app) doctorCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Diagnose configuration, environment, and adapter readiness",
		RunE: func(*cobra.Command, []string) error {
			a.emit(a.runDoctor())
			return nil
		},
	}
}

// runDoctor produces a diagnostics report (SPEC.md §17) without leaking secrets:
// it reports config validity, missing env references by name only, and adapter
// readiness.
func (a *app) runDoctor() *contract.DoctorResult {
	res := &contract.DoctorResult{OK: true}

	loaded, cerr := config.Load(a.configPath)
	if cerr != nil {
		res.OK = false
		res.Checks = append(res.Checks, contract.DoctorCheck{
			Name:   "config",
			Status: contract.CheckError,
			Detail: cerr.Message,
		})
		res.AgentInstruction = cerr.AgentInstruction
		return res
	}

	res.Checks = append(res.Checks, contract.DoctorCheck{
		Name:   "config",
		Status: contract.CheckOK,
		Detail: fmt.Sprintf("valid; loaded from %s", loaded.Path),
	})

	res.Checks = append(res.Checks, contract.DoctorCheck{
		Name:   "servers",
		Status: contract.CheckOK,
		Detail: fmt.Sprintf("%d configured", len(loaded.Resolved.MCP)),
	})

	// Secret hygiene runs on the RAW config: resolved values have {env:...}
	// already substituted, so scanning them would flag properly-referenced
	// secrets. Findings name the location and pattern kind — never the value.
	if hygiene := secretHygieneChecks(loaded.Raw); len(hygiene) > 0 {
		res.OK = false
		res.Checks = append(res.Checks, hygiene...)
	}

	// Missing env references are reported by variable name only — never values.
	if len(loaded.Missing) == 0 {
		res.Checks = append(res.Checks, contract.DoctorCheck{
			Name:   "environment",
			Status: contract.CheckOK,
			Detail: "all referenced environment variables are set",
		})
	} else {
		res.OK = false
		for _, m := range loaded.Missing {
			res.Checks = append(res.Checks, contract.DoctorCheck{
				Name:   "environment",
				Status: contract.CheckWarn,
				Detail: fmt.Sprintf("missing env var %s (server %q, field %s)", m.Var, m.Server, m.Field),
			})
		}
		res.AgentInstruction = "Set the missing environment variables, then re-run `ozy doctor`."
	}

	catalogTotal := 0
	toolCounts, err := indexedToolCounts()
	if err != nil {
		res.OK = false
		res.Checks = append(res.Checks, contract.DoctorCheck{
			Name:   "catalog",
			Status: contract.CheckWarn,
			Detail: fmt.Sprintf("could not read catalog: %v", err),
		})
	} else {
		for _, count := range toolCounts {
			catalogTotal += count
		}
		res.Checks = append(res.Checks, contract.DoctorCheck{
			Name:   "catalog",
			Status: contract.CheckOK,
			Detail: fmt.Sprintf("%d indexed tools", catalogTotal),
		})
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	serverHealth := downstream.New().ConnectAll(ctx, loaded.Resolved)
	for _, check := range serverHealthChecks(serverHealth, toolCounts) {
		if check.Status != contract.CheckOK {
			res.OK = false
		}
		res.Checks = append(res.Checks, check)
	}

	res.Checks = append(res.Checks, contract.DoctorCheck{
		Name:   "mcp-adapter",
		Status: contract.CheckOK,
		Detail: "ready (run `ozy mcp` to serve)",
	})

	// Embedding subsystem — Python sidecar health, backend, model, count.
	// When semantic is enabled and the inspector hasn't been overridden by
	// tests, wire the real sidecar probe; otherwise use the injected fake.
	if !sidecarInspectorOverridden && loaded.Resolved.Search.Semantic.Enabled {
		sidecarInspector = newSidecarProbe(loaded.Resolved.Embedding)
	}
	// The readiness probe may pay a cold model download on first run, so the
	// embedding check gets a generous ceiling; the probe's own liveness step
	// still fails fast on a dead sidecar.
	embCtx, embCancel := context.WithTimeout(context.Background(), sidecar.DefaultProvisionTimeout)
	res.Checks = append(res.Checks, embeddingCheck(embCtx, sidecarInspector, catalogTotal))
	embCancel()

	return res
}

// embeddingCheck turns a SidecarStatus into one DoctorCheck. The check is OK
// when the sidecar is up AND vector coverage matches the catalog; WARN otherwise
// (the user is still served lexical search — degradation is the supported safety
// net, not a failure). catalogTotal is the catalog tool count for the coverage
// cross-check.
func embeddingCheck(ctx context.Context, inspector SidecarInspector, catalogTotal int) contract.DoctorCheck {
	if inspector == nil {
		return contract.DoctorCheck{
			Name:   "embedding",
			Status: contract.CheckWarn,
			Detail: "semantic unavailable (lexical-only)",
		}
	}
	st := inspector(ctx)
	if !st.Available {
		return contract.DoctorCheck{
			Name:   "embedding",
			Status: contract.CheckWarn,
			Detail: "semantic unavailable (lexical-only)" + reasonSuffix(st.Reason),
		}
	}
	// Coverage cross-check: the sidecar is up, but if the catalog holds more
	// tools than the vector store has vectors, semantic results are partial or
	// stale. Surface it rather than reporting two independently green checks.
	if catalogTotal > 0 && st.VectorCount < catalogTotal {
		return contract.DoctorCheck{
			Name:   "embedding",
			Status: contract.CheckWarn,
			Detail: fmt.Sprintf("partial embedding coverage: %d vectors for %d catalog tools — run `ozy index` to rebuild", st.VectorCount, catalogTotal),
		}
	}
	return contract.DoctorCheck{
		Name:   "embedding",
		Status: contract.CheckOK,
		Detail: fmt.Sprintf("ready; backend=%s model=%s dim=%d vectors=%d catalog_tools=%d", st.Backend, st.Model, st.Dim, st.VectorCount, catalogTotal),
	}
}

func reasonSuffix(r string) string {
	if r == "" {
		return ""
	}
	return ": " + r
}

func indexedToolCounts() (map[string]int, error) {
	store, err := catalog.NewFile(catalog.DefaultPath())
	if err != nil {
		return nil, err
	}
	tools, err := store.Tools(context.Background())
	if err != nil {
		return nil, err
	}
	counts := make(map[string]int)
	for _, tool := range tools {
		counts[tool.ServerID]++
	}
	return counts, nil
}

func serverHealthChecks(results []downstream.Result, toolCounts map[string]int) []contract.DoctorCheck {
	if len(results) == 0 {
		return nil
	}
	out := make([]contract.DoctorCheck, 0, len(results))
	for _, result := range results {
		count := toolCounts[result.ServerID]
		check := contract.DoctorCheck{
			Name:   "server:" + result.ServerID,
			Status: contract.CheckOK,
			Detail: fmt.Sprintf("reachable; indexed tools: %d", count),
		}
		switch {
		case result.Skipped:
			check.Status = contract.CheckWarn
			check.Detail = fmt.Sprintf("disabled; indexed tools: %d", count)
		case result.Error != nil:
			check.Status = contract.CheckWarn
			check.Detail = fmt.Sprintf("unreachable: %s; indexed tools: %d", result.Error.Message, count)
		default:
			if result.Session != nil {
				_ = result.Session.Close()
			}
		}
		out = append(out, check)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}
