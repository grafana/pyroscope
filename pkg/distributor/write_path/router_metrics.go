package writepath

import (
	"strconv"

	"github.com/prometheus/client_golang/prometheus"
)

type metrics struct {
	durationHistogram *prometheus.HistogramVec
}

func newMetrics(reg prometheus.Registerer) *metrics {
	m := &metrics{
		durationHistogram: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "pyroscope_write_path_downstream_request_duration_seconds",
			Buckets: prometheus.ExponentialBucketsRange(0.001, 10, 30),
			Help:    "Duration of downstream requests made by the write path router.",
		}, []string{"route", "primary", "status"}),
	}
	if reg != nil {
		reg.MustRegister(m.durationHistogram)
	}
	return m
}

func newDurationHistogramDims(r *route, code int) []string {
	dims := []string{string(r.path), "1", strconv.Itoa(code)}
	if !r.primary {
		dims[1] = "0"
	}
	return dims
}
