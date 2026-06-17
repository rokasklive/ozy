package bench

import (
	"fmt"

	"github.com/spf13/cobra"
)

func (a *app) fixtureCmd() *cobra.Command {
	var (
		scenario string
		out      string
	)
	cmd := &cobra.Command{
		Use:   "fixture",
		Short: "Generate a scenario fixture",
		Long:  "Generate the deterministic fixture for a named scenario into a target directory.",
		RunE: func(_ *cobra.Command, _ []string) error {
			if scenario == "" {
				fmt.Fprintln(a.errOut, "ozy-bench fixture: --scenario is required")
				return nil
			}
			if out == "" {
				out = "/tmp/ozy-bench-fixture-" + scenario
			}

			meta, err := GenerateFixture(out)
			if err != nil {
				fmt.Fprintf(a.errOut, "ozy-bench fixture: %v\n", err)
				return nil
			}

			fmt.Fprintf(a.out, "Fixture generated at %s\n", meta.TargetDir)
			fmt.Fprintf(a.out, "  Culprit commit: %s (%s)\n", meta.CulpritHash, meta.CulpritSubject)
			return nil
		},
	}
	cmd.Flags().StringVar(&scenario, "scenario", "", "scenario name to generate fixture for")
	cmd.Flags().StringVar(&out, "out", "", "output directory (default: /tmp/ozy-bench-fixture-<scenario>)")
	return cmd
}
