package command

import (
	"os"
	goexec "os/exec"

	"github.com/spf13/cobra"

	"github.com/pyroscope-io/pyroscope/pkg/cli"
	"github.com/pyroscope-io/pyroscope/pkg/config"
	"github.com/pyroscope-io/pyroscope/pkg/exec"
)

func newExecCmd(cfg *config.Exec) *cobra.Command {
	vpr := newViper()
	execCmd := &cobra.Command{
		Use:   "exec [flags] <args>",
		Short: "Start a new process from arguments and profile it",
		Args:  cobra.MinimumNArgs(1),

		DisableFlagParsing: true,
		RunE: createCmdRunFn(cfg, vpr, func(_ *cobra.Command, args []string) error {
			err := exec.Cli(cfg, args)
			// Normally, if the program ran, the call should return ExitError and
			// the exit code must be preserved. Otherwise, the error originates from
			// pyroscope and will be printed.
			if e, ok := err.(*goexec.ExitError); ok {
				os.Exit(e.ExitCode())
			}
			return err
		}),
	}

	cli.PopulateFlagSet(cfg, execCmd.Flags(), vpr, cli.WithSkip("pid"))
	return execCmd
}
