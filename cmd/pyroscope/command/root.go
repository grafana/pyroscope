package command

import (
	"fmt"
	"runtime"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/pyroscope-io/pyroscope/pkg/cli"
	"github.com/pyroscope-io/pyroscope/pkg/config"
)

func Execute() error {
	var cfg config.Config
	rootCmd := newRootCmd(&cfg)
	rootCmd.SilenceErrors = true

	subcommands := []*cobra.Command{
		newAgentCmd(&cfg.Agent),
		newConnectCmd(&cfg.Exec),
		newConvertCmd(&cfg.Convert),
		newDbManagerCmd(&config.CombinedDbManager{DbManager: &cfg.DbManager, Server: &cfg.Server}),
		newExecCmd(&cfg.Exec),
		newServerCmd(&cfg.Server),
		newVersionCmd(),
		completionCmd,
		newAdminCmd(&cfg.Admin),
	}

	for _, c := range subcommands {
		addHelpSubcommand(c)
		c.HasHelpSubCommands()
		rootCmd.AddCommand(c)
	}

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

	return rootCmd.Execute()
}

func newRootCmd(cfg *config.Config) *cobra.Command {
	vpr := newViper()
	rootCmd := &cobra.Command{
		Use: "pyroscope [flags] <subcommand>",
		Run: func(cmd *cobra.Command, _ []string) {
			if cfg.Version {
				printVersion(cmd)
			} else {
				printHelpMessage(cmd, nil)
			}
		},
	}

	rootCmd.SetUsageFunc(printUsageMessage)
	rootCmd.SetHelpFunc(printHelpMessage)
	cli.PopulateFlagSet(cfg, rootCmd.Flags(), vpr)
	return rootCmd
}

func printUsageMessage(cmd *cobra.Command) error {
	printHelpMessage(cmd, nil)
	return nil
}

func printHelpMessage(cmd *cobra.Command, _ []string) {
	cmd.Println(gradientBanner())
	cmd.Println(cli.DefaultUsageFunc(cmd.Flags(), cmd))
}

func addHelpSubcommand(cmd *cobra.Command) {
	cmd.AddCommand(&cobra.Command{
		Use: "help",
		Run: func(_ *cobra.Command, _ []string) {
			printHelpMessage(cmd, nil)
		},
	})
}
