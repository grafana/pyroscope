package writepath

import (
	"strconv"

	"github.com/prometheus/client_golang/prometheus"

	httputil "github.com/grafana/pyroscope/pkg/util/http"
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

type durationHistogramDims struct {
	path    string
	primary string
	status  string
}

func (d durationHistogramDims) slice() []string {
	return []string{d.path, d.primary, d.status}
}

func newDurationHistogramDims(r *route, err error) (d durationHistogramDims) {
	d.path = string(r.path)
	if r.primary {
		d.primary = "1"
	} else {
		d.primary = "0"
	}
	code, _ := httputil.ClientHTTPStatusAndError(err)
	d.status = strconv.Itoa(code)
	return d
}
