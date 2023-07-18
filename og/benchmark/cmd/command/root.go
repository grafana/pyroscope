package command

import (
	"fmt"
	"os"
	"runtime"
	"strings"

	"github.com/pyroscope-io/pyroscope/benchmark/internal/config"
	"github.com/pyroscope-io/pyroscope/pkg/cli"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

func newRootCmd(*config.Config) *cobra.Command {
	rootCmd := &cobra.Command{
		Use: "pyrobench [flags] <subcommand>",
	}

	rootCmd.SetUsageFunc(func(cmd *cobra.Command) error {
		fmt.Println(gradientBanner())
		fmt.Println(cli.DefaultUsageFunc(cmd.Flags(), cmd))
		return nil
	})

	rootCmd.SetHelpFunc(func(cmd *cobra.Command, a []string) {
		fmt.Println(gradientBanner())
		fmt.Println(cli.DefaultUsageFunc(cmd.Flags(), cmd))
	})

	return rootCmd
}

// Initialize adds all child commands to the root command and sets flags appropriately
func Initialize() error {
	var cfg config.Config

	rootCmd := newRootCmd(&cfg)
	rootCmd.SilenceErrors = true
	rootCmd.AddCommand(
		newLoadGen(&cfg.LoadGen),
		newPromQuery(&cfg.PromQuery),
		newReport(&cfg.Report),
	)

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
	return rootCmd.Execute()
}
