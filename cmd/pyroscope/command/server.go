package command

import (
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
		RunE: func(cmd *cobra.Command, args []string) error {
			srv, err := cli.NewServer(cli.NewViperConfigProvider(vpr, cmd, args))
			if err != nil {
				return err
			}
			return srv.Start()
		},
	}

	cli.PopulateFlagSet(cfg, serverCmd.Flags(), vpr, cli.WithSkip("scrape-configs"))
	_ = serverCmd.Flags().MarkHidden("metrics-export-rules")
	return serverCmd
}
