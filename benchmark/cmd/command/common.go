// Copied as is from github.com/pyroscope-io/pyroscope/cmd/command/common.go
package command

import (
	"fmt"
	"os"
	"reflect"
	"strings"

	"github.com/mitchellh/mapstructure"
	"github.com/pyroscope-io/pyroscope/benchmark/config"
	"github.com/pyroscope-io/pyroscope/pkg/cli"
	"github.com/pyroscope-io/pyroscope/pkg/util/bytesize"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
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

func bindFlags(cfg interface{}, cmd *cobra.Command, vpr *viper.Viper) error {
	if err := vpr.BindPFlags(cmd.Flags()); err != nil {
		return err
	}
	return viperUnmarshalWithBytesHook(vpr, cfg)
}

func newViper() *viper.Viper {
	v := viper.New()
	v.SetEnvPrefix("PYROBENCH")
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
		return viperUnmarshalWithBytesHook(vpr, v)
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

func viperUnmarshalWithBytesHook(vpr *viper.Viper, cfg interface{}) error {
	return vpr.Unmarshal(cfg, viper.DecodeHook(
		mapstructure.ComposeDecodeHookFunc(
			// Function to add a special type for «env. mode»
			func(f reflect.Type, t reflect.Type, data interface{}) (interface{}, error) {
				if t != reflect.TypeOf(bytesize.Byte) {
					return data, nil
				}

				stringData, ok := data.(string)
				if !ok {
					return data, nil
				}

				return bytesize.Parse(stringData)
			},
			// Function to support net.IP
			mapstructure.StringToIPHookFunc(),
			// Appended by the two default functions
			mapstructure.StringToTimeDurationHookFunc(),
			mapstructure.StringToSliceHookFunc(","),
		),
	))
}

func isUserDefined(f *pflag.Flag, v *viper.Viper) bool {
	return f.Changed || (f.DefValue != "" && f.DefValue != v.GetString(f.Name))
}
