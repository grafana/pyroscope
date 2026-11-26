package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
)

type indexedRulerMetrics struct {
	indexTenants prometheus.Gauge
	indexFailure prometheus.Counter
	fetchTotal   *prometheus.CounterVec
}

func newIndexedRulerMetrics(reg prometheus.Registerer) *indexedRulerMetrics {
	m := &indexedRulerMetrics{
		indexTenants: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: "pyroscope",
			Name:      "recording_rules_index_tenants",
			Help:      "Number of tenants with stored recording rules in the loaded index.",
		}),
		indexFailure: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "pyroscope",
			Name:      "recording_rules_index_refresh_failures_total",
			Help:      "Number of failed refreshes of the recording rules index.",
		}),
		fetchTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "pyroscope",
			Name:      "recording_rules_fetches_total",
			Help:      "Number of fetches of a tenant's stored recording rules, by result.",
		}, []string{"result"}),
	}
	if reg != nil {
		reg.MustRegister(
			m.indexTenants,
			m.indexFailure,
			m.fetchTotal,
		)
	}
	return m
}
