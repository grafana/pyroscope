package command

import (
	"github.com/pyroscope-io/pyroscope/pkg/cli"
	"github.com/pyroscope-io/pyroscope/pkg/config"
	"github.com/pyroscope-io/pyroscope/pkg/dbmanager"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

func newDbManagerCmd(dbManagerCfg *config.DbManager, serverCfg *config.Server) *cobra.Command {
	dbmanagerCmd := &cobra.Command{
		Use:   "dbmanager [flags] <args>",
		Short: "tools for managing database",
		RunE: func(cmd *cobra.Command, args []string) error {
			if l, err := logrus.ParseLevel(dbManagerCfg.LogLevel); err == nil {
				logrus.SetLevel(l)
			}

			err := dbmanager.Cli(dbManagerCfg, serverCfg, args)
			if err != nil {
				// do not print usage in case of an error while running the subcommand
				cmd.SilenceUsage = true
			}

			return err
		},
	}

	loadFlags(dbManagerCfg, dbmanagerCmd, cli.WithSkip("group-name", "user-name", "no-root-drop"))
	return dbmanagerCmd
}
