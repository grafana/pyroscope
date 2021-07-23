// +build linux

package main

import (
	"errors"

	"github.com/pyroscope-io/pyroscope/pkg/util/caps"
)

func performOSChecks() error {
	if !caps.HasSysPtraceCap() {
		return errors.New("if you're running pyroscope in a Docker container,  add --cap-add=sys_ptrace." +
			"See our Docker Guide for more information: https://pyroscope.io/docs/docker-guide")
	}
	return nil
}
