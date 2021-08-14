package command

import (
	"fmt"
	"os"

	"github.com/pyroscope-io/pyroscope/pkg/cli"
	"github.com/pyroscope-io/pyroscope/pkg/config"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func newServerCmd(cfg *config.Server) *cobra.Command {
	serverCmd := &cobra.Command{
		Use:   "server [flags]",
		Short: "starts pyroscope server. This is the database + web-based user interface",
		RunE: func(cmd *cobra.Command, args []string) error {
			if cfg.Config != "" {
				// Use config file from the flag.
				viper.SetConfigFile(cfg.Config)

				// If a config file is found, read it in.
				if err := viper.ReadInConfig(); err == nil {
					fmt.Fprintln(os.Stderr, "Using config file:", viper.ConfigFileUsed())
				}

				if err := viper.Unmarshal(cfg); err != nil {
					fmt.Fprintln(os.Stderr, "Unable to unmarshal:", err)
				}

			}

			err := cli.StartServer(cfg)
			if err != nil {
				cmd.SilenceUsage = true
			}

			return err
		},
	}

	cli.PopulateFlagSet(cfg, serverCmd.Flags(), cli.WithSkip("metric-export-rules"))
	viper.BindPFlags(serverCmd.Flags())

	serverCmd.SetUsageFunc(func(cmd *cobra.Command) error {
		fmt.Println(gradientBanner() + "\n" + DefaultUsageFunc(cmd.Flags(), cmd))
		return nil
	})

	if err := viper.Unmarshal(cfg); err != nil {
		fmt.Fprintln(os.Stderr, "Unable to unmarshal:", err)
	}

	return serverCmd
}
