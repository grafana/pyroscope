package debug

import (
	"runtime"
	"strconv"

	"github.com/petethepig/pyroscope/pkg/util/bytesize"
	"github.com/sirupsen/logrus"
)

func PrintMemUsage() {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	logrus.WithFields(logrus.Fields{
		"Alloc":      bytesize.ByteSize(m.Alloc).String(),
		"TotalAlloc": bytesize.ByteSize(m.TotalAlloc).String(),
		"Sys":        bytesize.ByteSize(m.Sys).String(),
		"NumGC":      strconv.Itoa(int(m.NumGC)),
	}).Debug("RAM stats")
}
