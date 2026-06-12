package cli

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/spf13/cobra"

	"github.com/rokask/ozy/internal/catalog"
	"github.com/rokask/ozy/internal/config"
	"github.com/rokask/ozy/internal/contract"
	"github.com/rokask/ozy/internal/downstream"
)

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

	return res
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
