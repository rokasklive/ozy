package cli

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/spf13/cobra"

	"github.com/rokasklive/ozy/internal/catalog"
	"github.com/rokasklive/ozy/internal/config"
	"github.com/rokasklive/ozy/internal/contract"
	"github.com/rokasklive/ozy/internal/downstream"
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

// sidecarInspector is the active inspector. Tests can override it.
var sidecarInspector SidecarInspector = func(_ context.Context) SidecarStatus {
	return SidecarStatus{Available: false, Reason: "semantic unavailable (lexical-only)"}
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

	toolCounts, err := indexedToolCounts()
	if err != nil {
		res.OK = false
		res.Checks = append(res.Checks, contract.DoctorCheck{
			Name:   "catalog",
			Status: contract.CheckWarn,
			Detail: fmt.Sprintf("could not read catalog: %v", err),
		})
	} else {
		total := 0
		for _, count := range toolCounts {
			total += count
		}
		res.Checks = append(res.Checks, contract.DoctorCheck{
			Name:   "catalog",
			Status: contract.CheckOK,
			Detail: fmt.Sprintf("%d indexed tools", total),
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
	// Reports a single check that is OK when the sidecar is up, WARN when it
	// is absent (lexical-only degradation is the supported safety net).
	embCtx, embCancel := context.WithTimeout(context.Background(), 3*time.Second)
	res.Checks = append(res.Checks, embeddingCheck(embCtx, sidecarInspector))
	embCancel()

	return res
}

// embeddingCheck turns a SidecarStatus into one DoctorCheck. The check is OK
// when the sidecar is up; WARN otherwise (the user is still served lexical
// search — degradation is the supported safety net, not a failure).
func embeddingCheck(ctx context.Context, inspector SidecarInspector) contract.DoctorCheck {
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
	return contract.DoctorCheck{
		Name:   "embedding",
		Status: contract.CheckOK,
		Detail: fmt.Sprintf("ready; backend=%s model=%s dim=%d indexed_tools=%d", st.Backend, st.Model, st.Dim, st.ToolCount),
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
