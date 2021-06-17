package debug

import (
	"time"

	"github.com/shirou/gopsutil/cpu"
)

func CPUUsage(interval time.Duration) map[string]interface{} {
	cpuVal, err := cpu.Percent(interval, false)
	if err != nil {
		return map[string]interface{}{}
	}

	return map[string]interface{}{
		"utilization": cpuVal[0],
	}
}
