package storage

import (
	"io/ioutil"
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
	b, err := ioutil.ReadAll(f)
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
	if file.Exists(memLimitPath) {
		v, err := getCgroupMemLimit()
		if err == nil {
			return v, nil
		}
		logrus.WithError(err).Warn("Could not read cgroup memory limit")
	}

	vm, err := mem.VirtualMemory()
	if err != nil {
		return 0, err
	}

	return vm.Total, nil
}
