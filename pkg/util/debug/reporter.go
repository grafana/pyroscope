package debug

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/pyroscope-io/pyroscope/pkg/build"
	"github.com/pyroscope-io/pyroscope/pkg/config"
	"github.com/pyroscope-io/pyroscope/pkg/storage"
	"github.com/pyroscope-io/pyroscope/pkg/util/bytesize"
	"github.com/sirupsen/logrus"
)

const debugInfoReportingInterval = time.Second

type Reporter struct {
	config  *config.Server
	storage *storage.Storage
	logger  *logrus.Logger
	stopped chan struct{}
	done    chan struct{}

	cpuUtilization      prometheus.Gauge
	diskLocalProfiles   prometheus.Gauge
	diskMain            prometheus.Gauge
	diskSegments        prometheus.Gauge
	diskTrees           prometheus.Gauge
	diskDicts           prometheus.Gauge
	diskDimensions      prometheus.Gauge
	cacheDimensionsSize prometheus.Gauge
	cacheSegmentsSize   prometheus.Gauge
	cacheDictsSize      prometheus.Gauge
	cacheTreesSize      prometheus.Gauge
}

func NewReporter(l *logrus.Logger, s *storage.Storage, c *config.Server, reg prometheus.Registerer) *Reporter {
	promauto.With(reg).NewGaugeFunc(
		prometheus.GaugeOpts{
			Name:        "pyroscope_build_info",
			Help:        "A metric with a constant '1' value labeled by version, revision and other info from which pyroscope was built.",
			ConstLabels: build.PrometheusBuildLabels(),
		},
		func() float64 { return 1 },
	)

	diskMetrics := promauto.With(reg).NewGaugeVec(prometheus.GaugeOpts{
		Name: "pyroscope_storage_disk_bytes",
		Help: "size of items in disk",
	}, []string{"name"})
	cacheSizeMetrics := promauto.With(reg).NewGaugeVec(prometheus.GaugeOpts{
		Name: "pyroscope_storage_cache_size",
		Help: "number of items in cache (memory)",
	}, []string{"name"})

	return &Reporter{
		config:  c,
		storage: s,
		logger:  l,
		stopped: make(chan struct{}),
		done:    make(chan struct{}),

		cpuUtilization: promauto.With(reg).NewGauge(prometheus.GaugeOpts{
			Name: "pyroscope_cpu_utilization",
			Help: "cpu utilization (percentage)",
		}),

		diskLocalProfiles: diskMetrics.With(prometheus.Labels{"name": "local_profiles"}),
		diskMain:          diskMetrics.With(prometheus.Labels{"name": "main"}),
		diskSegments:      diskMetrics.With(prometheus.Labels{"name": "segments"}),
		diskTrees:         diskMetrics.With(prometheus.Labels{"name": "trees"}),
		diskDicts:         diskMetrics.With(prometheus.Labels{"name": "dicts"}),
		diskDimensions:    diskMetrics.With(prometheus.Labels{"name": "dimensions"}),

		cacheDimensionsSize: cacheSizeMetrics.With(prometheus.Labels{"name": "dimensions"}),
		cacheSegmentsSize:   cacheSizeMetrics.With(prometheus.Labels{"name": "segments"}),
		cacheDictsSize:      cacheSizeMetrics.With(prometheus.Labels{"name": "dicts"}),
		cacheTreesSize:      cacheSizeMetrics.With(prometheus.Labels{"name": "trees"}),
	}
}

func (d *Reporter) Stop() {
	close(d.stopped)
	<-d.done
}

func (d *Reporter) Start() {
	defer close(d.done)
	ticker := time.NewTicker(debugInfoReportingInterval)
	defer ticker.Stop()
	var counter int
	for {
		select {
		case <-d.stopped:
			return
		case <-ticker.C:
			retAndConv := func(m map[string]interface{}, k string) (float64, bool) {
				if v, ok := m[k]; ok {
					if iv, ok := v.(bytesize.ByteSize); ok {
						v = int64(iv)
					}

					switch n := v.(type) {
					case int:
						return float64(n), true
					case uint:
						return float64(n), true
					case int64:
						return float64(n), true
					case uint64:
						return float64(n), true
					case int32:
						return float64(n), true
					case uint32:
						return float64(n), true
					case int16:
						return float64(n), true
					case uint16:
						return float64(n), true
					case int8:
						return float64(n), true
					case uint8:
						return float64(n), true
					case float64:
						return n, true
					case float32:
						return float64(n), true
					}
					return 0.0, false
				}

				return 0, false
			}

			logData := func(m map[string]interface{}, msg string) {
				if counter%30 == 0 {
					d.logger.WithFields(m).Debug(msg)
				}
			}

			// CPU
			c := CPUUsage(debugInfoReportingInterval)

			if v, ok := retAndConv(c, "utilization"); ok {
				d.cpuUtilization.Set(v)
			}
			logData(c, "cpu stats")

			// DISK
			disk := DiskUsage(d.config.StoragePath)
			if v, ok := retAndConv(disk, "local_profiles"); ok {
				d.diskLocalProfiles.Set(v)
			}
			if v, ok := retAndConv(disk, "main"); ok {
				d.diskMain.Set(v)
			}
			if v, ok := retAndConv(disk, "segments"); ok {
				d.diskSegments.Set(v)
			}
			if v, ok := retAndConv(disk, "trees"); ok {
				d.diskTrees.Set(v)
			}
			if v, ok := retAndConv(disk, "dicts"); ok {
				d.diskDicts.Set(v)
			}
			if v, ok := retAndConv(disk, "dimensions"); ok {
				d.diskDimensions.Set(v)
			}
			logData(disk, "disk stats")

			// CACHE
			cache := d.storage.CacheStats()
			if v, ok := retAndConv(cache, "dimensions_size"); ok {
				d.cacheDimensionsSize.Set(v)
			}
			if v, ok := retAndConv(cache, "segments_size"); ok {
				d.cacheSegmentsSize.Set(v)
			}
			if v, ok := retAndConv(cache, "dicts_size"); ok {
				d.cacheDictsSize.Set(v)
			}
			if v, ok := retAndConv(cache, "trees_size"); ok {
				d.cacheTreesSize.Set(v)
			}
			logData(cache, "cache stats")

			counter++
		}
	}
}
