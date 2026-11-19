package querybackend

import "github.com/prometheus/client_golang/prometheus"

type metrics struct {
	datasetTenantIsolationFailure prometheus.Counter
	inflightEstimatedBytes        prometheus.Gauge
}

func newMetrics(reg prometheus.Registerer) *metrics {
	m := &metrics{
		datasetTenantIsolationFailure: prometheus.NewCounter(
			prometheus.CounterOpts{
				Namespace: "pyroscope",
				Subsystem: "query_backend",
				Name:      "dataset_tenant_isolation_failure_total",
			}),
		inflightEstimatedBytes: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Namespace: "pyroscope",
				Subsystem: "query_backend",
				Name:      "inflight_estimated_bytes",
				Help:      "Estimated dataset bytes in blocks currently being read.",
			}),
	}
	if reg != nil {
		reg.MustRegister(
			m.datasetTenantIsolationFailure,
			m.inflightEstimatedBytes,
		)
	}
	return m
}
