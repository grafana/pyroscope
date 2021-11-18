package command

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/pyroscope-io/pyroscope/pkg/cli"
	"github.com/pyroscope-io/pyroscope/pkg/config"
)

func newServerCmd(cfg *config.Server) *cobra.Command {
	vpr := newViper()
	serverCmd := &cobra.Command{
		Use:   "server [flags]",
		Short: "Start pyroscope server. This is the database + web-based user interface",

		DisableFlagParsing: true,
		RunE: cli.CreateCmdRunFn(cfg, vpr, func(_ *cobra.Command, _ []string) error {
			return cli.StartServer(context.Background(), cfg)
		}),
	}

	cli.PopulateFlagSet(cfg, serverCmd.Flags(), vpr)
	_ = serverCmd.Flags().MarkHidden("metrics-export-rules")
	return serverCmd
}
