package command

import (
	"fmt"

	"github.com/pyroscope-io/pyroscope/benchmark/config"
	"github.com/pyroscope-io/pyroscope/pkg/cli"
	"github.com/spf13/cobra"
)

func newLoadGen(cfg *config.Config) *cobra.Command {
	vpr := newViper()
	loadgenCmd := &cobra.Command{
		Use:   "loadgen [flags]",
		Short: "Generates load",
		RunE: createCmdRunFn(cfg, vpr, false, func(cmd *cobra.Command, args []string, logger config.LoggerFunc) error {
			fmt.Println("address", cfg.ServerAddress)

			return nil
		}),
	}

	cli.PopulateFlagSet(cfg, loadgenCmd.Flags(), vpr)
	return loadgenCmd
}
