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
	state State

	available bytesize.ByteSize

	WarningThreshold  bytesize.ByteSize
	CriticalThreshold bytesize.ByteSize

	Config *config.Server
	ticker *time.Ticker
}

func (d *DiskPressure) State() State {
	if d.available > d.WarningThreshold {
		d.state = Healthy
	} else if d.available < d.WarningThreshold {
		d.state = Warning
	} else if d.available < d.CriticalThreshold {
		d.state = Critical
	} else {
		d.state = NoData
	}
	return d.state
}
func (d *DiskPressure) Message() string {
	return fmt.Sprintf("The condition is %d with %d avaialable", d.state, d.available)
}
func (d *DiskPressure) Stop() error {
	d.ticker.Stop()
	return nil
}
func (d *DiskPressure) MakeProbe() error {
	d.ticker = time.NewTicker(time.Second)
	<-d.ticker.C
	freeSpace, err := disk.FreeSpace(d.Config.StoragePath)
	if err == nil {
		d.available = freeSpace
	}
	return nil
}
