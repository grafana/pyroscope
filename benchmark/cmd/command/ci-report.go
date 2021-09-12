package command

import (
	"context"
	"fmt"
	"time"

	"github.com/pyroscope-io/pyroscope/benchmark/cireport"
	"github.com/pyroscope-io/pyroscope/benchmark/config"
	"github.com/pyroscope-io/pyroscope/benchmark/promquery"
	"github.com/pyroscope-io/pyroscope/pkg/cli"
	"github.com/sirupsen/logrus"
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
		RunE: cli.CreateCmdRunFn(&cfg.ImageReport, vpr, func(_ *cobra.Command, args []string) error {
			setLogLevel(cfg.ImageReport.LogLevel)

			logrus.Debugf("config %+v", cfg.ImageReport)

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

	report.AddCommand(tableReport)
	report.AddCommand(imageReport)

	cli.PopulateFlagSet(&cfg.TableReport, tableReport.Flags(), vpr)
	cli.PopulateFlagSet(&cfg.ImageReport, imageReport.Flags(), vpr)
	return report
}
