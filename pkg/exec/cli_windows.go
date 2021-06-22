package exec

import (
	"os"
	"os/exec"

	"github.com/pyroscope-io/pyroscope/pkg/config"
)

func performOSChecks(_ string) error {
	return nil
}

func adjustCmd(_ *exec.Cmd, _ config.Exec) error {
	return nil
}

func processExists(pid int) bool {
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	_ = p.Release()
	return true
}
