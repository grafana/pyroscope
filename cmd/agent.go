package cmd

import (
	"fmt"
	"os"

	"github.com/pyroscope-io/pyroscope/pkg/cli"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// agentCmd represents the agent command
var agentCmd = &cobra.Command{
	Use:   "agent [flags]",
	Short: "starts pyroscope agent.",
	RunE: func(cmd *cobra.Command, args []string) error {
		if cfg.Agent.Config != "" {
			// Use config file from the flag.
			viper.SetConfigFile(cfg.Agent.Config)

			// If a config file is found, read it in.
			if err := viper.ReadInConfig(); err == nil {
				fmt.Fprintln(os.Stderr, "Using config file:", viper.ConfigFileUsed())
			}

			if err := viper.Unmarshal(&cfg.Agent); err != nil {
				fmt.Fprintln(os.Stderr, "Unable to unmarshal:", err)
			}
		}

		return cli.StartAgent(&cfg.Agent)
	},
}

func init() {
	rootCmd.AddCommand(agentCmd)

	cli.PopulateFlagSet(&cfg.Agent, agentCmd.Flags(), cli.WithSkip("targets"))
	viper.BindPFlags(agentCmd.Flags())

	agentCmd.SetUsageFunc(func(cmd *cobra.Command) error {
		fmt.Println(gradientBanner() + "\n" + DefaultUsageFunc(cmd.Flags(), cmd))
		return nil
	})

	if err := viper.Unmarshal(&cfg.Agent); err != nil {
		fmt.Fprintln(os.Stderr, "Unable to unmarshal:", err)
	}
}
