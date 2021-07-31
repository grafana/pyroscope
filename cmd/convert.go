package cmd

import (
	"fmt"
	"os"

	"github.com/pyroscope-io/pyroscope/pkg/cli"
	"github.com/pyroscope-io/pyroscope/pkg/convert"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// convertCmd represents the convert command
var convertCmd = &cobra.Command{
	Use:   "convert [flags] <input-file>",
	Short: "converts between different profiling formats",
	RunE: func(cmd *cobra.Command, args []string) error {
		logrus.SetOutput(os.Stderr)
		logger := func(s string) {
			logrus.Fatal(s)
		}

		err := convert.Cli(&cfg.Convert, logger, args)
		return err
	},
}

func init() {
	rootCmd.AddCommand(convertCmd)

	cli.PopulateFlagSet(&cfg.Convert, convertCmd.Flags())
	viper.BindPFlags(convertCmd.Flags())

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// convertCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// convertCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
	convertCmd.SetUsageFunc(func(cmd *cobra.Command) error {
		fmt.Println(gradientBanner() + "\n" + DefaultUsageFunc(cmd.Flags(), cmd))
		return nil
	})

	if err := viper.Unmarshal(&cfg.Convert); err != nil {
		fmt.Fprintln(os.Stderr, "Unable to unmarshal:", err)
	}
}
