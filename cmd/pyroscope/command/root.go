package command

import (
	"fmt"
	"os"
	"runtime"
	"strings"

	"github.com/pyroscope-io/pyroscope/pkg/build"
	"github.com/pyroscope-io/pyroscope/pkg/config"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

func newRootCmd(cfg *config.Config) *cobra.Command {
	rootCmd := &cobra.Command{
		Use: "pyroscope [flags] <subcommand>",
		Run: func(cmd *cobra.Command, args []string) {
			if cfg.Version {
				fmt.Println(gradientBanner())
				fmt.Println(build.Summary())
				fmt.Println("")
			} else {
				fmt.Println(gradientBanner())
				fmt.Println(DefaultUsageFunc(cmd.Flags(), cmd))
			}
		},
	}

	rootCmd.SetUsageFunc(func(cmd *cobra.Command) error {
		fmt.Println(gradientBanner())
		fmt.Println(DefaultUsageFunc(cmd.Flags(), cmd))
		return nil
	})

	rootCmd.SetHelpFunc(func(cmd *cobra.Command, a []string) {
		fmt.Println(gradientBanner())
		fmt.Println(DefaultUsageFunc(cmd.Flags(), cmd))
	})

	rootCmd.PersistentFlags().BoolVarP(&cfg.Version, "version", "v", false, "print pyroscope version details")
	return rootCmd
}

// Initialize adds all child commands to the root command and sets flags appropriately
func Initialize() error {
	var cfg config.Config

	rootCmd := newRootCmd(&cfg)
	rootCmd.SilenceErrors = true
	rootCmd.AddCommand(
		newAgentCmd(&cfg.Agent),
		newConnectCmd(&cfg.Exec),
		newConvertCmd(&cfg.Convert),
		newDbManagerCmd(&config.CombinedDbManager{DbManager: &cfg.DbManager, Server: &cfg.Server}),
		newExecCmd(&cfg.Exec),
		newServerCmd(&cfg.Server),
		newVersionCmd(),
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
