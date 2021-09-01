package command

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"

	"github.com/pyroscope-io/pyroscope/pkg/cli"
	"github.com/pyroscope-io/pyroscope/pkg/config"
	"github.com/pyroscope-io/pyroscope/pkg/util/slices"
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

		var xargs []string
		x := firstArgumentIndex(cmd.Flags(), prependDash(args))
		if x >= 0 {
			xargs = args[:x]
			args = args[x:]
		} else {
			xargs = args
			args = nil
		}
		if err = cmd.Flags().Parse(prependDash(xargs)); err != nil {
			return err
		}
		if slices.StringContains(xargs, "--help") {
			_ = cmd.Help()
			return nil
		}

		if err = fn(cmd, args); err != nil {
			cmd.SilenceUsage = true
		}
		return err
	}
}

func prependDash(args []string) []string {
	for i, arg := range args {
		if len(arg) > 2 && strings.HasPrefix(arg, "-") && !strings.HasPrefix(arg, "--") {
			args[i] = "-" + arg
		}
	}
	return args
}

func firstArgumentIndex(flags *pflag.FlagSet, args []string) int {
	for i := 0; i < len(args); i++ {
		a := args[i]
		var x []string
		var f *pflag.Flag
		switch {
		default:
			return i
		case a == "--":
			return i + 1
		case strings.HasPrefix(a, "--") && len(a) > 2:
			x = strings.SplitN(a[2:], "=", 2)
			f = flags.Lookup(x[0])
		case strings.HasPrefix(a, "-") && len(a) > 1:
			x = strings.SplitN(a[1:], "=", 1)
			f = flags.ShorthandLookup(x[0])
		}
		if f == nil {
			return -1
		}
		if f.Value.Type() == "bool" {
			continue
		}
		if len(x) == 1 {
			i++
		}
	}

	// Should have returned earlier.
	return -1
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
