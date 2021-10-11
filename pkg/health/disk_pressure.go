package health

import (
	"fmt"

	"github.com/pyroscope-io/pyroscope/pkg/config"
	"github.com/pyroscope-io/pyroscope/pkg/util/bytesize"
	"github.com/pyroscope-io/pyroscope/pkg/util/disk"
)

type DiskPressure struct {
	WarningThreshold  bytesize.ByteSize
	CriticalThreshold bytesize.ByteSize
	Config            *config.Server
}

func (d *DiskPressure) Probe() (HealthStatusMessage, error) {
	freeSpace, err := disk.FreeSpace(d.Config.StoragePath)
	if err == nil {
		status := d.status(freeSpace)
		message := d.message(status)
		return HealthStatusMessage{status, message}, nil
	}
	return HealthStatusMessage{NoData, ""}, err
}

func (d *DiskPressure) status(available bytesize.ByteSize) HealthStatus {
	if available < d.CriticalThreshold {
		return Critical
	} else if available < d.WarningThreshold {
		return Warning
	} else {
		return Healthy
	}
}

func (*DiskPressure) message(status HealthStatus) string {
	return fmt.Sprintf("Disk Pressure %s", status.String())
}
