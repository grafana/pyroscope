package config

import (
	"context"
	"flag"
	"os"
	"reflect"
	"strconv"
	"strings"
	"time"

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
	return MaybeGradientBanner() + "\n" + DefaultUsageFunc(c)
}

func (cfg *Config) Usage() string {
	return DefaultUsageFunc(cfg.ffCommand)
}

func (cfg *Config) Load() error {
	var (
		rootFlagSet   = flag.NewFlagSet("pyroscope", flag.ExitOnError)
		agentFlagSet  = flag.NewFlagSet("pyroscope agent", flag.ExitOnError)
		serverFlagSet = flag.NewFlagSet("pyroscope server", flag.ExitOnError)
	)

	populateFlagSet(cfg, rootFlagSet)
	populateFlagSet(&cfg.Agent, agentFlagSet)
	populateFlagSet(&cfg.Server, serverFlagSet)

	options := []ff.Option{
		ff.WithConfigFileParser(ffyaml.Parser),
		ff.WithEnvVarPrefix("PYROSCOPE"),
		ff.WithAllowMissingConfigFile(true),
		ff.WithConfigFileFlag("config"),
	}

	agent := &ffcli.Command{
		UsageFunc:  printUsage,
		Options:    options,
		Name:       "agent",
		ShortUsage: "pyroscope agent [flags]",
		ShortHelp:  "starts pyroscope agent. Run this one on the machines you want to profile",
		FlagSet:    agentFlagSet,
	}

	server := &ffcli.Command{
		UsageFunc:  printUsage,
		Options:    options,
		Name:       "server",
		ShortUsage: "pyroscope server [flags]",
		ShortHelp:  "starts pyroscope server. This is the database + web-based user interface",
		FlagSet:    serverFlagSet,
	}

	root := &ffcli.Command{
		UsageFunc:   printUsage,
		Options:     options,
		ShortUsage:  "pyroscope [flags] <subcommand>",
		FlagSet:     rootFlagSet,
		Subcommands: []*ffcli.Command{agent, server},
	}

	agent.Exec = func(_ context.Context, args []string) error {
		cfg.ffCommand = agent
		cfg.Subcommand = "agent"
		return nil
	}
	server.Exec = func(_ context.Context, args []string) error {
		cfg.ffCommand = server
		cfg.Subcommand = "server"
		return nil
	}
	root.Exec = func(_ context.Context, args []string) error {
		cfg.ffCommand = root
		cfg.Subcommand = "main"
		return nil
	}

	return root.ParseAndRun(context.Background(), os.Args[1:])
}
