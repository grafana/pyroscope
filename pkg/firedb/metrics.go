package firedb

import "github.com/prometheus/client_golang/prometheus"

type headMetrics struct {
	series        prometheus.GaugeFunc
	seriesCreated prometheus.Counter

	profiles        prometheus.GaugeFunc
	profilesCreated prometheus.Counter
}

func newHeadMetrics(head *Head, reg prometheus.Registerer) *headMetrics {
	m := &headMetrics{
		series: prometheus.NewGaugeFunc(prometheus.GaugeOpts{
			Name: "fire_tsdb_head_series",
			Help: "Total number of series in the head block.",
		}, func() float64 {
			return float64(head.index.totalSeries.Load())
		}),
		seriesCreated: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "fire_tsdb_head_series_created_total",
			Help: "Total number of series created in the head",
		}),
		profiles: prometheus.NewGaugeFunc(prometheus.GaugeOpts{
			Name: "fire_head_profiles",
			Help: "Total number of profiles in the head block.",
		}, func() float64 {
			return float64(head.index.totalProfiles.Load())
		}),
		profilesCreated: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "fire_head_profiles_created_total",
			Help: "Total number of profiles created in the head",
		}),
	}
	if reg != nil {
		reg.MustRegister(
			m.series,
			m.seriesCreated,
			m.profiles,
			m.profilesCreated,
		)
	}
	return m
}
