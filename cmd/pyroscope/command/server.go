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
		Args:  cobra.NoArgs,
		Short: "Start pyroscope server. This is the database + web-based user interface",
		RunE: createCmdRunFn(cfg, vpr, func(cmd *cobra.Command, args []string) error {
			return cli.StartServer(cfg)
		}),
	}

	cli.PopulateFlagSet(cfg, serverCmd.Flags(), vpr)
	_ = serverCmd.Flags().MarkHidden("metric-export-rules")
	return serverCmd
}
