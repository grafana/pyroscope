package health

import (
	"errors"
	"fmt"

	"github.com/pyroscope-io/pyroscope/pkg/util/disk"
)

var (
	errZeroTotalSize          = errors.New("total disk size is zero")
	errTotalLessThanAvailable = errors.New("total disk size is less than available space")
)

type DiskPressure struct {
	Threshold float64
	Path      string
}

func (d DiskPressure) Probe() (StatusMessage, error) {
	if d.Threshold == 0 {
		return StatusMessage{Status: Healthy}, nil
	}
	u, err := disk.Usage(d.Path)
	if err != nil {
		return StatusMessage{}, err
	}
	return d.makeProbe(u)
}

func (d DiskPressure) makeProbe(u disk.UsageStats) (StatusMessage, error) {
	var m StatusMessage
	if u.Total == 0 {
		return m, errZeroTotalSize
	}
	if u.Available > u.Total {
		return m, errTotalLessThanAvailable
	}
	m.Status = Healthy
	availPercent := 100 * float64(u.Available) / float64(u.Total)
	if availPercent < d.Threshold {
		m.Status = Critical
		m.Message = fmt.Sprintf("Disk space is running low: %v available (%.1f%%)", u.Available, availPercent)
	}
	return m, nil
}
