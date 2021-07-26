package cmd

import (
	"fmt"

	"github.com/pyroscope-io/pyroscope/pkg/cli"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// agentCmd represents the agent command
var agentCmd = &cobra.Command{
	Use:   "agent [flags]",
	Short: "starts pyroscope agent.",
	RunE: func(cmd *cobra.Command, args []string) error {
		return cli.StartAgent(&cfg.Agent)
	},
}

func init() {
	rootCmd.AddCommand(agentCmd)

	cli.PopulateFlagSet(&cfg.Agent, agentCmd.Flags(), cli.WithSkip("targets"))
	viper.BindPFlags(agentCmd.Flags())

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// agentCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// agentCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
	agentCmd.SetUsageFunc(func(cmd *cobra.Command) error {
		fmt.Println(gradientBanner() + "\n" + DefaultUsageFunc(cmd.Flags(), cmd))
		return nil
	})
}
