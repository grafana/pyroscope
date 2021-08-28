package command

import (
	"github.com/pyroscope-io/pyroscope/pkg/cli"
	"github.com/pyroscope-io/pyroscope/pkg/config"
	"github.com/pyroscope-io/pyroscope/pkg/convert"
	"github.com/spf13/cobra"
)

func newConvertCmd(cfg *config.Convert) *cobra.Command {
	vpr := newViper()
	convertCmd := &cobra.Command{
		Use:   "convert [flags] <input-file>",
		Short: "Convert between different profiling formats",
		RunE: createCmdRunFn(cfg, vpr, false, func(cmd *cobra.Command, args []string, logger config.LoggerFunc) error {
			return convert.Cli(cfg, logger, args)
		}),
		Hidden: true,
	}

	cli.PopulateFlagSet(cfg, convertCmd.Flags(), vpr)
	return convertCmd
}
