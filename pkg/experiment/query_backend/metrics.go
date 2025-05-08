package query_backend

import "github.com/prometheus/client_golang/prometheus"

type metrics struct {
	mismatchingTenantDataset *prometheus.CounterVec
}

func newMetrics(reg prometheus.Registerer) *metrics {
	m := &metrics{
		mismatchingTenantDataset: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "pyroscope",
				Subsystem: "query_backend",
				Name:      "invalid_tenant_query_total",
			}, []string{"tenant"}),
	}
	if reg != nil {
		reg.MustRegister(m.mismatchingTenantDataset)
	}
	return m
}
