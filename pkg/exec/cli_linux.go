package exec

import (
	"errors"

	"github.com/pyroscope-io/pyroscope/pkg/util/caps"
)

func performOSChecks(spyName string) error {
	if disableLinuxChecks {
		return nil
	}
	if spyName == "ebpfspy" {
		if !isRoot() {
			return errors.New("when using eBPF you're required to run the agent with sudo")
		}
	} else {
		if !caps.HasSysPtraceCap() {
			return errors.New("if you're running pyroscope in a Docker container, add --cap-add=sys_ptrace. See our Docker Guide for more information: https://pyroscope.io/docs/docker-guide")
		}
	}
	return nil
}
