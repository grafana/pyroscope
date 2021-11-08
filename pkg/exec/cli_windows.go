package exec

import (
	"os/exec"

	"github.com/pyroscope-io/pyroscope/pkg/config"
)

func performOSChecks(_ string) error { return nil }

func adjustCmd(_ *exec.Cmd, _ config.Exec) error { return nil }
