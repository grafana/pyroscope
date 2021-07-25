package cmd

import (
	"fmt"
	"os"
	goexec "os/exec"

	"github.com/pyroscope-io/pyroscope/pkg/exec"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

// execCmd represents the exec command
var execCmd = &cobra.Command{
	Use:   "pyroscope exec [flags] <args>",
	Short: "starts a new process from arguments and profiles it",
	RunE: func(cmd *cobra.Command, args []string) error {
		if cfg.Exec.NoLogging {
			logrus.SetLevel(logrus.PanicLevel)
		} else if l, err := logrus.ParseLevel(cfg.Exec.LogLevel); err == nil {
			logrus.SetLevel(l)
		}
		if len(args) == 0 || args[0] == "help" {
			fmt.Println(gradientBanner())
			fmt.Println(DefaultUsageFunc(cmd.Flags(), cmd))
			return nil
		}
		err := exec.Cli(&cfg.Exec, args)
		// Normally, if the program ran, the call should return ExitError and
		// the exit code must be preserved. Otherwise, the error originates from
		// pyroscope and will be printed.
		if e, ok := err.(*goexec.ExitError); ok {
			os.Exit(e.ExitCode())
		}

		return err
	},
}

func init() {
	rootCmd.AddCommand(execCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// execCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// execCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
	execCmd.SetUsageFunc(func(cmd *cobra.Command) error {
		fmt.Println(gradientBanner() + "\n" + DefaultUsageFunc(cmd.Flags(), cmd))
		return nil
	})
}
