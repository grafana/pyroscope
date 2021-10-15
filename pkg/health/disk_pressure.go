package health

import (
	"fmt"

	"github.com/pyroscope-io/pyroscope/pkg/util/bytesize"
	"github.com/pyroscope-io/pyroscope/pkg/util/disk"
)

type DiskPressure struct {
	WarningThreshold  bytesize.ByteSize
	CriticalThreshold bytesize.ByteSize
	Path              string
}

func (d *DiskPressure) Probe() (StatusMessage, error) {
	var m StatusMessage
	available, err := disk.FreeSpace(d.Path)
	if err != nil {
		return m, err
	}
	switch {
	case available < d.CriticalThreshold:
		m.Status = Critical
	case available < d.WarningThreshold:
		m.Status = Warning
	default:
		m.Status = Healthy
	}
	m.Message = fmt.Sprintf("%v! Running out of disk space. Only %v is available", m.Status, available)
	return m, nil
}
