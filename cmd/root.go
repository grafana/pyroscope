package cmd

import (
	"fmt"
	"os"
	"runtime"
	"strings"

	"github.com/pyroscope-io/pyroscope/pkg/build"
	"github.com/pyroscope-io/pyroscope/pkg/config"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/spf13/viper"
)

var cfg config.Config

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use: "pyroscope [flags] <subcommand>",
	Run: func(cmd *cobra.Command, args []string) {
		if cfg.Version || len(args) > 0 && args[0] == "version" {
			fmt.Println(gradientBanner())
			fmt.Println(build.Summary())
			fmt.Println("")
		} else {
			fmt.Println(gradientBanner())
			fmt.Println(DefaultUsageFunc(cmd.Flags(), cmd))
		}
	},
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	logrus.SetReportCaller(true)
	logrus.SetFormatter(&logrus.TextFormatter{
		TimestampFormat: "2006-01-02T15:04:05.000000",
		FullTimestamp:   true,
		CallerPrettyfier: func(f *runtime.Frame) (string, string) {
			filename := f.File
			if len(filename) > 38 {
				filename = filename[38:]
			}
			return "", fmt.Sprintf(" %s:%d", filename, f.Line)
		},
	})

	args := os.Args[1:]
	for i, arg := range args {
		if len(arg) > 2 && strings.HasPrefix(arg, "-") && !strings.HasPrefix(arg, "--") {
			args[i] = fmt.Sprintf("-%s", arg)
		}
	}

	rootCmd.SetArgs(args)
	rootCmd.Execute()
}

func init() {
	viper.SetEnvPrefix("PYROSCOPE")
	viper.AutomaticEnv() // read in environment variables that match
	replacer := strings.NewReplacer("-", "_", ".", "_")
	viper.SetEnvKeyReplacer(replacer)

	rootCmd.SetUsageFunc(func(cmd *cobra.Command) error {
		fmt.Println(gradientBanner() + "\n" + DefaultUsageFunc(cmd.Flags(), cmd))
		return nil
	})
}
