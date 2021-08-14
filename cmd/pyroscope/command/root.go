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
	"github.com/spf13/viper"
)

func newRootCmd(cfg *config.Config) *cobra.Command {
	rootCmd := &cobra.Command{
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

	rootCmd.SetUsageFunc(func(cmd *cobra.Command) error {
		fmt.Println(gradientBanner() + "\n" + DefaultUsageFunc(cmd.Flags(), cmd))
		return nil
	})

	return rootCmd
}

// Initialize adds all child commands to the root command and sets flags appropriately
func Initialize() {
	var cfg config.Config

	viper.SetEnvPrefix("PYROSCOPE")
	viper.AutomaticEnv() // read in environment variables that match
	replacer := strings.NewReplacer("-", "_", ".", "_")
	viper.SetEnvKeyReplacer(replacer)

	rootCmd := newRootCmd(&cfg)
	agentCmd := newAgentCmd(&cfg.Agent)
	connectCmd := newConnectCmd(&cfg.Exec)
	convertCmd := newConvertCmd(&cfg.Convert)
	dbManagerCmd := newDbManagerCmd(&cfg.DbManager, &cfg.Server)
	execCmd := newExecCmd(&cfg.Exec)
	serverCmd := newServerCmd(&cfg.Server)

	rootCmd.AddCommand(agentCmd)
	rootCmd.AddCommand(connectCmd)
	rootCmd.AddCommand(convertCmd)
	rootCmd.AddCommand(dbManagerCmd)
	rootCmd.AddCommand(execCmd)
	rootCmd.AddCommand(serverCmd)

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
