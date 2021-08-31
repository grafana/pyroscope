package command

import (
	"github.com/spf13/cobra"

	"github.com/pyroscope-io/pyroscope/pkg/cli"
	"github.com/pyroscope-io/pyroscope/pkg/config"
)

func newAgentCmd(cfg *config.Agent) *cobra.Command {
	vpr := newViper()
	agentCmd := &cobra.Command{
		Use:   "agent [flags]",
		Short: "Start pyroscope agent",
		Args:  cobra.NoArgs,
		RunE: createCmdRunFn(cfg, vpr, func(cmd *cobra.Command, args []string) error {
			return cli.StartAgent(cfg)
		}),
	}

	cli.PopulateFlagSet(cfg, agentCmd.Flags(), vpr, cli.WithSkip("targets"))
	return agentCmd
}
