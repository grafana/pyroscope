package command

import (
	"fmt"
	"os"

	"github.com/pyroscope-io/pyroscope/pkg/cli"
	"github.com/pyroscope-io/pyroscope/pkg/config"
	"github.com/pyroscope-io/pyroscope/pkg/exec"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func newConnectCmd(cfg *config.Exec) *cobra.Command {
	connectCmd := &cobra.Command{
		Use:   "connect [flags]",
		Short: "connects to an existing process and profiles it",
		RunE: func(cmd *cobra.Command, args []string) error {
			if cfg.NoLogging {
				logrus.SetLevel(logrus.PanicLevel)
			} else if l, err := logrus.ParseLevel(cfg.LogLevel); err == nil {
				logrus.SetLevel(l)
			}
			if len(args) > 0 && args[0] == "help" {
				fmt.Println(gradientBanner())
				fmt.Println(DefaultUsageFunc(cmd.Flags(), cmd))
			}

			err := exec.Cli(cfg, args)
			if err != nil {
				cmd.SilenceUsage = true
			}

			return err
		},
	}

	cli.PopulateFlagSet(cfg, connectCmd.Flags(), cli.WithSkip("group-name", "user-name", "no-root-drop"))
	viper.BindPFlags(connectCmd.Flags())

	connectCmd.SetUsageFunc(func(cmd *cobra.Command) error {
		fmt.Println(gradientBanner() + "\n" + DefaultUsageFunc(cmd.Flags(), cmd))
		return nil
	})

	if err := viper.Unmarshal(cfg); err != nil {
		fmt.Fprintln(os.Stderr, "Unable to unmarshal:", err)
	}

	return connectCmd
}
