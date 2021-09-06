package command

import (
	"context"
	"strconv"

	"github.com/pyroscope-io/pyroscope/benchmark/cireport"
	"github.com/pyroscope-io/pyroscope/benchmark/config"
	"github.com/pyroscope-io/pyroscope/pkg/cli"
	"github.com/spf13/cobra"
)

func newScreenshotPane(cfg *config.DashboardScreenshot) *cobra.Command {
	vpr := newViper()
	cmd := &cobra.Command{
		Use:    "screenshot-dashboard [from (unix time in ms)] [to (unix time in ms)] [flags]",
		Short:  "take a screenshot of all panes of a grafana dashboard",
		Hidden: true,
		Args:   cobra.ExactArgs(2),
		RunE: cli.CreateCmdRunFn(cfg, vpr, func(_ *cobra.Command, args []string) error {
			setLogLevel(cfg.LogLevel)

			from, err := strconv.Atoi(args[0])
			if err != nil {
				return err
			}

			to, err := strconv.Atoi(args[1])
			if err != nil {
				return err
			}

			_, err = cireport.ScreenshotAllPanes(context.Background(), cireport.ScreenshotAllPanesConfig{
				GrafanaURL:   cfg.GrafanaAddress,
				DashboardUid: cfg.DashboardUid,
				From:         from,
				To:           to,
				Dest:         cfg.Destination,
			})

			return err
		}),
	}

	cli.PopulateFlagSet(cfg, cmd.Flags(), vpr)
	return cmd
}
