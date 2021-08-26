package command

import (
	"fmt"

	"github.com/pyroscope-io/pyroscope/pkg/build"
	"github.com/spf13/cobra"
)

func newVersionCmd() *cobra.Command {
	versionCmd := &cobra.Command{
		Use:   "version",
		Short: "Print pyroscope version details",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println(gradientBanner())
			fmt.Println(build.Summary())
			fmt.Println("")
		},
	}

	return versionCmd
}
