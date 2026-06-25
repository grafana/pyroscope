package querybackend

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

type metrics struct {
	datasetTenantIsolationFailure prometheus.Counter

	// Symbol-services query metrics.
	symbolServicesCandidatesTotal prometheus.Counter
	symbolServicesVerifyDuration  prometheus.Histogram
}

func newMetrics(reg prometheus.Registerer) *metrics {
	m := &metrics{
		datasetTenantIsolationFailure: prometheus.NewCounter(
			prometheus.CounterOpts{
				Namespace: "pyroscope",
				Subsystem: "query_backend",
				Name:      "dataset_tenant_isolation_failure_total",
			}),

		symbolServicesCandidatesTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "pyroscope",
			Subsystem: "query_backend",
			Name:      "symbol_services_candidates_total",
			Help:      "Total number of symbol-bloom candidates verified across all symbol-services queries.",
		}),

		symbolServicesVerifyDuration: prometheus.NewHistogram(prometheus.HistogramOpts{
			Namespace:                       "pyroscope",
			Subsystem:                       "query_backend",
			Name:                            "symbol_services_verify_duration_seconds",
			Help:                            "Duration of the symbol-services verification phase (LookupSymbolBloomServices) per block.",
			Buckets:                         []float64{.05, .1, .25, .5, 1, 2.5, 5, 10, 30, 60},
			NativeHistogramBucketFactor:     1.1,
			NativeHistogramMaxBucketNumber:  100,
			NativeHistogramMinResetDuration: time.Hour,
		}),
	}
	if reg != nil {
		reg.MustRegister(
			m.datasetTenantIsolationFailure,
			m.symbolServicesCandidatesTotal,
			m.symbolServicesVerifyDuration,
		)
	}
	return m
}
