package command

import (
	"fmt"
	"os"

	"github.com/pyroscope-io/pyroscope/pkg/cli"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func loadConfig(path string, v interface{}) error {
	if path == "" {
		return nil
	}

	viper.SetConfigFile(path)
	if err := viper.ReadInConfig(); err != nil {
		return err
	}

	return viper.Unmarshal(v)
}

func loadFlags(cfg interface{}, cmd *cobra.Command, opts ...cli.FlagOption) {
	cli.PopulateFlagSet(cfg, cmd.Flags(), opts...)
	viper.BindPFlags(cmd.Flags())

	if err := viper.Unmarshal(cfg); err != nil {
		fmt.Fprintln(os.Stderr, "Unable to unmarshal:", err)
	}
}
