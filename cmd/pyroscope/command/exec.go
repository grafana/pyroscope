package command

import (
	"github.com/pyroscope-io/pyroscope/pkg/cli"
	"github.com/pyroscope-io/pyroscope/pkg/config"
	"github.com/pyroscope-io/pyroscope/pkg/exec"
	"github.com/spf13/cobra"
)

func newExecCmd(cfg *config.Exec) *cobra.Command {
	vpr := newViper()
	execCmd := &cobra.Command{
		Use:   "exec [flags] <args>",
		Short: "Start a new process from arguments and profile it",
		RunE: createCmdRunFn(cfg, vpr, true, func(cmd *cobra.Command, args []string, logger config.LoggerFunc) error {
			return exec.Cli(cfg, args)
		}),
	}

	cli.PopulateFlagSet(cfg, execCmd.Flags(), vpr, cli.WithSkip("pid"))
	return execCmd
}
