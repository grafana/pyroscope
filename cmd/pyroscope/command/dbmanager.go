package command

import (
	"fmt"
	"os"

	"github.com/pyroscope-io/pyroscope/pkg/cli"
	"github.com/pyroscope-io/pyroscope/pkg/config"
	"github.com/pyroscope-io/pyroscope/pkg/dbmanager"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
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
				cmd.SilenceUsage = true
			}

			return err
		},
	}

	cli.PopulateFlagSet(dbManagerCfg, dbmanagerCmd.Flags(), cli.WithSkip("group-name", "user-name", "no-root-drop"))
	viper.BindPFlags(dbmanagerCmd.Flags())

	dbmanagerCmd.SetUsageFunc(func(cmd *cobra.Command) error {
		fmt.Println(gradientBanner() + "\n" + DefaultUsageFunc(cmd.Flags(), cmd))
		return nil
	})

	if err := viper.Unmarshal(dbManagerCfg); err != nil {
		fmt.Fprintln(os.Stderr, "Unable to unmarshal:", err)
	}

	return dbmanagerCmd
}
