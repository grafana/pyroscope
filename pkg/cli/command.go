package cli

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"

	"github.com/pyroscope-io/pyroscope/pkg/util/slices"
)

// https://pubs.opengroup.org/onlinepubs/9699919799/basedefs/V1_chap12.html#tag_12_02
const OptionsEnd = "--"

type CmdRunFn func(cmd *cobra.Command, args []string) error

func CreateCmdRunFn(cfg interface{}, vpr *viper.Viper, fn CmdRunFn) CmdRunFn {
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
		if err = cmd.Flags().Parse(xargs); err != nil {
			return err
		}

		// Read configuration from file, if applicable.
		if err = loadConfigFile(cmd, vpr); err != nil {
			return err
		}
		// Viper deals with both environment variable mappings as well as config files.
		// That's why this is not included in the previous if statement
		if err = Unmarshal(vpr, cfg); err != nil {
			return err
		}

		// Parsing arguments one more time to override anything set in environment variables or config file
		if err = cmd.Flags().Parse(xargs); err != nil {
			return err
		}

		if err = fn(cmd, args); err != nil {
			cmd.SilenceUsage = true
		}
		return err
	}
}

func NewViper(prefix string) *viper.Viper {
	v := viper.New()
	v.SetEnvPrefix(prefix)
	v.SetConfigType("yaml")
	v.AutomaticEnv()
	v.SetEnvKeyReplacer(strings.NewReplacer("-", "_", ".", "_"))
	return v
}

// splitArgs splits raw arguments into
func splitArgs(flags *pflag.FlagSet, args []string) ([]string, []string) {
	var xargs []string
	x := firstArgumentIndex(flags, args)
	if x >= 0 {
		xargs = args[:x]
		args = args[x:]
	} else {
		xargs = args
		args = nil
	}
	return args, prependDash(xargs)
}

func prependDash1(arg string) string {
	if len(arg) > 2 && strings.HasPrefix(arg, "-") && !strings.HasPrefix(arg, "--") {
		return "-" + arg
	}
	return arg
}

func prependDash(args []string) []string {
	for i, arg := range args {
		args[i] = prependDash1(arg)
	}
	return args
}

// firstArgumentIndex returns index of the first encountered argument.
// If args does not contain arguments, or contains undefined flags,
// the call returns -1.
func firstArgumentIndex(flags *pflag.FlagSet, args []string) int {
	for i := 0; i < len(args); i++ {
		a := prependDash1(args[i])
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

func loadConfigFile(cmd *cobra.Command, vpr *viper.Viper) error {
	cf := cmd.Flags().Lookup("config")
	if cf == nil {
		return nil
	}
	var configPath string
	configPath = cf.Value.String()
	// Note that Changed is set to true even if the specified flag value
	// is equal to the default one. For backward compatibility we only
	// consider an option as user-defined, if its value is different;
	// which may be unexpected.
	userDefined := cf.Changed && configPath != cf.DefValue
	// If configuration file path is overridden with the environment variable
	// and the flag is not specified, read config by the path from the env var.
	if !userDefined {
		if v := os.Getenv("PYROSCOPE_CONFIG"); v != "" {
			configPath = v
			userDefined = true
		}
	}
	if configPath == "" {
		// Must never happen.
		return nil
	}
	vpr.SetConfigFile(configPath)
	err := vpr.ReadInConfig()
	if err == nil || (errors.Is(err, os.ErrNotExist) && !userDefined) {
		// The default config file can be missing.
		return nil
	}
	return fmt.Errorf("loading configuration file: %w", err)
}
