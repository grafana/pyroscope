package exec

import (
	"os/exec"
)

func performOSChecks(_ string) error { return nil }

func adjustCmd(_ *exec.Cmd, _ bool, _, _ string) error { return nil }
