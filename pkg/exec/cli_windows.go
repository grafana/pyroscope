package exec

import (
	"os/exec"
)

func performOSChecks(_ string) error { return nil }

func adjustCmd(_ *exec.Cmd, _ Config) error { return nil }
