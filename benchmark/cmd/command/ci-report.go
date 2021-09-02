package command

import (
	"fmt"

	"github.com/pyroscope-io/pyroscope/benchmark/cireport"
	"github.com/pyroscope-io/pyroscope/benchmark/config"
	"github.com/pyroscope-io/pyroscope/benchmark/promquery"
	"github.com/pyroscope-io/pyroscope/pkg/cli"
	"github.com/spf13/cobra"
)

func newCIReport(cfg *config.CIReport) *cobra.Command {
	vpr := newViper()
	ciReport := &cobra.Command{
		Use:    "ci-report [flags]",
		Short:  "markdown report to be used by ci",
		Hidden: true,
		RunE: cli.CreateCmdRunFn(cfg, vpr, func(_ *cobra.Command, args []string) error {
			// TODO
			// get same data from the command line?
			pq := promquery.New(&config.PromQuery{
				PrometheusAddress: "http://prometheus:9090",
				//PrometheusAddress: "http://localhost:9091",
			})

			r := cireport.New(pq)

			report, err := r.Report()
			if err != nil {
				return err
			}

			fmt.Println(report)
			r.Report()
			return nil
		}),
	}

	cli.PopulateFlagSet(cfg, ciReport.Flags(), vpr)
	return ciReport
}
