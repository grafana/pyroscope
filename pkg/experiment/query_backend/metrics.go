package query_backend

import "github.com/prometheus/client_golang/prometheus"

type metrics struct {
	tenantIsolationViolationAttempt prometheus.Counter
}

func newMetrics(reg prometheus.Registerer) *metrics {
	m := &metrics{
		tenantIsolationViolationAttempt: prometheus.NewCounter(
			prometheus.CounterOpts{
				Namespace: "pyroscope",
				Subsystem: "query_backend",
				Name:      "tenant_isolation_violation_query_attempt_total",
			}),
	}
	if reg != nil {
		reg.MustRegister(m.tenantIsolationViolationAttempt)
	}
	return m
}
