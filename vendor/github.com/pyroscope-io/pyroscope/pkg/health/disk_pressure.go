package health

import (
	"fmt"

	"github.com/pyroscope-io/pyroscope/pkg/util/bytesize"
	"github.com/pyroscope-io/pyroscope/pkg/util/disk"
)

type DiskPressure struct {
	Threshold bytesize.ByteSize
	Path      string
}

func (d DiskPressure) Probe() (StatusMessage, error) {
	var m StatusMessage
	available, err := disk.FreeSpace(d.Path)
	if err != nil {
		return m, err
	}
	if available < d.Threshold {
		m.Status = Critical
	} else {
		m.Status = Healthy
	}
	m.Message = fmt.Sprintf("Disk space is running low: %v available", available)
	return m, nil
}
