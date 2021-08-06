package cmd

import (
	"fmt"
	"os"

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

		err := exec.Cli(&cfg.Exec, args)
		if err != nil {
			cmd.SilenceUsage = true
		}

		return err
	},
}

func init() {
	rootCmd.AddCommand(connectCmd)

	cli.PopulateFlagSet(&cfg.Exec, connectCmd.Flags(), cli.WithSkip("group-name", "user-name", "no-root-drop"))
	viper.BindPFlags(connectCmd.Flags())

	connectCmd.SetUsageFunc(func(cmd *cobra.Command) error {
		fmt.Println(gradientBanner() + "\n" + DefaultUsageFunc(cmd.Flags(), cmd))
		return nil
	})

	if err := viper.Unmarshal(&cfg.Exec); err != nil {
		fmt.Fprintln(os.Stderr, "Unable to unmarshal:", err)
	}
}
