package cmd

import (
	"fmt"

	"github.com/pyroscope-io/pyroscope/pkg/dbmanager"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

// dbmanagerCmd represents the dbmanager command
var dbmanagerCmd = &cobra.Command{
	Use:   "pyroscope dbmanager [flags] <args>",
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

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// dbmanagerCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// dbmanagerCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
	dbmanagerCmd.SetUsageFunc(func(cmd *cobra.Command) error {
		fmt.Println(gradientBanner() + "\n" + DefaultUsageFunc(cmd.Flags(), cmd))
		return nil
	})
}
