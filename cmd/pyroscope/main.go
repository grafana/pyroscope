package main

import (
	_ "embed"
	"flag"
	"fmt"
	"os"
	"sort"

	"github.com/grafana/dskit/flagext"
	"github.com/prometheus/common/version"

	"github.com/grafana/pyroscope/pkg/cfg"
	"github.com/grafana/pyroscope/pkg/phlare"
	"github.com/grafana/pyroscope/pkg/usage"
	_ "github.com/grafana/pyroscope/pkg/util/build"
)

type mainFlags struct {
	phlare.Config `yaml:",inline"`

	PrintVersion bool `yaml:"-"`
	PrintModules bool `yaml:"-"`
	PrintHelp    bool `yaml:"-"`
	PrintHelpAll bool `yaml:"-"`
}

func (mf *mainFlags) Clone() flagext.Registerer {
	return func(mf mainFlags) *mainFlags {
		return &mf
	}(*mf)
}

func (mf *mainFlags) PhlareConfig() *phlare.Config {
	return &mf.Config
}

func (mf *mainFlags) RegisterFlags(fs *flag.FlagSet) {
	mf.Config.RegisterFlags(fs)
	fs.BoolVar(&mf.PrintVersion, "version", false, "Show the version of pyroscope and exit")
	fs.BoolVar(&mf.PrintModules, "modules", false, "List available modules that can be used as target and exit.")
	fs.BoolVar(&mf.PrintHelp, "h", false, "Print basic help.")
	fs.BoolVar(&mf.PrintHelp, "help", false, "Print basic help.")
	fs.BoolVar(&mf.PrintHelpAll, "help-all", false, "Print help, also including advanced and experimental parameters.")
}
func errorHandler() {
	testMode := cfg.GetTestMode()
	if !testMode {
		os.Exit(1)
	}

}
func main() {
	var (
		flags mainFlags
	)

	if err := cfg.DynamicUnmarshal(&flags, os.Args[1:], flag.CommandLine); err != nil {
		fmt.Fprintf(os.Stderr, "failed parsing config: %v\n", err)
		errorHandler()
		return
	}

	f, err := phlare.New(flags.Config)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed creating pyroscope: %v\n", err)
		errorHandler()
		return
	}

	if flags.PrintVersion {
		fmt.Println(version.Print("pyroscope"))
		return
	}

	if flags.PrintModules {
		allDeps := f.ModuleManager.DependenciesForModule(phlare.All)

		for _, m := range f.ModuleManager.UserVisibleModuleNames() {
			ix := sort.SearchStrings(allDeps, m)
			included := ix < len(allDeps) && allDeps[ix] == m

			if included {
				fmt.Fprintln(os.Stdout, m, "*")
			} else {
				fmt.Fprintln(os.Stdout, m)
			}
		}

		fmt.Fprintln(os.Stdout)
		fmt.Fprintln(os.Stdout, "Modules marked with * are included in target All.")
		return
	}

	if flags.PrintHelp || flags.PrintHelpAll {
		// Print available parameters to stdout, so that users can grep/less them easily.
		flag.CommandLine.SetOutput(os.Stdout)
		if err := usage.Usage(flags.PrintHelpAll, &flags); err != nil {
			fmt.Fprintf(os.Stderr, "error printing usage: %s\n", err)
			errorHandler()
			return
		}

		return
	}

	err = f.Run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed running pyroscope: %v\n", err)
		errorHandler()
		return
	}
}
