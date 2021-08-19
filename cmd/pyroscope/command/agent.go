package command

import (
	"github.com/pyroscope-io/pyroscope/pkg/cli"
	"github.com/pyroscope-io/pyroscope/pkg/config"
	"github.com/spf13/cobra"
)

func newAgentCmd(cfg *config.Agent) *cobra.Command {
	agentCmd := &cobra.Command{
		Use:   "agent [flags]",
		Short: "starts pyroscope agent.",
		RunE: func(cmd *cobra.Command, args []string) error {
			err := loadConfig(cfg.Config, cfg)
			if err != nil {
				return err
			}

			err = cli.StartAgent(cfg)
			if err != nil {
				// do not print usage in case of an error while running the subcommand
				cmd.SilenceUsage = true
			}

			return err
		},
	}

	loadFlags(cfg, agentCmd, cli.WithSkip("targets"))
	return agentCmd
}
