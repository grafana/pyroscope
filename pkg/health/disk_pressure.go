// pkg/server/health/disk.go

package health

import (
	"fmt"
	"time"

	"github.com/pyroscope-io/pyroscope/pkg/config"
	"github.com/pyroscope-io/pyroscope/pkg/util/bytesize"
	"github.com/pyroscope-io/pyroscope/pkg/util/disk"
)

type DiskPressure struct {
	WarningThreshold  bytesize.ByteSize
	CriticalThreshold bytesize.ByteSize
	Name              string
	Config            *config.Server

	ticker *time.Ticker
}

func (d *DiskPressure) GetName() string {
	return d.Name
}

func (d *DiskPressure) Stop() {
	d.ticker.Stop()
}

func (d *DiskPressure) Probe() (HealthStatusMessage, error) {
	freeSpace, err := disk.FreeSpace(d.Config.StoragePath)
	if err == nil {
		state := d.state(freeSpace)
		message := d.message(state)
		return HealthStatusMessage{state, message}, nil
	}
	return HealthStatusMessage{NoData, ""}, err
}

func (d *DiskPressure) state(available bytesize.ByteSize) HealthStatus {
	if available < d.CriticalThreshold {
		return Critical
	} else if available < d.WarningThreshold {
		return Warning
	} else {
		return Healthy
	}
}

func (*DiskPressure) message(state HealthStatus) string {
	return fmt.Sprintf("Disk Pressure %s", state.ToString())
}
