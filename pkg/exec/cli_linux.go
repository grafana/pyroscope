// +build linux

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
		if !hasSysPtraceCap() {
			return errors.New("if you're running pyroscope in a Docker container, add --cap-add=sys_ptrace. See our Docker Guide for more information: https://pyroscope.io/docs/docker-guide")
		}
	}
	return nil
}

// See linux source code: https://github.com/torvalds/linux/blob/6ad4bf6ea1609fb539a62f10fca87ddbd53a0315/include/uapi/linux/capability.h#L235
const CAP_SYS_PTRACE = 19

func hasSysPtraceCap() bool {
	c, err := caps.Get()
	if err != nil {
		return true // I don't know of cases when this would happen, but if it does I'd rather give this program a chance
	}

	if c.Inheritable() == 0 {
		return true // I don't know of cases when this would happen, but if it does I'd rather give this program a chance
	}

	return c.Has(CAP_SYS_PTRACE)
}
