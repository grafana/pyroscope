package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"

	"github.com/pyroscope-io/pyroscope/pkg/config"
	"github.com/pyroscope-io/pyroscope/pkg/util/slices"
)

// https://pubs.opengroup.org/onlinepubs/9699919799/basedefs/V1_chap12.html#tag_12_02
const OptionsEnd = "--"

type cmdRunFn func(cmd *cobra.Command, args []string) error

func CreateCmdRunFn(cfg interface{}, vpr *viper.Viper, fn cmdRunFn) cmdRunFn {
	return func(cmd *cobra.Command, args []string) error {
		var err error
		var xargs []string

		args, xargs = splitArgs(cmd.Flags(), args)
		if slices.StringContains(xargs, "--help") {
			_ = cmd.Help()
			return nil
		}

		if err = vpr.BindPFlags(cmd.Flags()); err != nil {
			return err
		}

		// Here's the correct order for configuration precedence:
		// * command line arguments
		// * environment variables
		// * config file
		// * defaults
		// also documented here: https://pyroscope.io/docs/server-configuration

		// Parsing arguments for the first time.
		// The only reason we do this here is so that if you provide -config argument we use the right config path
		if err = cmd.Flags().Parse(prependDash(xargs)); err != nil {
			return err
		}

		// some subcommands don't have config files, so we use this File interface
		// TODO: maybe should we rename this interface? `File` is too generic and confusing imo
		if c, ok := cfg.(config.File); ok {
			configPath := os.Getenv("PYROSCOPE_CONFIG")
			if cmd.Flags().Lookup("config").Changed {
				configPath = c.Path()
			}
			if err = loadConfigFile(configPath, cmd, vpr, cfg); err != nil {
				return fmt.Errorf("loading configuration file: %w", err)
			}
		}
		// Viper deals with both environment variable mappings as well as config files.
		// That's why this is not included in the previous if statement
		if err = Unmarshal(vpr, cfg); err != nil {
			return err
		}

		// Parsing arguments one more time to override anything set in environment variables or config file
		if err = cmd.Flags().Parse(prependDash(xargs)); err != nil {
			return err
		}

		if err = fn(cmd, args); err != nil {
			cmd.SilenceUsage = true
		}
		return err
	}
}

// splitArgs splits raw arguments into
func splitArgs(flags *pflag.FlagSet, args []string) ([]string, []string) {
	var xargs []string
	x := firstArgumentIndex(flags, prependDash(args))
	if x >= 0 {
		xargs = args[:x]
		args = args[x:]
	} else {
		xargs = args
		args = nil
	}
	return args, xargs
}

func prependDash(args []string) []string {
	for i, arg := range args {
		if len(arg) > 2 && strings.HasPrefix(arg, "-") && !strings.HasPrefix(arg, "--") {
			args[i] = "-" + arg
		}
	}
	return args
}

// firstArgumentIndex returns index of the first encountered argument.
// If args does not contain arguments, or contains undefined flags,
// the call returns -1.
func firstArgumentIndex(flags *pflag.FlagSet, args []string) int {
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		default:
			return i
		case a == OptionsEnd:
			return i + 1
		case strings.HasPrefix(a, OptionsEnd) && len(a) > 2:
			x := strings.SplitN(a[2:], "=", 2)
			f := flags.Lookup(x[0])
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
	}
	// Should have returned earlier.
	return -1
}

func NewViper(prefix string) *viper.Viper {
	v := viper.New()
	v.SetEnvPrefix(prefix)
	v.SetConfigType("yaml")
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
		// if it's default value and file doesn't exist that's okay
		// we can't use Flag("config").Changed because it might be set via an env variable
		if cmd.Flag("config").DefValue == path {
			return nil
		}
		// if user set a custom file name and file doesn't exist that's not okay
		return err
	default:
		return err
	}
}

func isUserDefined(f *pflag.Flag, v *viper.Viper) bool {
	return f.Changed || (f.DefValue != "" && f.DefValue != v.GetString(f.Name))
}
