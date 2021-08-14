package command

import (
	"fmt"
	"os"
	goexec "os/exec"

	"github.com/pyroscope-io/pyroscope/pkg/cli"
	"github.com/pyroscope-io/pyroscope/pkg/config"
	"github.com/pyroscope-io/pyroscope/pkg/exec"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func newExecCmd(cfg *config.Exec) *cobra.Command {
	execCmd := &cobra.Command{
		Use:   "exec [flags] <args>",
		Short: "starts a new process from arguments and profiles it",
		RunE: func(cmd *cobra.Command, args []string) error {
			if cfg.NoLogging {
				logrus.SetLevel(logrus.PanicLevel)
			} else if l, err := logrus.ParseLevel(cfg.LogLevel); err == nil {
				logrus.SetLevel(l)
			}
			if len(args) == 0 || args[0] == "help" {
				fmt.Println(gradientBanner())
				fmt.Println(DefaultUsageFunc(cmd.Flags(), cmd))
				return nil
			}

			err := exec.Cli(cfg, args)
			if err != nil {
				cmd.SilenceUsage = true
			}
			// Normally, if the program ran, the call should return ExitError and
			// the exit code must be preserved. Otherwise, the error originates from
			// pyroscope and will be printed.
			if e, ok := err.(*goexec.ExitError); ok {
				os.Exit(e.ExitCode())
			}

			return err
		},
	}

	cli.PopulateFlagSet(cfg, execCmd.Flags(), cli.WithSkip("pid"))
	viper.BindPFlags(execCmd.Flags())

	execCmd.SetUsageFunc(func(cmd *cobra.Command) error {
		fmt.Println(gradientBanner() + "\n" + DefaultUsageFunc(cmd.Flags(), cmd))
		return nil
	})

	if err := viper.Unmarshal(cfg); err != nil {
		fmt.Fprintln(os.Stderr, "Unable to unmarshal:", err)
	}

	return execCmd
}
