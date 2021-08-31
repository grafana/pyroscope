package command

import (
	"fmt"
	"os"
	"strings"

	"github.com/pyroscope-io/pyroscope/pkg/cli"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"

	"github.com/pyroscope-io/pyroscope/pkg/config"
)

type cmdRunFn func(cmd *cobra.Command, args []string) error

func createCmdRunFn(cfg interface{}, vpr *viper.Viper, fn cmdRunFn) cmdRunFn {
	return func(cmd *cobra.Command, args []string) error {
		var err error
		if err = vpr.BindPFlags(cmd.Flags()); err != nil {
			return err
		}
		if c, ok := cfg.(config.File); ok {
			if err = loadConfigFile(c.Path(), cmd, vpr, cfg); err != nil {
				return fmt.Errorf("loading configuration file: %w", err)
			}
		}
		if err = cli.Unmarshal(vpr, cfg); err != nil {
			return err
		}
		if err = fn(cmd, args); err != nil {
			cmd.SilenceUsage = true
		}
		return err
	}
}

func newViper() *viper.Viper {
	v := viper.New()
	v.SetEnvPrefix("PYROSCOPE")
	v.AutomaticEnv()
	v.SetEnvKeyReplacer(strings.NewReplacer("-", "_", ".", "_"))
	return v
}

func loadConfigFile(path string, cmd *cobra.Command, vpr *viper.Viper, v interface{}) error {
	if path == "" {
		return nil
	}

	vpr.SetConfigFile(path)
	err := vpr.ReadInConfig()
	switch {
	case err == nil:
		return nil
	case isUserDefined(cmd.Flag("config"), vpr):
		// User-defined configuration can not be read.
		return err
	case os.IsNotExist(err):
		// Default configuration file not found.
		return nil
	default:
		return err
	}
}

func isUserDefined(f *pflag.Flag, v *viper.Viper) bool {
	return f.Changed || (f.DefValue != "" && f.DefValue != v.GetString(f.Name))
}
