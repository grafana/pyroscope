package command

import (
	"github.com/spf13/cobra"

	"github.com/pyroscope-io/pyroscope/pkg/build"
)

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Args:  cobra.NoArgs,
		Short: "Print pyroscope version details",
		Run: func(cmd *cobra.Command, _ []string) {
			printVersion(cmd)
		},
	}
}

func printVersion(cmd *cobra.Command) {
	cmd.Println(gradientBanner())
	cmd.Println(build.Summary())
	cmd.Println()
}
