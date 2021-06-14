package exec

import (
	"os/exec"

	"github.com/pyroscope-io/pyroscope/pkg/config"
)

func performOSChecks(_ string) error {
	return nil
}

func adjustCmd(cmd *exec.Cmd, cfg config.Exec) error {
	return nil
}
