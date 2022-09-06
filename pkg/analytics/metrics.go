package analytics

import (
	"bytes"
	"fmt"
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/expfmt"
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
	"pyroscope_parser_incoming_requests_total",
	"pyroscope_parser_incoming_requests_bytes",
}

func (s *Service) sendMetrics() {
	res, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		logrus.WithError(err).Error("failed to gather prometheus metrics")
	}
	tmp := &bytes.Buffer{}
	for _, g := range res {
		if slices.StringContains(metricsToCollect, *g.Name) {
			expfmt.MetricFamilyToText(tmp, g)
		}
	}
	url := fmt.Sprintf(host+"/metrics/job/analytics/install_id/%s", s.s.InstallID())
	http.Post(url, "text/plain", tmp)
}
