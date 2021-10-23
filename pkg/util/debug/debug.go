package debug

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/shirou/gopsutil/cpu"
	"github.com/sirupsen/logrus"

	"github.com/pyroscope-io/pyroscope/pkg/build"
	"github.com/pyroscope-io/pyroscope/pkg/storage"
)

// TODO(kolesnikovae): Get rid of it.

const debugInfoReportingInterval = time.Minute

type Reporter struct {
	logger  *logrus.Logger
	storage *storage.Storage
	stop    chan struct{}
	done    chan struct{}
}

func NewReporter(l *logrus.Logger, s *storage.Storage, reg prometheus.Registerer) *Reporter {
	promauto.With(reg).NewGaugeFunc(
		prometheus.GaugeOpts{
			Name:        "pyroscope_build_info",
			Help:        "A metric with a constant '1' value labeled by version, revision and other info from which pyroscope was built.",
			ConstLabels: build.PrometheusBuildLabels(),
		},
		func() float64 { return 1 },
	)

	return &Reporter{
		storage: s,
		logger:  l,
		stop:    make(chan struct{}),
		done:    make(chan struct{}),
	}
}

func CPUUsage(interval time.Duration) float64 {
	if v, err := cpu.Percent(interval, false); err == nil && len(v) > 0 {
		return v[0]
	}
	return 0
}

func (d *Reporter) Stop() {
	close(d.stop)
	<-d.done
}

func (d *Reporter) Start() {
	defer close(d.done)
	ticker := time.NewTicker(debugInfoReportingInterval)
	defer ticker.Stop()
	for {
		select {
		case <-d.stop:
			return
		case <-ticker.C:
			d.logger.WithField("utilization", CPUUsage(debugInfoReportingInterval)).Debug("cpu stats")
		}
	}
}
