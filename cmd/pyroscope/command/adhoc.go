package command

import (
	"github.com/spf13/cobra"

	"github.com/pyroscope-io/pyroscope/pkg/adhoc"
	"github.com/pyroscope-io/pyroscope/pkg/cli"
	"github.com/pyroscope-io/pyroscope/pkg/config"
)

func newAdhocCmd(cfg *config.Adhoc) *cobra.Command {
	vpr := newViper()

	cmd := &cobra.Command{
		Use:   "adhoc [flags] [<args>]",
		Short: "Profile a process and save the results to be used in adhoc mode",
		Long: `adhoc command is a complete toolset to profile a process and save the profiling
results.

These results are then available to be visualized both as standalone HTML files
(unless '--no-standalone-html' argument is provided) and through the
 'Adhoc Profiling' section in the UI (available through 'pyroscope server').

There are multiple ways to gather the profiling data, and not all of them are
available for all the languages.
Which way is better depends on several factors: what the language supports,
how the profiled process is launched, and how the profiled process provides
the profiled data.

The different supported ways are:
- exec. In this case, pyroscope creates a different process for the profiled
program and uses a spy to directly gather profiling data. It's a useful way
to profile a whole execution of some program that has no other pyroscope
integration or way of exposing profiling data.
It's the default mode for languages with a supported spy when either the spy
name is specified (through '--spy-name' flag) or when the spyname is
autodetected.
- connect. Similar to exec, pyroscope uses a spy to gather profiling data,
but instead of creating a new profiled process, it spies an already running
process, indicated through '--pid' flag.
- push. In this case, pyroscope creates a different process for the profiled
program and launches an HTTP server with an ingestion endpoint. It's useful
to profile programs already integrated with Pyroscope using its HTTP API.
Push mode is used by default when no spy is detected and no '--url' flag is
provided. It can also be override the default exec mode with the '--push' flag.
- pull. In this case, pyroscope periodically connects to the URL specified
thorugh '--url' where it tries to retrieve profiling data in any of the
supported formats. In this case arguments are optional, and if provided,
they are used to launch a new process before polling the URL.`,
		DisableFlagParsing: true,
		RunE: cli.CreateCmdRunFn(cfg, vpr, func(_ *cobra.Command, args []string) error {
			return adhoc.Cli(cfg, args)
		}),
	}

	cli.PopulateFlagSet(cfg, cmd.Flags(), vpr)
	return cmd
}
