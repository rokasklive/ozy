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
		scenario    string
		mode        string
		numRuns     int
		surfaceOnly bool
	)
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run a scenario benchmark",
		Long:  "Run a scenario benchmark in direct, direct-lean, ozy, all, or both modes against an OpenCode built-in model. 'all' (the default) runs direct + direct-lean + ozy in one pass; 'both' is a back-compat alias for direct + ozy. Always writes the static surface tier; --surface-only skips the live tier.",
		// The harness exits 0 for any completed invocation and non-zero only for
		// harness errors (fixture/config failure, unresolvable model). A returned
		// error becomes a non-zero exit; nil is exit 0.
		RunE: func(_ *cobra.Command, _ []string) error {
			if scenario == "" {
				scenario = os.Getenv("SCENARIO")
			}
			if scenario == "" {
				scenario = "historical-weather-report"
			}

			scenarioDir := filepath.Join("scenarios", scenario)
			cfgPath := filepath.Join(scenarioDir, "scenario.jsonc")

			cfg, err := LoadScenario(cfgPath)
			if err != nil {
				return fmt.Errorf("load scenario: %w", err)
			}

			if mode == "" {
				mode = os.Getenv("MODE")
			}
			if mode == "" {
				mode = "all"
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

			// The corpus dir is scenario-independent (shared estate). Default to
			// bench/corpus relative to the bench cwd; OZY_BENCH_CORPUS_DIR overrides.
			corpusDir := os.Getenv("OZY_BENCH_CORPUS_DIR")
			if corpusDir == "" {
				corpusDir = "corpus"
			}

			fmt.Fprintf(a.out, "=== Scenario bench: %s ===\n", scenario)
			fmt.Fprintf(a.out, "Mode: %s, Runs: %d, Model: %s, Surface-only: %v\n", mode, runCount, benchModel(), surfaceOnly)

			orchestrator := &Orchestrator{
				Scenario:    cfg,
				FixtureDir:  fixtureDir,
				CorpusDir:   corpusDir,
				RunDir:      runDir,
				NumRuns:     runCount,
				Mode:        mode,
				SurfaceOnly: surfaceOnly,
			}

			if err := orchestrator.Run(context.Background()); err != nil {
				return fmt.Errorf("run: %w", err)
			}

			fmt.Fprintf(a.out, "Run directory: %s\n", runDir)
			fmt.Fprintf(a.out, "Comparison: %s/comparison.md\n", runDir)
			return nil
		},
	}
	cmd.Flags().StringVar(&scenario, "scenario", "", "scenario name to run (default: SCENARIO env or historical-weather-report)")
	cmd.Flags().StringVar(&mode, "mode", "", "execution mode: direct, direct-lean, ozy, all, or both (default: MODE env or all)")
	cmd.Flags().IntVar(&numRuns, "runs", 0, "live runs per mode; precedence: flag > BENCH_RUNS > scenario config > 5")
	cmd.Flags().BoolVar(&surfaceOnly, "surface-only", false, "compute the static surface tier only — no model, no live runs")
	return cmd
}
