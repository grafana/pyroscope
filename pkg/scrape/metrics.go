package scrape

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

type metrics struct {
	pools             prometheus.Counter
	poolsFailed       prometheus.Counter
	poolReloads       prometheus.Counter
	poolReloadsFailed prometheus.Counter

	// Once pool exits, these metrics should be also unregistered.
	// Metrics specific to jobs (pools).
	poolReloadIntervalLength *prometheus.SummaryVec
	poolSyncIntervalLength   *prometheus.SummaryVec
	poolSyncs                *prometheus.CounterVec
	poolSyncFailed           *prometheus.CounterVec
	poolTargetsAdded         *prometheus.GaugeVec
	// Metrics shared by scrape loops.
	scrapes              *prometheus.CounterVec
	scrapesFailed        *prometheus.CounterVec
	scrapeIntervalLength *prometheus.SummaryVec
	// Metrics specific to targets.
	profileSize    *prometheus.SummaryVec
	profileSamples *prometheus.SummaryVec
	scrapeDuration *prometheus.SummaryVec
}

type poolMetrics struct {
	poolSyncs                prometheus.Counter
	poolSyncFailed           prometheus.Counter
	poolReloadIntervalLength prometheus.Observer
	poolSyncIntervalLength   prometheus.Observer
	poolTargetsAdded         prometheus.Gauge

	scrapes              prometheus.Counter
	scrapesFailed        prometheus.Counter
	scrapeIntervalLength prometheus.Observer
}

type targetMetrics struct {
	profileSize    prometheus.Observer
	profileSamples prometheus.Observer
	scrapeDuration prometheus.Observer
}

func newMetrics(r prometheus.Registerer) *metrics {
	poolLabels := []string{"scrape_job"}
	targetLabels := []string{"scrape_job", "profile_path"}
	return &metrics{
		pools: promauto.With(r).NewCounter(prometheus.CounterOpts{
			Name: "pyroscope_scrape_target_pools_total",
			Help: "Total number of scrape pool creation attempts.",
		}),
		poolsFailed: promauto.With(r).NewCounter(prometheus.CounterOpts{
			Name: "pyroscope_scrape_target_pools_failed_total",
			Help: "Total number of scrape pool creations that failed.",
		}),
		poolReloads: promauto.With(r).NewCounter(prometheus.CounterOpts{
			Name: "pyroscope_scrape_target_pool_reloads_total",
			Help: "Total number of scrape pool reloads.",
		}),
		poolReloadsFailed: promauto.With(r).NewCounter(prometheus.CounterOpts{
			Name: "pyroscope_scrape_target_pool_reloads_failed_total",
			Help: "Total number of failed scrape pool reloads.",
		}),

		poolSyncs: promauto.With(r).NewCounterVec(prometheus.CounterOpts{
			Name: "pyroscope_scrape_target_pool_sync_total",
			Help: "Total number of syncs that were executed on a scrape pool.",
		}, poolLabels),
		poolSyncFailed: promauto.With(r).NewCounterVec(prometheus.CounterOpts{
			Name: "pyroscope_scrape_target_pool_sync_failed_total",
			Help: "Total number of target sync failures.",
		}, poolLabels),
		poolSyncIntervalLength: promauto.With(r).NewSummaryVec(prometheus.SummaryOpts{
			Name:       "pyroscope_scrape_target_pool_sync_length_seconds",
			Help:       "Actual interval to sync the scrape pool.",
			Objectives: map[float64]float64{0.01: 0.001, 0.05: 0.005, 0.5: 0.05, 0.90: 0.01, 0.99: 0.001},
		}, poolLabels),
		poolReloadIntervalLength: promauto.With(r).NewSummaryVec(prometheus.SummaryOpts{
			Name:       "pyroscope_scrape_target_pool_reload_length_seconds",
			Help:       "Actual interval to reload the scrape pool with a given configuration.",
			Objectives: map[float64]float64{0.01: 0.001, 0.05: 0.005, 0.5: 0.05, 0.90: 0.01, 0.99: 0.001},
		}, poolLabels),
		poolTargetsAdded: promauto.With(r).NewGaugeVec(prometheus.GaugeOpts{
			Name: "pyroscope_scrape_target_pool_targets",
			Help: "Current number of targets in this scrape pool.",
		}, poolLabels),

		scrapes: promauto.With(r).NewCounterVec(prometheus.CounterOpts{
			Name: "pyroscope_scrape_target_pool_scrapes_total",
			Help: "Total number of scrapes that were executed on a scrape pool.",
		}, poolLabels),
		scrapesFailed: promauto.With(r).NewCounterVec(prometheus.CounterOpts{
			Name: "pyroscope_scrape_target_pool_scrapes_failed_total",
			Help: "Total number of scrapes failed.",
		}, poolLabels),
		scrapeIntervalLength: promauto.With(r).NewSummaryVec(prometheus.SummaryOpts{
			Name:       "pyroscope_scrape_target_pool_scrape_interval_length_seconds",
			Help:       "Actual intervals between scrapes.",
			Objectives: map[float64]float64{0.01: 0.001, 0.05: 0.005, 0.5: 0.05, 0.90: 0.01, 0.99: 0.001},
		}, poolLabels),

		profileSize: promauto.With(r).NewSummaryVec(prometheus.SummaryOpts{
			Name:       "pyroscope_scrape_target_profile_size_bytes",
			Help:       "Size of scraped profiles.",
			Objectives: map[float64]float64{0.01: 0.001, 0.05: 0.005, 0.5: 0.05, 0.90: 0.01, 0.99: 0.001},
		}, targetLabels),
		profileSamples: promauto.With(r).NewSummaryVec(prometheus.SummaryOpts{
			Name:       "pyroscope_scrape_target_profile_samples",
			Help:       "Number of samples per profile.",
			Objectives: map[float64]float64{0.01: 0.001, 0.05: 0.005, 0.5: 0.05, 0.90: 0.01, 0.99: 0.001},
		}, targetLabels),
		scrapeDuration: promauto.With(r).NewSummaryVec(prometheus.SummaryOpts{
			Name:       "pyroscope_scrape_target_scrape_duration_seconds",
			Help:       "Actual duration of profile scraping.",
			Objectives: map[float64]float64{0.01: 0.001, 0.05: 0.005, 0.5: 0.05, 0.90: 0.01, 0.99: 0.001},
		}, targetLabels),
	}
}

func (m *metrics) poolMetrics(jobName string) *poolMetrics {
	return &poolMetrics{
		poolSyncs:                m.poolSyncs.WithLabelValues(jobName),
		poolSyncFailed:           m.poolSyncFailed.WithLabelValues(jobName),
		poolSyncIntervalLength:   m.poolSyncIntervalLength.WithLabelValues(jobName),
		poolReloadIntervalLength: m.poolReloadIntervalLength.WithLabelValues(jobName),
		poolTargetsAdded:         m.poolTargetsAdded.WithLabelValues(jobName),

		scrapes:              m.scrapes.WithLabelValues(jobName),
		scrapesFailed:        m.scrapesFailed.WithLabelValues(jobName),
		scrapeIntervalLength: m.scrapeIntervalLength.WithLabelValues(jobName),
	}
}

func (m *metrics) targetMetrics(jobName, profilePath string) *targetMetrics {
	return &targetMetrics{
		profileSize:    m.profileSize.WithLabelValues(jobName, profilePath),
		profileSamples: m.profileSamples.WithLabelValues(jobName, profilePath),
		scrapeDuration: m.scrapeDuration.WithLabelValues(jobName, profilePath),
	}
}
