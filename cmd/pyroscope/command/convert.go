package command

import (
	"os"

	"github.com/pyroscope-io/pyroscope/pkg/config"
	"github.com/pyroscope-io/pyroscope/pkg/convert"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

func newConvertCmd(cfg *config.Convert) *cobra.Command {
	convertCmd := &cobra.Command{
		Use:   "convert [flags] <input-file>",
		Short: "converts between different profiling formats",
		RunE: func(cmd *cobra.Command, args []string) error {
			logrus.SetOutput(os.Stderr)
			logger := func(s string) {
				logrus.Fatal(s)
			}

			err := convert.Cli(cfg, logger, args)
			if err != nil {
				// do not print usage in case of an error while running the subcommand
				cmd.SilenceUsage = true
			}

			return err
		},
	}

	loadFlags(cfg, convertCmd)
	return convertCmd
}
