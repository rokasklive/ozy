package bench

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
)

func (a *app) runCmd() *cobra.Command {
	var (
		scenario string
		mode     string
		numRuns  int
	)
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run a scenario benchmark",
		Long:  "Run a scenario benchmark in direct, ozy, or both modes against an OpenAI-compatible model endpoint.",
		RunE: func(_ *cobra.Command, _ []string) error {
			if scenario == "" {
				scenario = os.Getenv("SCENARIO")
			}
			if scenario == "" {
				scenario = "suspended-account-invoice-regression"
			}

			scenarioDir := filepath.Join("scenarios", scenario)
			cfgPath := filepath.Join(scenarioDir, "scenario.jsonc")

			cfg, err := LoadScenario(cfgPath)
			if err != nil {
				fmt.Fprintf(a.errOut, "ozy-bench run: load scenario: %v\n", err)
				return nil
			}

			runCount := ResolveRunCount(numRuns, cfg)
			timestamp := time.Now().UTC().Format("20060102-150405")
			// ponytail: paths are relative to the bench/ working dir (cwd=/bench in
			// the container, where ./runs is bind-mounted to the host). "bench/runs"
			// here would resolve to /bench/bench/runs — outside the mount — and the
			// whole run would be discarded when the container exits. Keep it a sibling
			// of scenarioDir ("scenarios/<name>").
			runDir := filepath.Join("runs", timestamp+"-"+scenario)

			fixtureDir := os.Getenv("OZY_BENCH_FIXTURE_DIR")
			if fixtureDir == "" {
				fixtureDir, _ = filepath.Abs(scenarioDir)
			}

			fmt.Fprintf(a.out, "=== Scenario bench: %s ===\n", scenario)
			fmt.Fprintf(a.out, "Mode: %s, Runs: %d\n", mode, runCount)

			if os.Getenv("MODEL_BASE_URL") == "" {
				fmt.Fprintf(a.out, "=== Live agent tier skipped (no MODEL_BASE_URL) ===\n")
				return nil
			}

			orchestrator := &Orchestrator{
				Scenario:   cfg,
				FixtureDir: fixtureDir,
				RunDir:     runDir,
				NumRuns:    runCount,
				Mode:       mode,
			}

			fmt.Fprintf(a.out, "=== Live agent tier ===\n")
			if err := orchestrator.Run(context.Background()); err != nil {
				fmt.Fprintf(a.errOut, "ozy-bench run: %v\n", err)
				return nil
			}

			fmt.Fprintf(a.out, "Run directory: %s\n", runDir)
			fmt.Fprintf(a.out, "Comparison: %s/comparison.md\n", runDir)
			return nil
		},
	}
	cmd.Flags().StringVar(&scenario, "scenario", "", "scenario name to run")
	cmd.Flags().StringVar(&mode, "mode", "both", "execution mode: direct, ozy, or both")
	cmd.Flags().IntVar(&numRuns, "runs", 0, "number of live runs per mode (overrides scenario config and BENCH_RUNS)")
	return cmd
}
