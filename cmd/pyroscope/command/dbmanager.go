package command

import (
	"github.com/pyroscope-io/pyroscope/pkg/cli"
	"github.com/pyroscope-io/pyroscope/pkg/config"
	"github.com/pyroscope-io/pyroscope/pkg/dbmanager"
	"github.com/spf13/cobra"
)

func newDbManagerCmd(cfg *config.CombinedDbManager) *cobra.Command {
	vpr := newViper()
	dbmanagerCmd := &cobra.Command{
		Use:   "dbmanager [flags] <args>",
		Short: "tools for managing database",
		RunE: createCmdRunFn(cfg, vpr, false, func(cmd *cobra.Command, args []string, logger config.LoggerFunc) error {
			return dbmanager.Cli(cfg.DbManager, cfg.Server, args)

		}),
		Hidden: true,
	}

	cli.PopulateFlagSet(cfg.DbManager, dbmanagerCmd.Flags(), vpr)
	cli.PopulateFlagSet(cfg.Server, dbmanagerCmd.Flags(), vpr, cli.WithSkip("log-level", "storage-path", "metric-export-rules"))
	return dbmanagerCmd
}
