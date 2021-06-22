package debug

import (
	"time"

	"github.com/pyroscope-io/pyroscope/pkg/config"
	"github.com/pyroscope-io/pyroscope/pkg/storage"
	"github.com/pyroscope-io/pyroscope/pkg/util/bytesize"
	"github.com/pyroscope-io/pyroscope/pkg/util/metrics"
	"github.com/sirupsen/logrus"
)

const debugInfoReportingInterval = time.Second

type Reporter struct {
	config  *config.Server
	storage *storage.Storage
	logger  *logrus.Logger
	stopped chan struct{}
	done    chan struct{}
}

func NewReporter(l *logrus.Logger, s *storage.Storage, c *config.Server) *Reporter {
	return &Reporter{
		config:  c,
		storage: s,
		logger:  l,
		stopped: make(chan struct{}),
		done:    make(chan struct{}),
	}
}

func (d *Reporter) Stop() {
	close(d.stopped)
	<-d.done
}

func (d *Reporter) Start() {
	defer close(d.done)
	ticker := time.NewTicker(debugInfoReportingInterval)
	defer ticker.Stop()
	var counter int
	for {
		select {
		case <-d.stopped:
			return
		case <-ticker.C:
			maps := map[string]map[string]interface{}{
				"cpu":   CPUUsage(debugInfoReportingInterval),
				"disk":  DiskUsage(d.config.StoragePath),
				"cache": d.storage.CacheStats(),
			}
			for dataType, data := range maps {
				for k, v := range data {
					if iv, ok := v.(bytesize.ByteSize); ok {
						v = int64(iv)
					}
					metrics.Gauge(dataType+"_"+k, v)
				}
				if counter%30 == 0 {
					d.logger.WithFields(data).Debug(dataType + " stats")
				}
			}
			counter++
		}
	}
}
