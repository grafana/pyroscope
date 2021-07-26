package cmd

import (
	"fmt"

	"github.com/pyroscope-io/pyroscope/pkg/cli"
	"github.com/pyroscope-io/pyroscope/pkg/exec"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// connectCmd represents the connect command
var connectCmd = &cobra.Command{
	Use:   "connect [flags]",
	Short: "connects to an existing process and profiles it",
	RunE: func(cmd *cobra.Command, args []string) error {
		if cfg.Exec.NoLogging {
			logrus.SetLevel(logrus.PanicLevel)
		} else if l, err := logrus.ParseLevel(cfg.Exec.LogLevel); err == nil {
			logrus.SetLevel(l)
		}
		if len(args) > 0 && args[0] == "help" {
			fmt.Println(gradientBanner())
			fmt.Println(DefaultUsageFunc(cmd.Flags(), cmd))
		}

		return exec.Cli(&cfg.Exec, args)
	},
}

func init() {
	rootCmd.AddCommand(connectCmd)

	cli.PopulateFlagSet(&cfg.Exec, connectCmd.Flags())
	viper.BindPFlags(connectCmd.Flags())

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// connectCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// connectCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")

	connectCmd.SetUsageFunc(func(cmd *cobra.Command) error {
		fmt.Println(gradientBanner() + "\n" + DefaultUsageFunc(cmd.Flags(), cmd))
		return nil
	})
}
