package debug

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
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
	return &Reporter{
		config:  c,
		storage: s,
		logger:  l,
		stopped: make(chan struct{}),
		done:    make(chan struct{}),

		// these metrics were previously lazily instantiated
		// so by moving here their semantics have changed
		// since they are now initialized to 0
		cpuUtilization: promauto.With(reg).NewGauge(prometheus.GaugeOpts{
			Name: "cpu_utilization",
		}),
		diskLocalProfiles: promauto.With(reg).NewGauge(prometheus.GaugeOpts{
			Name: "disk_local_profiles",
		}),
		diskMain: promauto.With(reg).NewGauge(prometheus.GaugeOpts{
			Name: "disk_main",
		}),
		diskSegments: promauto.With(reg).NewGauge(prometheus.GaugeOpts{
			Name: "disk_segments",
		}),
		diskTrees: promauto.With(reg).NewGauge(prometheus.GaugeOpts{
			Name: "disk_trees",
		}),
		diskDicts: promauto.With(reg).NewGauge(prometheus.GaugeOpts{
			Name: "disk_dicts",
		}),
		diskDimensions: promauto.With(reg).NewGauge(prometheus.GaugeOpts{
			Name: "disk_dimensions",
		}),
		cacheDimensionsSize: promauto.With(reg).NewGauge(prometheus.GaugeOpts{
			Name: "cache_dimensions_size",
		}),
		cacheSegmentsSize: promauto.With(reg).NewGauge(prometheus.GaugeOpts{
			Name: "cache_segments_size",
		}),
		cacheDictsSize: promauto.With(reg).NewGauge(prometheus.GaugeOpts{
			Name: "cache_dicts_size",
		}),
		cacheTreesSize: promauto.With(reg).NewGauge(prometheus.GaugeOpts{
			Name: "cache_trees_size",
		}),
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
						return float64(iv), true
					}
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
			logData(c, "cache stats")

			counter++
		}
	}
}
