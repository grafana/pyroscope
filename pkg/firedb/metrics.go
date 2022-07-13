package firedb

import (
	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/atomic"
)

type headMetrics struct {
	series        prometheus.GaugeFunc
	seriesCreated prometheus.Counter

	profiles        prometheus.GaugeFunc
	profilesCreated prometheus.Counter

	sizeBytes       *prometheus.GaugeVec
	sizeBytesByType map[string]*atomic.Uint64
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
		sizeBytes: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "fire_head_size_bytes",
				Help: "Size of a particular in memory store within the head firedb block.",
			},
			[]string{"type"}),
		sizeBytesByType: map[string]*atomic.Uint64{
			"strings":      &head.strings.size,
			"mappings":     &head.mappings.size,
			"functions":    &head.functions.size,
			"stacktraces":  &head.stacktraces.size,
			"profiles":     &head.profiles.size,
			"pprof-labels": &head.pprofLabelCache.size,
		},
	}
	if reg != nil {
		reg.MustRegister(
			m.series,
			m.seriesCreated,
			m.profiles,
			m.profilesCreated,
			m,
		)
	}
	return m
}

func (m *headMetrics) Describe(ch chan<- *prometheus.Desc) {
	m.sizeBytes.Describe(ch)
}

func (m *headMetrics) Collect(ch chan<- prometheus.Metric) {
	for typ, val := range m.sizeBytesByType {
		m.sizeBytes.WithLabelValues(typ).Set(float64(val.Load()))
	}
	m.sizeBytes.Collect(ch)
}
