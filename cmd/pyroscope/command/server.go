package command

import (
	"github.com/pyroscope-io/pyroscope/pkg/cli"
	"github.com/pyroscope-io/pyroscope/pkg/config"
	"github.com/spf13/cobra"
)

func newServerCmd(cfg *config.Server) *cobra.Command {
	vpr := newViper()
	serverCmd := &cobra.Command{
		Use:   "server [flags]",
		Short: "Start pyroscope server. This is the database + web-based user interface",
		RunE: createCmdRunFn(cfg, vpr, false, func(cmd *cobra.Command, args []string, logger config.LoggerFunc) error {
			return cli.StartServer(cfg)
		}),
	}

	cli.PopulateFlagSet(cfg, serverCmd.Flags(), vpr, cli.WithSkip("metric-export-rules"))
	return serverCmd
}
