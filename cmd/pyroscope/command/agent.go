package command

import (
	"github.com/pyroscope-io/pyroscope/pkg/cli"
	"github.com/pyroscope-io/pyroscope/pkg/config"
	"github.com/spf13/cobra"
)

func newAgentCmd(cfg *config.Agent) *cobra.Command {
	vpr := newViper()
	agentCmd := &cobra.Command{
		Use:   "agent [flags]",
		Short: "Start pyroscope agent.",
		RunE: createCmdRunFn(cfg, vpr, false, func(cmd *cobra.Command, args []string, logger config.LoggerFunc) error {
			return cli.StartAgent(cfg)
		}),
	}

	cli.PopulateFlagSet(cfg, agentCmd.Flags(), vpr, cli.WithSkip("targets"))
	return agentCmd
}
