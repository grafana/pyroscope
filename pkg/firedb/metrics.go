package firedb

import (
	"github.com/prometheus/client_golang/prometheus"
)

type headMetrics struct {
	head *Head

	series        prometheus.GaugeFunc
	seriesCreated *prometheus.CounterVec

	profiles        prometheus.GaugeFunc
	profilesCreated *prometheus.CounterVec

	sizeBytes   *prometheus.GaugeVec
	rowsWritten *prometheus.CounterVec

	sampleValuesIngested *prometheus.CounterVec
	sampleValuesReceived *prometheus.CounterVec
}

func newHeadMetrics(reg prometheus.Registerer) *headMetrics {
	m := &headMetrics{
		seriesCreated: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "fire_tsdb_head_series_created_total",
			Help: "Total number of series created in the head",
		}, []string{"profile_name"}),
		sizeBytes: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "fire_head_size_bytes",
				Help: "Size of a particular in memory store within the head firedb block.",
			},
			[]string{"type"}),
		rowsWritten: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "fire_rows_written",
				Help: "Number of rows written to a parquet table.",
			},
			[]string{"type"}),
		profilesCreated: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "fire_head_profiles_created_total",
			Help: "Total number of profiles created in the head",
		}, []string{"profile_name"}),
		sampleValuesIngested: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "fire_head_ingested_sample_values_total",
				Help: "Number of sample values ingested into the head per profile type.",
			},
			[]string{"profile_name"}),
		sampleValuesReceived: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "fire_head_received_sample_values_total",
				Help: "Number of sample values received into the head per profile type.",
			},
			[]string{"profile_name"}),
	}

	m.series = prometheus.NewGaugeFunc(prometheus.GaugeOpts{
		Name: "fire_tsdb_head_series",
		Help: "Total number of series in the head block.",
	}, func() float64 {
		if m.head == nil {
			return 0.0
		}
		return float64(m.head.index.totalSeries.Load())
	})
	m.profiles = prometheus.NewGaugeFunc(prometheus.GaugeOpts{
		Name: "fire_head_profiles",
		Help: "Total number of profiles in the head block.",
	}, func() float64 {
		return float64(m.head.index.totalProfiles.Load())
	})

	if reg != nil {
		reg.MustRegister(
			m.series,
			m.seriesCreated,
			m.profiles,
			m.profilesCreated,
			m.rowsWritten,
			m.sampleValuesIngested,
			m.sampleValuesReceived,
			m,
		)
	}
	return m
}

func (m *headMetrics) setHead(head *Head) *headMetrics {
	m.head = head
	return m
}

func (m *headMetrics) Describe(ch chan<- *prometheus.Desc) {
	m.sizeBytes.Describe(ch)
}

func (m *headMetrics) Collect(ch chan<- prometheus.Metric) {
	if m.head != nil {
		for _, t := range m.head.tables {
			m.sizeBytes.WithLabelValues(t.Name()).Set(float64(t.Size()))
		}
	}
	m.sizeBytes.Collect(ch)
}
