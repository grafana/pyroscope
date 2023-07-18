//go:build ebpfspy

package command

import (
	"github.com/spf13/cobra"

	"github.com/pyroscope-io/pyroscope/pkg/cli"
	"github.com/pyroscope-io/pyroscope/pkg/config"
	"github.com/pyroscope-io/pyroscope/pkg/exec"
)

func newEBPFSpyCmd(cfg *config.EBPF) *cobra.Command {
	vpr := newViper()
	connectCmd := &cobra.Command{
		Use:   "ebpf [flags]",
		Short: "Profile whole system using eBPF sampling profiler",
		Args:  cobra.NoArgs,

		RunE: cli.CreateCmdRunFn(cfg, vpr, func(_ *cobra.Command, _ []string) error {
			return exec.RunEBPF(cfg)
		}),
	}

	cli.PopulateFlagSet(cfg, connectCmd.Flags(), vpr)
	return connectCmd
}
