package command

import (
	"github.com/pyroscope-io/pyroscope/pkg/cli"
	"github.com/pyroscope-io/pyroscope/pkg/config"
	"github.com/spf13/cobra"
)

func newServerCmd(cfg *config.Server) *cobra.Command {
	serverCmd := &cobra.Command{
		Use:   "server [flags]",
		Short: "starts pyroscope server. This is the database + web-based user interface",
		RunE: func(cmd *cobra.Command, args []string) error {
			err := loadConfig(cfg.Config, cfg)
			if err != nil {
				return err
			}

			err = cli.StartServer(cfg)
			if err != nil {
				// do not print usage in case of an error while running the subcommand
				cmd.SilenceUsage = true
			}

			return err
		},
	}

	loadFlags(cfg, serverCmd, cli.WithSkip("metric-export-rules"))
	return serverCmd
}
