package cli

import (
	"context"
	"flag"
	"fmt"
	"os"
	goexec "os/exec"
	"runtime"

	"github.com/peterbourgon/ff/v3"
	"github.com/peterbourgon/ff/v3/ffcli"
	"github.com/sirupsen/logrus"

	"github.com/pyroscope-io/pyroscope/pkg/build"
	"github.com/pyroscope-io/pyroscope/pkg/config"
	"github.com/pyroscope-io/pyroscope/pkg/convert"
	"github.com/pyroscope-io/pyroscope/pkg/dbmanager"
	"github.com/pyroscope-io/pyroscope/pkg/exec"
)

func generateRootCmd(cfg *config.Config) *ffcli.Command {
	// init the log formatter for logrus
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

	var (
		serverFlagSet    = flag.NewFlagSet("pyroscope server", flag.ExitOnError)
		agentFlagSet     = flag.NewFlagSet("pyroscope agent", flag.ExitOnError)
		convertFlagSet   = flag.NewFlagSet("pyroscope convert", flag.ExitOnError)
		execFlagSet      = flag.NewFlagSet("pyroscope exec", flag.ExitOnError)
		connectFlagSet   = flag.NewFlagSet("pyroscope connect", flag.ExitOnError)
		dbmanagerFlagSet = flag.NewFlagSet("pyroscope dbmanager", flag.ExitOnError)
		rootFlagSet      = flag.NewFlagSet("pyroscope", flag.ExitOnError)
	)

	serverSortedFlags := PopulateFlagSet(&cfg.Server, serverFlagSet, WithSkip("metric-export-rules"))
	agentSortedFlags := PopulateFlagSet(&cfg.Agent, agentFlagSet, WithSkip("targets"))
	convertSortedFlags := PopulateFlagSet(&cfg.Convert, convertFlagSet)
	execSortedFlags := PopulateFlagSet(&cfg.Exec, execFlagSet, WithSkip("pid"))
	connectSortedFlags := PopulateFlagSet(&cfg.Exec, connectFlagSet, WithSkip("group-name", "user-name", "no-root-drop"))
	dbmanagerSortedFlags := PopulateFlagSet(&cfg.DbManager, dbmanagerFlagSet)
	rootSortedFlags := PopulateFlagSet(cfg, rootFlagSet)

	options := []ff.Option{
		ff.WithConfigFileParser(parser),
		ff.WithEnvVarPrefix("PYROSCOPE"),
		ff.WithAllowMissingConfigFile(true),
		ff.WithConfigFileFlag("config"),
	}

	serverCmd := &ffcli.Command{
		UsageFunc:  serverSortedFlags.printUsage,
		Options:    append(options, ff.WithIgnoreUndefined(true)),
		Name:       "server",
		ShortUsage: "pyroscope server [flags]",
		ShortHelp:  "starts pyroscope server. This is the database + web-based user interface",
		FlagSet:    serverFlagSet,
	}

	agentCmd := &ffcli.Command{
		UsageFunc:  agentSortedFlags.printUsage,
		Options:    append(options, ff.WithIgnoreUndefined(true)),
		Name:       "agent",
		ShortUsage: "pyroscope agent [flags]",
		ShortHelp:  "starts pyroscope agent.",
		FlagSet:    agentFlagSet,
	}

	convertCmd := &ffcli.Command{
		UsageFunc:  convertSortedFlags.printUsage,
		Options:    options,
		Name:       "convert",
		ShortUsage: "pyroscope convert [flags] <input-file>",
		ShortHelp:  "converts between different profiling formats",
		FlagSet:    convertFlagSet,
	}

	execCmd := &ffcli.Command{
		UsageFunc:  execSortedFlags.printUsage,
		Options:    options,
		Name:       "exec",
		ShortUsage: "pyroscope exec [flags] <args>",
		ShortHelp:  "starts a new process from <args> and profiles it",
		FlagSet:    execFlagSet,
	}

	connectCmd := &ffcli.Command{
		UsageFunc:  connectSortedFlags.printUsage,
		Options:    options,
		Name:       "connect",
		ShortUsage: "pyroscope connect [flags]",
		ShortHelp:  "connects to an existing process and profiles it",
		FlagSet:    connectFlagSet,
	}

	dbmanagerCmd := &ffcli.Command{
		UsageFunc:  dbmanagerSortedFlags.printUsage,
		Options:    options,
		Name:       "dbmanager",
		ShortUsage: "pyroscope dbmanager [flags] <args>",
		ShortHelp:  "tools for managing database",
		FlagSet:    dbmanagerFlagSet,
	}

	serverCmd.Exec = func(ctx context.Context, args []string) error {
		return startServer(&cfg.Server)
	}

	agentCmd.Exec = func(ctx context.Context, args []string) error {
		return startAgent(&cfg.Agent)
	}

	convertCmd.Exec = func(ctx context.Context, args []string) error {
		logrus.SetOutput(os.Stderr)
		logger := func(s string) {
			logrus.Fatal(s)
		}
		return convert.Cli(&cfg.Convert, logger, args)
	}

	execCmd.Exec = func(_ context.Context, args []string) error {
		if cfg.Exec.NoLogging {
			logrus.SetLevel(logrus.PanicLevel)
		} else if l, err := logrus.ParseLevel(cfg.Exec.LogLevel); err == nil {
			logrus.SetLevel(l)
		}
		if len(args) == 0 || args[0] == "help" {
			fmt.Println(gradientBanner())
			fmt.Println(DefaultUsageFunc(execSortedFlags, execCmd, []string{}))
			return nil
		}
		err := exec.Cli(&cfg.Exec, args)
		// Normally, if the program ran, the call should return ExitError and
		// the exit code must be preserved. Otherwise, the error originates from
		// pyroscope and will be printed.
		if e, ok := err.(*goexec.ExitError); ok {
			os.Exit(e.ExitCode())
		}

		return err
	}

	connectCmd.Exec = func(ctx context.Context, args []string) error {
		if cfg.Exec.NoLogging {
			logrus.SetLevel(logrus.PanicLevel)
		} else if l, err := logrus.ParseLevel(cfg.Exec.LogLevel); err == nil {
			logrus.SetLevel(l)
		}
		if len(args) > 0 && args[0] == "help" {
			fmt.Println(gradientBanner())
			fmt.Println(DefaultUsageFunc(connectSortedFlags, connectCmd, []string{}))
			return nil
		}
		return exec.Cli(&cfg.Exec, args)
	}

	dbmanagerCmd.Exec = func(ctx context.Context, args []string) error {
		if l, err := logrus.ParseLevel(cfg.DbManager.LogLevel); err == nil {
			logrus.SetLevel(l)
		}
		return dbmanager.Cli(&cfg.DbManager, &cfg.Server, args)
	}

	rootCmd := &ffcli.Command{
		UsageFunc:  rootSortedFlags.printUsage,
		Options:    options,
		ShortUsage: "pyroscope [flags] <subcommand>",
		FlagSet:    rootFlagSet,
		Subcommands: []*ffcli.Command{
			convertCmd,
			serverCmd,
			agentCmd,
			execCmd,
			connectCmd,
			dbmanagerCmd,
		},
	}

	rootCmd.Exec = func(ctx context.Context, args []string) error {
		if cfg.Version || len(args) > 0 && args[0] == "version" {
			fmt.Println(gradientBanner())
			fmt.Println(build.Summary())
			fmt.Println("")
		} else {
			fmt.Println(gradientBanner())
			fmt.Println(DefaultUsageFunc(rootSortedFlags, rootCmd, []string{}))
		}
		return nil
	}

	return rootCmd
}

func Start(cfg *config.Config) error {
	return generateRootCmd(cfg).ParseAndRun(context.Background(), os.Args[1:])
}
