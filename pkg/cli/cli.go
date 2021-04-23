package cli

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/pyroscope-io/pyroscope/pkg/agent"
	"github.com/pyroscope-io/pyroscope/pkg/agent/spy"
	"github.com/pyroscope-io/pyroscope/pkg/agent/upstream/direct"
	"github.com/pyroscope-io/pyroscope/pkg/analytics"
	"github.com/pyroscope-io/pyroscope/pkg/build"
	"github.com/pyroscope-io/pyroscope/pkg/config"
	"github.com/pyroscope-io/pyroscope/pkg/convert"
	"github.com/pyroscope-io/pyroscope/pkg/dbmanager"
	"github.com/pyroscope-io/pyroscope/pkg/exec"
	"github.com/pyroscope-io/pyroscope/pkg/server"
	"github.com/pyroscope-io/pyroscope/pkg/storage"
	"github.com/pyroscope-io/pyroscope/pkg/util/atexit"
	"github.com/pyroscope-io/pyroscope/pkg/util/debug"
	"github.com/pyroscope-io/pyroscope/pkg/util/slices"
	"github.com/sirupsen/logrus"

	"github.com/iancoleman/strcase"
	"github.com/peterbourgon/ff/ffyaml"
	"github.com/peterbourgon/ff/v3"
	"github.com/peterbourgon/ff/v3/ffcli"
)

const timeFormat = "2006-01-02T15:04:05Z0700"

type arrayFlags []string

func (i *arrayFlags) String() string {
	return strings.Join(*i, ", ")
}

func (i *arrayFlags) Set(value string) error {
	*i = append(*i, value)
	return nil
}

type timeFlag time.Time

func (tf *timeFlag) String() string {
	v := time.Time(*tf)
	return v.Format(timeFormat)
}

func (tf *timeFlag) Set(value string) error {
	t2, err := time.Parse(timeFormat, value)
	if err != nil {
		var i int
		i, err = strconv.Atoi(value)
		if err != nil {
			return err
		}
		t2 = time.Unix(int64(i), 0)
	}

	t := (*time.Time)(tf)
	b, _ := t2.MarshalBinary()
	t.UnmarshalBinary(b)

	return nil
}

// this is mostly reflection magic
func PopulateFlagSet(obj interface{}, flagSet *flag.FlagSet, skip ...string) *SortedFlags {
	v := reflect.ValueOf(obj).Elem()
	t := reflect.TypeOf(v.Interface())
	num := t.NumField()

	installPrefix := getInstallPrefix()
	supportedSpies := strings.Join(spy.SupportedExecSpies(), ", ")

	for i := 0; i < num; i++ {
		field := t.Field(i)
		fieldV := v.Field(i)
		defaultValStr := field.Tag.Get("def")
		descVal := field.Tag.Get("desc")
		skipVal := field.Tag.Get("skip")
		nameVal := field.Tag.Get("name")
		if nameVal == "" {
			nameVal = strcase.ToKebab(field.Name)
		}
		if skipVal == "true" || slices.StringContains(skip, nameVal) {
			continue
		}

		descVal = strings.ReplaceAll(descVal, "<supportedProfilers>", supportedSpies)

		switch field.Type {
		case reflect.TypeOf([]string{}):
			val := fieldV.Addr().Interface().(*[]string)
			val2 := (*arrayFlags)(val)
			flagSet.Var(val2, nameVal, descVal)
		case reflect.TypeOf(""):
			val := fieldV.Addr().Interface().(*string)
			defaultValStr := strings.ReplaceAll(defaultValStr, "<installPrefix>", installPrefix)
			flagSet.StringVar(val, nameVal, defaultValStr, descVal)
		case reflect.TypeOf(true):
			val := fieldV.Addr().Interface().(*bool)
			flagSet.BoolVar(val, nameVal, defaultValStr == "true", descVal)
		case reflect.TypeOf(time.Time{}):
			valTime := fieldV.Addr().Interface().(*time.Time)
			val := (*timeFlag)(valTime)
			flagSet.Var(val, nameVal, descVal)
		case reflect.TypeOf(time.Second):
			val := fieldV.Addr().Interface().(*time.Duration)
			var defaultVal time.Duration
			if defaultValStr == "" {
				defaultVal = time.Duration(0)
			} else {
				var err error
				defaultVal, err = time.ParseDuration(defaultValStr)
				if err != nil {
					logrus.Fatalf("invalid default value: %q (%s)", defaultValStr, nameVal)
				}
			}
			flagSet.DurationVar(val, nameVal, defaultVal, descVal)
		case reflect.TypeOf(1):
			val := fieldV.Addr().Interface().(*int)
			var defaultVal int
			if defaultValStr == "" {
				defaultVal = 0
			} else {
				var err error
				defaultVal, err = strconv.Atoi(defaultValStr)
				if err != nil {
					logrus.Fatalf("invalid default value: %q (%s)", defaultValStr, nameVal)
				}
			}
			flagSet.IntVar(val, nameVal, defaultVal, descVal)
		default:
			logrus.Fatalf("type %s is not supported", field.Type)
		}
	}
	return NewSortedFlags(obj, flagSet)
}

// on mac pyroscope is usually installed via homebrew. homebrew installs under a prefix
//   this is logic to figure out what prefix it is
func getInstallPrefix() string {
	if runtime.GOOS != "darwin" {
		return ""
	}

	executablePath, err := os.Executable()
	if err != nil {
		// TODO: figure out what kind of errors might happen, handle it
		return ""
	}
	cellarPath := filepath.Clean(filepath.Join(resolvePath(executablePath), "../../../.."))

	if !strings.HasSuffix(cellarPath, "Cellar") {
		// looks like it's not installed via homebrew
		return ""
	}

	return filepath.Clean(filepath.Join(cellarPath, "../"))
}

func resolvePath(path string) string {
	if res, err := filepath.EvalSymlinks(path); err == nil {
		return res
	}
	return path
}

func generateRootCmd(cfg *config.Config) *ffcli.Command {
	var (
		serverFlagSet    = flag.NewFlagSet("pyroscope server", flag.ExitOnError)
		convertFlagSet   = flag.NewFlagSet("pyroscope convert", flag.ExitOnError)
		execFlagSet      = flag.NewFlagSet("pyroscope exec", flag.ExitOnError)
		connectFlagSet   = flag.NewFlagSet("pyroscope connect", flag.ExitOnError)
		dbmanagerFlagSet = flag.NewFlagSet("pyroscope dbmanager", flag.ExitOnError)
		rootFlagSet      = flag.NewFlagSet("pyroscope", flag.ExitOnError)
	)

	serverSortedFlags := PopulateFlagSet(&cfg.Server, serverFlagSet)
	convertSortedFlags := PopulateFlagSet(&cfg.Convert, convertFlagSet)
	execSortedFlags := PopulateFlagSet(&cfg.Exec, execFlagSet, "pid")
	connectSortedFlags := PopulateFlagSet(&cfg.Exec, connectFlagSet)
	dbmanagerSortedFlags := PopulateFlagSet(&cfg.DbManager, dbmanagerFlagSet)
	rootSortedFlags := PopulateFlagSet(cfg, rootFlagSet)

	options := []ff.Option{
		ff.WithConfigFileParser(ffyaml.Parser),
		ff.WithEnvVarPrefix("PYROSCOPE"),
		ff.WithAllowMissingConfigFile(true),
		ff.WithConfigFileFlag("config"),
	}

	serverCmd := &ffcli.Command{
		UsageFunc:  serverSortedFlags.printUsage,
		Options:    options,
		Name:       "server",
		ShortUsage: "pyroscope server [flags]",
		ShortHelp:  "starts pyroscope server. This is the database + web-based user interface",
		FlagSet:    serverFlagSet,
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

	rootCmd := &ffcli.Command{
		UsageFunc:  rootSortedFlags.printUsage,
		Options:    options,
		ShortUsage: "pyroscope [flags] <subcommand>",
		FlagSet:    rootFlagSet,
		Subcommands: []*ffcli.Command{
			convertCmd,
			serverCmd,
			execCmd,
			connectCmd,
			dbmanagerCmd,
		},
	}

	serverCmd.Exec = func(_ context.Context, args []string) error {
		if l, err := logrus.ParseLevel(cfg.Server.LogLevel); err == nil {
			logrus.SetLevel(l)
		}
		startServer(cfg)
		return nil
	}
	convertCmd.Exec = func(_ context.Context, args []string) error {
		return convert.Cli(cfg, args)
	}
	execCmd.Exec = func(_ context.Context, args []string) error {
		if cfg.Exec.NoLogging {
			logrus.SetLevel(logrus.PanicLevel)
		} else if l, err := logrus.ParseLevel(cfg.Exec.LogLevel); err == nil {
			logrus.SetLevel(l)
		}
		if len(args) == 0 || args[0] == "help" {
			fmt.Println(gradientBanner())
			fmt.Println(DefaultUsageFunc(execSortedFlags, execCmd))
			return nil
		}

		return exec.Cli(cfg, args)
	}

	connectCmd.Exec = func(_ context.Context, args []string) error {
		if cfg.Exec.NoLogging {
			logrus.SetLevel(logrus.PanicLevel)
		} else if l, err := logrus.ParseLevel(cfg.Exec.LogLevel); err == nil {
			logrus.SetLevel(l)
		}
		if len(args) > 0 && args[0] == "help" {
			fmt.Println(gradientBanner())
			fmt.Println(DefaultUsageFunc(connectSortedFlags, connectCmd))
			return nil
		}

		return exec.Cli(cfg, args)
	}

	dbmanagerCmd.Exec = func(_ context.Context, args []string) error {
		if l, err := logrus.ParseLevel(cfg.DbManager.LogLevel); err == nil {
			logrus.SetLevel(l)
		}
		return dbmanager.Cli(cfg, args)
	}
	rootCmd.Exec = func(_ context.Context, args []string) error {
		if cfg.Version || len(args) > 0 && args[0] == "version" {
			fmt.Println(gradientBanner())
			fmt.Println(build.Summary())
			fmt.Println("")
		} else {
			fmt.Println(gradientBanner())
			fmt.Println(DefaultUsageFunc(rootSortedFlags, rootCmd))
		}
		return nil
	}

	return rootCmd
}

func Start(cfg *config.Config) error {
	return generateRootCmd(cfg).ParseAndRun(context.Background(), os.Args[1:])
}

func startServer(cfg *config.Config) {
	s, err := storage.New(cfg)
	atexit.Register(func() { s.Close() })
	if err != nil {
		panic(err)
	}
	u := direct.New(cfg, s)
	go agent.SelfProfile(cfg, u, "pyroscope.server", logrus.StandardLogger())
	go printRAMUsage()
	go printDiskUsage(cfg)
	c := server.New(cfg, s)
	atexit.Register(func() { c.Stop() })
	if !cfg.Server.AnalyticsOptOut {
		analyticsService := analytics.NewService(cfg, s, c)
		go analyticsService.Start()
		atexit.Register(func() { analyticsService.Stop() })
	}
	// if you ever change this line, make sure to update this homebrew test:
	//   https://github.com/pyroscope-io/homebrew-brew/blob/main/Formula/pyroscope.rb#L94
	logrus.Info("starting HTTP server")
	c.Start()
}

func printRAMUsage() {
	t := time.NewTicker(30 * time.Second)
	for {
		<-t.C
		if logrus.IsLevelEnabled(logrus.DebugLevel) {
			debug.PrintMemUsage()
		}
	}
}

func printDiskUsage(cfg *config.Config) {
	t := time.NewTicker(30 * time.Second)
	for {
		<-t.C
		if logrus.IsLevelEnabled(logrus.DebugLevel) {
			debug.PrintDiskUsage(cfg.Server.StoragePath)
		}
	}
}
