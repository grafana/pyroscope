package querybackend

import "github.com/prometheus/client_golang/prometheus"

type metrics struct {
	datasetTenantIsolationFailure prometheus.Counter
	symbolizedDatasets            *prometheus.CounterVec
}

func newMetrics(reg prometheus.Registerer) *metrics {
	m := &metrics{
		datasetTenantIsolationFailure: prometheus.NewCounter(
			prometheus.CounterOpts{
				Namespace: "pyroscope",
				Subsystem: "query_backend",
				Name:      "dataset_tenant_isolation_failure_total",
			}),
		symbolizedDatasets: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "pyroscope",
				Subsystem: "query_backend",
				Name:      "symbolized_datasets_total",
				Help:      "Number of datasets symbolized at query time, by status.",
			}, []string{"status"}),
	}
	if reg != nil {
		reg.MustRegister(m.datasetTenantIsolationFailure)
		reg.MustRegister(m.symbolizedDatasets)
	}
	return m
}
