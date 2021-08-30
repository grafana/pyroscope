package command

import (
	"github.com/pyroscope-io/pyroscope/pkg/cli"
	"github.com/pyroscope-io/pyroscope/pkg/config"
	"github.com/pyroscope-io/pyroscope/pkg/exec"
	"github.com/spf13/cobra"
)

func newConnectCmd(cfg *config.Exec) *cobra.Command {
	vpr := newViper()
	connectCmd := &cobra.Command{
		Use:   "connect [flags]",
		Short: "Connect to an existing process and profile it",
		RunE: createCmdRunFn(cfg, vpr, true, func(cmd *cobra.Command, args []string, logger config.LoggerFunc) error {
			return exec.Cli(cfg, args)
		}),
	}

	cli.PopulateFlagSet(cfg, connectCmd.Flags(), vpr, cli.WithSkip("group-name", "user-name", "no-root-drop"))
	return connectCmd
}
