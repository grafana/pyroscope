package command

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/pyroscope-io/pyroscope/benchmark/cireport"
	"github.com/pyroscope-io/pyroscope/benchmark/config"
	"github.com/pyroscope-io/pyroscope/benchmark/promquery"
	"github.com/pyroscope-io/pyroscope/pkg/cli"
)

func newReport(cfg *config.Report) *cobra.Command {
	vpr := newViper()
	report := &cobra.Command{
		Use:    "report [subcommand]",
		Hidden: true,
	}

	// output logs to stderr
	// since the commands below output their data to stdout
	logrus.SetOutput(os.Stderr)

	tableReport := &cobra.Command{
		Use:   "table [flags]",
		Short: "runs queries and generates a markdown report",
		RunE: cli.CreateCmdRunFn(cfg, vpr, func(_ *cobra.Command, args []string) error {
			setLogLevel(cfg.TableReport.LogLevel)
			pq := promquery.New(&config.PromQuery{
				PrometheusAddress: cfg.PrometheusAddress,
			})

			r, err := cireport.NewTableReport(pq, cfg.TableReport)
			if err != nil {
				return err
			}

			report, err := r.Report(context.Background())
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
		RunE: cli.CreateCmdRunFn(cfg, vpr, func(_ *cobra.Command, args []string) error {
			setLogLevel(cfg.ImageReport.LogLevel)

			r, err := cireport.NewImageReporter(
				cfg.GrafanaAddress,
				cfg.TimeoutSeconds,
				cfg.UploadType,
				cfg.UploadBucket,
			)
			if err != nil {
				return err
			}

			now := time.Now()
			from := int64(cfg.From)
			to := int64(cfg.To)

			// set defaults if appropriate
			if to == 0 {
				// TODO use UnixMilli()
				to = now.UnixNano() / int64(time.Millisecond)
			}

			if from == 0 {
				// TODO use UnixMilli()
				from = now.Add(time.Duration(5)*-time.Minute).UnixNano() / int64(time.Millisecond)
			}

			report, err := r.ImageReport(context.Background(), cfg.DashboardUid, cfg.UploadDest, from, to)
			if err != nil {
				return err
			}

			fmt.Println(report)
			return nil
		}),
	}

	report.AddCommand(
		tableReport,
		imageReport,
	)

	cli.PopulateFlagSet(&cfg.ImageReport, imageReport.Flags(), vpr)
	cli.PopulateFlagSet(&cfg.TableReport, tableReport.Flags(), vpr)
	return report
}
