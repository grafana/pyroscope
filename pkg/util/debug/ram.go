package debug

import (
	"runtime"

	"github.com/pyroscope-io/pyroscope/pkg/util/bytesize"
)

func MemUsage() map[string]interface{} {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	return map[string]interface{}{
		"Alloc":      bytesize.ByteSize(m.Alloc),
		"TotalAlloc": bytesize.ByteSize(m.TotalAlloc),
		"Sys":        bytesize.ByteSize(m.Sys),
		"NumGC":      m.NumGC,
	}
}
