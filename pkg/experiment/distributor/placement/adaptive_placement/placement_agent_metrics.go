package adaptive_placement

import (
	"github.com/prometheus/client_golang/prometheus"
)

type agentMetrics struct {
	lag prometheus.Gauge
}

func newAgentMetrics(reg prometheus.Registerer) *agentMetrics {
	m := &agentMetrics{
		lag: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "pyroscope_adaptive_sharding_rules_update_lag_seconds",
			Help: "Delay in seconds since the last update of the placement rules.",
		}),
	}
	if reg != nil {
		reg.MustRegister(m.lag)
	}
	return m
}
