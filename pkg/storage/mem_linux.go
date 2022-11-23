//go:build linux
// +build linux

package storage

import (
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/pyroscope-io/pyroscope/pkg/util/file"
	"github.com/shirou/gopsutil/mem"
	"github.com/sirupsen/logrus"
)

const memLimitPath = "/sys/fs/cgroup/memory/memory.limit_in_bytes"

func getCgroupMemLimit() (uint64, error) {
	f, err := os.Open(memLimitPath)
	if err != nil {
		return 0, err
	}
	b, err := io.ReadAll(f)
	if err != nil {
		return 0, err
	}
	r, err := strconv.Atoi(strings.TrimSuffix(string(b), "\n"))
	if err != nil {
		return 0, err
	}

	return uint64(r), nil
}

func getMemTotal() (uint64, error) {
	vm, err := mem.VirtualMemory()
	if err != nil {
		return 0, err
	}

	if file.Exists(memLimitPath) {
		v, err := getCgroupMemLimit()
		if err == nil {
			if v < vm.Total {
				return v, nil
			}
			return vm.Total, nil
		}

		logrus.WithError(err).Warn("Could not read cgroup memory limit")
		return vm.Total, nil
	}

	return vm.Total, nil
}
