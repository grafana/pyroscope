package command

import (
	"fmt"
	"os"
	"reflect"
	"strings"

	goexec "os/exec"

	"github.com/mitchellh/mapstructure"
	"github.com/pyroscope-io/pyroscope/pkg/config"
	"github.com/pyroscope-io/pyroscope/pkg/util/bytesize"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

type cmdRunFn func(cmd *cobra.Command, args []string) error
type cmdStartFn func(cmd *cobra.Command, args []string, logger config.LoggerFunc) error

func createCmdRunFn(cfg interface{}, vpr *viper.Viper, requiresArgs bool, fn cmdStartFn) cmdRunFn {
	return func(cmd *cobra.Command, args []string) error {
		var err error
		if err = bindFlags(cfg, cmd, vpr); err != nil {
			return fmt.Errorf("invalid configuration: %w", err)
		}

		var logger func(s string)
		if l, ok := cfg.(config.LoggerConfiger); ok {
			logger = l.InitializeLogging()
		}

		if c, ok := cfg.(config.FileConfiger); ok {
			if err = loadConfigFile(c.ConfigFilePath(), cmd, vpr, cfg); err != nil {
				return fmt.Errorf("loading configuration file: %w", err)
			}
		}

		if (requiresArgs && len(args) == 0) || (len(args) > 0 && args[0] == "help") {
			_ = cmd.Help()
			return nil
		}

		if err = fn(cmd, args, logger); err != nil {
			cmd.SilenceUsage = true
		}

		// Normally, if the program ran, the call should return ExitError and
		// the exit code must be preserved. Otherwise, the error originates from
		// pyroscope and will be printed.
		if e, ok := err.(*goexec.ExitError); ok {
			os.Exit(e.ExitCode())
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
