package command

import (
	"context"

	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"

	"github.com/pyroscope-io/pyroscope/pkg/cli"
	"github.com/pyroscope-io/pyroscope/pkg/config"
	"github.com/pyroscope-io/pyroscope/pkg/exec"
)

func newAdhocCmd(cfg *config.Adhoc) *cobra.Command {
	vpr := newViper()
	adhocCmd := &cobra.Command{
		Use:   "adhoc [flags]",
		Short: "Start a new process from arguments, profile it and view the results",
		Args:  cobra.MinimumNArgs(1),

		DisableFlagParsing: true,
		RunE: cli.CreateCmdRunFn(cfg, vpr, func(_ *cobra.Command, args []string) error {
			g, ctx := errgroup.WithContext(context.Background())
			g.Go(func() error {
				return cli.StartServer(ctx, cfg.Server)
			})
			g.Go(func() error {
				return exec.Cli(cfg.Exec, args)
			})
			return g.Wait()
		}),
	}

	cli.PopulateFlagSet(cfg.Exec, adhocCmd.Flags(), vpr, cli.WithSkip("pid"))
	cli.PopulateFlagSet(cfg.Server, adhocCmd.Flags(), vpr, cli.WithSkip("log-level", "sample-rate"))
	return adhocCmd
}
