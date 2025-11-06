package dlq

import (
	"github.com/prometheus/client_golang/prometheus"
)

type metrics struct {
	recoveryAttempts *prometheus.CounterVec
}

func newMetrics(reg prometheus.Registerer) *metrics {
	m := &metrics{
		recoveryAttempts: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "pyroscope",
				Subsystem: "metastore",
				Name:      "dlq_recovery_attempts_total",
				Help:      "Total number of DLQ block recovery attempts by status.",
			},
			[]string{"status"},
		),
	}

	if reg != nil {
		reg.MustRegister(m.recoveryAttempts)
	}

	return m
}
