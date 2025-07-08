package querybackend

import "github.com/prometheus/client_golang/prometheus"

type metrics struct {
	datasetTenantIsolationFailure prometheus.Counter
}

func newMetrics(reg prometheus.Registerer) *metrics {
	m := &metrics{
		datasetTenantIsolationFailure: prometheus.NewCounter(
			prometheus.CounterOpts{
				Namespace: "pyroscope",
				Subsystem: "query_backend",
				Name:      "dataset_tenant_isolation_failure_total",
			}),
	}
	if reg != nil {
		reg.MustRegister(m.datasetTenantIsolationFailure)
	}
	return m
}
