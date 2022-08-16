//go:build !ebpfspy

package command

import (
	"github.com/spf13/cobra"

	"github.com/pyroscope-io/pyroscope/pkg/config"
)

func newEBPFSpyCmd(cfg *config.EBPFSpy) *cobra.Command {
	return nil
}
