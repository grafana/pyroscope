package exec

import (
	"errors"

	"github.com/pyroscope-io/pyroscope/pkg/util/caps"
)

func performOSChecks(spyName string) error {
	var err error
	switch {
	case disableLinuxChecks:
	case spyName == "dotnetspy":
	case spyName == "ebpfspy":
		if !isRoot() {
			err = errors.New("when using eBPF you're required to run the agent with sudo")
		}
	case !caps.HasSysPtraceCap():
		err = errors.New("if you're running pyroscope in a Docker container, add --cap-add=sys_ptrace. " +
			"See our Docker Guide for more information: https://pyroscope.io/docs/docker-guide")
	}
	return err
}
