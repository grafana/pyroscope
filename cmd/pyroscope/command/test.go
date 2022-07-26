package command

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/pyroscope-io/pyroscope/pkg/cli"
	"github.com/pyroscope-io/pyroscope/pkg/config"
)

func newTestCmd(cfg *config.Test) *cobra.Command {
	vpr := newViper()

	var cmd *cobra.Command
	cmd = &cobra.Command{
		Use:   "test",
		Short: "profile tests",
		RunE: cli.CreateCmdRunFn(cfg, vpr, func(_ *cobra.Command, _ []string) error {
			fmt.Println(cfg)
			printUsageMessage(cmd)
			return nil
		}),
	}

	// admin
	cmd.AddCommand(newTestGoCmd(cfg))

	return cmd
}

func newTestGoCmd(cfg *config.Test) *cobra.Command {
	vpr := newViper()

	var cmd *cobra.Command
	cmd = &cobra.Command{
		Use:   "go",
		Short: "",
		RunE: cli.CreateCmdRunFn(cfg, vpr, func(_ *cobra.Command, _ []string) error {
			printUsageMessage(cmd)
			return nil
		}),
	}

	return cmd
}
