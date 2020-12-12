package cli

import (
	"context"
	"flag"
	"fmt"
	"os"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/petethepig/pyroscope/pkg/agent"
	"github.com/petethepig/pyroscope/pkg/agent/upstream/direct"
	"github.com/petethepig/pyroscope/pkg/build"
	"github.com/petethepig/pyroscope/pkg/config"
	"github.com/petethepig/pyroscope/pkg/convert"
	"github.com/petethepig/pyroscope/pkg/exec"
	"github.com/petethepig/pyroscope/pkg/server"
	"github.com/petethepig/pyroscope/pkg/storage"
	"github.com/petethepig/pyroscope/pkg/util/atexit"
	"github.com/petethepig/pyroscope/pkg/util/debug"
	"github.com/sirupsen/logrus"
	log "github.com/sirupsen/logrus"

	"github.com/iancoleman/strcase"
	"github.com/peterbourgon/ff/ffyaml"
	"github.com/peterbourgon/ff/v3"
	"github.com/peterbourgon/ff/v3/ffcli"
)

type arrayFlags []string

func (i *arrayFlags) String() string {
	return strings.Join(*i, ", ")
}

func (i *arrayFlags) Set(value string) error {
	*i = append(*i, value)
	return nil
}

// this is mostly reflection magic
func populateFlagSet(obj interface{}, flagSet *flag.FlagSet) {
	v := reflect.ValueOf(obj).Elem()
	t := reflect.TypeOf(v.Interface())
	num := t.NumField()

	for i := 0; i < num; i++ {
		field := t.Field(i)
		fieldV := v.Field(i)
		defaultValStr := field.Tag.Get("def")
		descVal := field.Tag.Get("desc")
		nameVal := field.Tag.Get("name")
		skipVal := field.Tag.Get("skip")
		if skipVal == "true" {
			continue
		}
		if nameVal == "" {
			nameVal = strcase.ToKebab(field.Name)
		}

		switch field.Type {
		case reflect.TypeOf([]string{}):
			val := fieldV.Addr().Interface().(*[]string)
			val2 := (*arrayFlags)(val)
			flagSet.Var(val2, nameVal, descVal)
		case reflect.TypeOf(""):
			val := fieldV.Addr().Interface().(*string)
			flagSet.StringVar(val, nameVal, defaultValStr, descVal)
		case reflect.TypeOf(true):
			val := fieldV.Addr().Interface().(*bool)
			flagSet.BoolVar(val, nameVal, defaultValStr == "true", descVal)
		case reflect.TypeOf(time.Second):
			val := fieldV.Addr().Interface().(*time.Duration)
			var defaultVal time.Duration
			if defaultValStr == "" {
				defaultVal = time.Duration(0)
			} else {
				var err error
				defaultVal, err = time.ParseDuration(defaultValStr)
				if err != nil {
					log.Fatalf("invalid default value: %q (%s)", defaultValStr, nameVal)
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
					log.Fatalf("invalid default value: %q (%s)", defaultValStr, nameVal)
				}
			}
			flagSet.IntVar(val, nameVal, defaultVal, descVal)
		default:
			log.Fatalf("type %s is not supported", field.Type)
		}
	}
}

func printUsage(c *ffcli.Command) string {
	return gradientBanner() + "\n" + DefaultUsageFunc(c)
}

func Start(cfg *config.Config) error {
	var (
		rootFlagSet    = flag.NewFlagSet("pyroscope", flag.ExitOnError)
		agentFlagSet   = flag.NewFlagSet("pyroscope agent", flag.ExitOnError)
		serverFlagSet  = flag.NewFlagSet("pyroscope server", flag.ExitOnError)
		convertFlagSet = flag.NewFlagSet("pyroscope convert", flag.ExitOnError)
		execFlagSet    = flag.NewFlagSet("pyroscope convert", flag.ExitOnError)
	)

	populateFlagSet(cfg, rootFlagSet)
	populateFlagSet(&cfg.Agent, agentFlagSet)
	populateFlagSet(&cfg.Server, serverFlagSet)
	populateFlagSet(&cfg.Convert, convertFlagSet)
	populateFlagSet(&cfg.Exec, execFlagSet)

	options := []ff.Option{
		ff.WithConfigFileParser(ffyaml.Parser),
		ff.WithEnvVarPrefix("PYROSCOPE"),
		ff.WithAllowMissingConfigFile(true),
		ff.WithConfigFileFlag("config"),
	}

	agentCmd := &ffcli.Command{
		UsageFunc:  printUsage,
		Options:    options,
		Name:       "agent",
		ShortUsage: "pyroscope agent [flags]",
		ShortHelp:  "starts pyroscope agent. Run this one on the machines you want to profile",
		FlagSet:    agentFlagSet,
	}

	serverCmd := &ffcli.Command{
		UsageFunc:  printUsage,
		Options:    options,
		Name:       "server",
		ShortUsage: "pyroscope server [flags]",
		ShortHelp:  "starts pyroscope server. This is the database + web-based user interface",
		FlagSet:    serverFlagSet,
	}

	convertCmd := &ffcli.Command{
		UsageFunc:  printUsage,
		Options:    options,
		Name:       "convert",
		ShortUsage: "pyroscope convert [flags] <input-file>",
		ShortHelp:  "converts between different profiling formats",
		FlagSet:    convertFlagSet,
	}

	execCmd := &ffcli.Command{
		UsageFunc:  printUsage,
		Options:    options,
		Name:       "exec",
		ShortUsage: "pyroscope exec [flags] args",
		ShortHelp:  "executes a command",
		FlagSet:    execFlagSet,
	}

	rootCmd := &ffcli.Command{
		UsageFunc:   printUsage,
		Options:     options,
		ShortUsage:  "pyroscope [flags] <subcommand>",
		FlagSet:     rootFlagSet,
		Subcommands: []*ffcli.Command{agentCmd, serverCmd, convertCmd, execCmd},
	}

	agentCmd.Exec = func(_ context.Context, args []string) error {
		if l, err := logrus.ParseLevel(cfg.Agent.LogLevel); err == nil {
			logrus.SetLevel(l)
		}
		a := agent.New(cfg)
		atexit.Register(a.Stop)
		a.Start()
		return nil
	}
	serverCmd.Exec = func(_ context.Context, args []string) error {
		if l, err := logrus.ParseLevel(cfg.Server.LogLevel); err == nil {
			logrus.SetLevel(l)
		}
		go printRAMUsage()
		go printDiskUsage(cfg)
		startServer(cfg)
		return nil
	}
	convertCmd.Exec = func(_ context.Context, args []string) error {
		return convert.Cli(cfg, args)
	}
	execCmd.Exec = func(_ context.Context, args []string) error {
		return exec.Cli(cfg, args)
	}
	rootCmd.Exec = func(_ context.Context, args []string) error {
		if cfg.Version || len(args) > 0 && args[0] == "version" {
			fmt.Println(gradientBanner())
			fmt.Println(build.Summary())
			fmt.Println("")
		} else {
			fmt.Println(gradientBanner())
			fmt.Println(DefaultUsageFunc(rootCmd))
		}
		return nil
	}

	return rootCmd.ParseAndRun(context.Background(), os.Args[1:])
}

func startServer(cfg *config.Config) {
	s, err := storage.New(cfg)
	if err != nil {
		panic(err)
	}
	u := direct.New(cfg, s)
	go agent.SelfProfile(cfg, u, "pyroscope.server.cpu{}")
	atexit.Register(func() { s.Close() })
	c := server.New(cfg, s)
	c.Start()
	time.Sleep(time.Second)
}

func printRAMUsage() {
	t := time.NewTicker(30 * time.Second)
	for {
		<-t.C
		if log.IsLevelEnabled(log.DebugLevel) {
			debug.PrintMemUsage()
		}
	}
}

func printDiskUsage(cfg *config.Config) {
	t := time.NewTicker(30 * time.Second)
	for {
		<-t.C
		if log.IsLevelEnabled(log.DebugLevel) {
			debug.PrintDiskUsage(cfg.Server.StoragePath)
		}
	}
}
