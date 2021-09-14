package command

import (
	"fmt"

	"github.com/pyroscope-io/pyroscope/benchmark/internal/cireport"
	"github.com/pyroscope-io/pyroscope/benchmark/internal/config"
	"github.com/pyroscope-io/pyroscope/benchmark/internal/promquery"
	"github.com/pyroscope-io/pyroscope/pkg/cli"
	"github.com/spf13/cobra"
)

func newReport(cfg *config.Report) *cobra.Command {
	report := &cobra.Command{
		Use:    "report [subcommand]",
		Hidden: true,
	}

	vpr := newViper()
	tableReport := &cobra.Command{
		Use:   "table [flags]",
		Short: "generates a markdown report to be used by ci",
		RunE: cli.CreateCmdRunFn(&cfg.TableReport, vpr, func(_ *cobra.Command, args []string) error {
			setLogLevel(cfg.TableReport.LogLevel)
			pq := promquery.New(&config.PromQuery{
				PrometheusAddress: cfg.PrometheusAddress,
			})

			report, err := cireport.TableReportCli(pq, cfg.TableReport.QueriesFile)
			if err != nil {
				return err
			}

			fmt.Println(report)
			return nil
		}),
	}

	imageReport := &cobra.Command{
		Use:   "image [flags]",
		Short: "generates a markdown report to be used by ci",
		RunE: cli.CreateCmdRunFn(&cfg.ImageReport, vpr, func(_ *cobra.Command, args []string) error {
			setLogLevel(cfg.ImageReport.LogLevel)

			report, err := cireport.ImageReportCLI(cfg.ImageReport)
			if err != nil {
				return err
			}

			fmt.Println(report)
			return nil
		}),
	}

	report.AddCommand(tableReport)
	report.AddCommand(imageReport)

	cli.PopulateFlagSet(&cfg.TableReport, tableReport.Flags(), vpr)
	cli.PopulateFlagSet(&cfg.ImageReport, imageReport.Flags(), vpr)
	return report
}
