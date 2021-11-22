package command

import (
	"github.com/spf13/cobra"

	"github.com/pyroscope-io/pyroscope/pkg/adhoc"
	"github.com/pyroscope-io/pyroscope/pkg/cli"
	"github.com/pyroscope-io/pyroscope/pkg/config"
)

func newAdhocCmd(cfg *config.Adhoc) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "adhoc",
		Short: "adhoc mode commands",
	}
	cmd.AddCommand(newAdhocRecordCmd(&cfg.AdhocRecord))
	cmd.AddCommand(newAdhocServerCmd(&cfg.AdhocServer))
	return cmd
}

func newAdhocRecordCmd(cfg *config.AdhocRecord) *cobra.Command {
	vpr := newViper()

	cmd := &cobra.Command{
		Use:   "record [flags]",
		Short: "Start a new process from arguments, profile it and record the results",
		Args:  cobra.MinimumNArgs(1),
		RunE: cli.CreateCmdRunFn(cfg, vpr, func(_ *cobra.Command, args []string) error {
			return adhoc.Record(cfg, args)
		}),
	}

	cli.PopulateFlagSet(cfg, cmd.Flags(), vpr)
	return cmd
}

func newAdhocServerCmd(cfg *config.AdhocServer) *cobra.Command {
	vpr := newViper()

	cmd := &cobra.Command{
		Use:   "server [flags]",
		Short: "Start the server to view adhoc results",

		DisableFlagParsing: true,
		RunE: cli.CreateCmdRunFn(cfg, vpr, func(_ *cobra.Command, args []string) error {
			return adhoc.Server(cfg)
		}),
	}

	cli.PopulateFlagSet(cfg, cmd.Flags(), vpr)
	return cmd
}
