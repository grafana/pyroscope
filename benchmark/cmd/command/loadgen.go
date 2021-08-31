package command

import (
	"github.com/pyroscope-io/pyroscope/benchmark/config"
	"github.com/spf13/cobra"
)

func newLoadGen(cfg *config.Config) *cobra.Command {
	vpr := newViper()
	loadgenCmd := &cobra.Command{
		Use:   "loadgen [flags]",
		Short: "Generates load",
		RunE: createCmdRunFn(cfg, vpr, false, func(cmd *cobra.Command, args []string, logger config.LoggerFunc) error {
			return nil
		}),
	}

	return loadgenCmd
}
