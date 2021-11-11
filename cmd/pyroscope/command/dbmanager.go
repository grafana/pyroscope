package command

import (
	"github.com/spf13/cobra"

	"github.com/pyroscope-io/pyroscope/pkg/cli"
	"github.com/pyroscope-io/pyroscope/pkg/config"
	"github.com/pyroscope-io/pyroscope/pkg/dbmanager"
)

func newDbManagerCmd(cfg *config.CombinedDbManager) *cobra.Command {
	vpr := newViper()
	dbmanagerCmd := &cobra.Command{
		Use:    "dbmanager [flags] <args>",
		Short:  "Manage database",
		Args:   cobra.ExactArgs(1), // TODO: should be implemented as subcommands.
		Hidden: true,

		DisableFlagParsing: true,
		RunE: cli.CreateCmdRunFn(cfg, vpr, func(_ *cobra.Command, args []string) error {
			return dbmanager.Cli(cfg.DbManager, cfg.Server, args)
		}),
	}

	cli.PopulateFlagSet(cfg.DbManager, dbmanagerCmd.Flags(), vpr)
	cli.PopulateFlagSet(cfg.Server, dbmanagerCmd.Flags(), vpr, cli.WithSkip("log-level", "storage-path", "metrics-export-rules"))
	return dbmanagerCmd
}
