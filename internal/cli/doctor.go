package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/rokask/ozy/internal/config"
	"github.com/rokask/ozy/internal/contract"
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
		Detail: fmt.Sprintf("%d configured", len(loaded.Resolved.Servers)),
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

	res.Checks = append(res.Checks, contract.DoctorCheck{
		Name:   "mcp-adapter",
		Status: contract.CheckOK,
		Detail: "ready (run `ozy mcp` to serve)",
	})

	return res
}
