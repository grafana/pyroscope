package command

import (
	"github.com/spf13/cobra"

	"github.com/pyroscope-io/pyroscope/pkg/cli"
	"github.com/pyroscope-io/pyroscope/pkg/config"
	"github.com/pyroscope-io/pyroscope/pkg/exec"
)

func newConnectCmd(cfg *config.Connect) *cobra.Command {
	vpr := newViper()
	connectCmd := &cobra.Command{
		Use:   "connect [flags]",
		Short: "Connect to an existing process and profile it",
		Args:  cobra.NoArgs,

		RunE: cli.CreateCmdRunFn(cfg, vpr, func(_ *cobra.Command, _ []string) error {
			c, err := exec.NewConnect(cfg)
			if err != nil {
				return err
			}
			return c.Run()
		}),
	}

	cli.PopulateFlagSet(cfg, connectCmd.Flags(), vpr, cli.WithSkip("group-name", "user-name", "no-root-drop"))
	_ = connectCmd.MarkFlagRequired("pid")
	return connectCmd
}
