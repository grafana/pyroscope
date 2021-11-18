package command

import (
	"github.com/spf13/cobra"

	"github.com/pyroscope-io/pyroscope/pkg/adhoc"
	"github.com/pyroscope-io/pyroscope/pkg/cli"
	"github.com/pyroscope-io/pyroscope/pkg/config"
)

func newAdhocCmd(cfg *config.Adhoc) *cobra.Command {
	vpr := newViper()
	adhocCmd := &cobra.Command{
		Use:   "adhoc [flags]",
		Short: "Start a new process from arguments, profile it and view the results",
		Args:  cobra.MinimumNArgs(1),

		DisableFlagParsing: true,
		RunE: cli.CreateCmdRunFn(cfg, vpr, func(_ *cobra.Command, args []string) error {
			return adhoc.Cli(cfg, args)
		}),
	}

	cli.PopulateFlagSet(cfg, adhocCmd.Flags(), vpr)
	cli.PopulateFlagSet(cfg.Exec, adhocCmd.Flags(), vpr, cli.WithSkip(
		"auth-token",
		"log-level",
		"pid",
		"server-address",
		"upstream-threads",
		"upstream-request-timeout",
		"tags",
	))
	cli.PopulateFlagSet(cfg.Server, adhocCmd.Flags(), vpr, cli.WithSkip(
		"badger-no-truncate",
		"cache-evict-threshold",
		"cache-evict-volume",
		"cache-dimensions-size",
		"cache-dictonary-size",
		"cache-segment-size",
		"cache-tree-size",
		"hide-applications",
		"max-nodes-serialization",
		"metrics-export-rules",
		"out-of-space-threshold",
		"retention",
		"retention-levels",
		"sample-rate",
		"storage-path",
	))
	return adhocCmd
}
