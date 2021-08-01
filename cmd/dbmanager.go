package cmd

import (
	"fmt"
	"os"

	"github.com/pyroscope-io/pyroscope/pkg/cli"
	"github.com/pyroscope-io/pyroscope/pkg/dbmanager"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// dbmanagerCmd represents the dbmanager command
var dbmanagerCmd = &cobra.Command{
	Use:   "dbmanager [flags] <args>",
	Short: "tools for managing database",
	RunE: func(cmd *cobra.Command, args []string) error {
		if l, err := logrus.ParseLevel(cfg.DbManager.LogLevel); err == nil {
			logrus.SetLevel(l)
		}

		return dbmanager.Cli(&cfg.DbManager, &cfg.Server, args)
	},
}

func init() {
	rootCmd.AddCommand(dbmanagerCmd)

	cli.PopulateFlagSet(&cfg.DbManager, dbmanagerCmd.Flags(), cli.WithSkip("group-name", "user-name", "no-root-drop"))
	viper.BindPFlags(dbmanagerCmd.Flags())

	dbmanagerCmd.SetUsageFunc(func(cmd *cobra.Command) error {
		fmt.Println(gradientBanner() + "\n" + DefaultUsageFunc(cmd.Flags(), cmd))
		return nil
	})

	if err := viper.Unmarshal(&cfg.DbManager); err != nil {
		fmt.Fprintln(os.Stderr, "Unable to unmarshal:", err)
	}
}
