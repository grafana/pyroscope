package analytics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/pyroscope-io/pyroscope/pkg/util/slices"
	"github.com/sirupsen/logrus"
)

var metricsToCollect = []string{
	"go_info",
	"pyroscope_build_info",
	"process_cpu_seconds_total",
	"process_start_time_seconds",
	"go_goroutines",
	"go_memstats_sys_bytes",
}

func (s *Service) sendMetrics() {
	res, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		logrus.WithError(err).Error("failed to gather prometheus metrics")
	}
	for _, g := range res {
		if slices.StringContains(metricsToCollect, *g.Name) {
			logrus.Debug(g)
			// spew.Dump(g)
		}
	}
}
