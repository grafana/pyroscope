package command

import (
	//	benchConfig "github.com/pyroscope-io/pyroscope/benchmark/config"
	"github.com/pyroscope-io/pyroscope/pkg/config"
	"github.com/spf13/cobra"
)

func newLoadGen(cfg *config.Bench) *cobra.Command {
	vpr := newViper()
	loadgenCmd := &cobra.Command{
		Use:   "loadgen [flags]",
		Short: "Generates load",
		RunE: createCmdRunFn(cfg, vpr, false, func(cmd *cobra.Command, args []string, logger config.LoggerFunc) error {
			return nil
			//			return cli.StartServer(cfg)
		}),
	}

	//	cli.PopulateFlagSet(cfg, loadgenCmd.Flags(), vpr, cli.WithSkip("metric-export-rules"))
	return loadgenCmd
}
