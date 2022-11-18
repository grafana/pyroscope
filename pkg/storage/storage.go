package storage

// revive:disable:max-public-structs complex package

import (
	"context"
	"errors"
	"runtime"
	"sync"
	"time"

	"github.com/dgraph-io/badger/v2"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"

	"github.com/pyroscope-io/pyroscope/pkg/health"
	"github.com/pyroscope-io/pyroscope/pkg/model/appmetadata"
	"github.com/pyroscope-io/pyroscope/pkg/storage/cache"
	"github.com/pyroscope-io/pyroscope/pkg/storage/labels"
	"github.com/pyroscope-io/pyroscope/pkg/storage/segment"
	"github.com/pyroscope-io/pyroscope/pkg/util/bytesize"
)

var (
	errRetention  = errors.New("could not write because of retention settings")
	errOutOfSpace = errors.New("running out of space")
	errClosed     = errors.New("storage closed")
)

type Storage struct {
	config *Config
	*storageOptions

	logger *logrus.Logger
	*metrics

	segments   BadgerDBWithCache
	dimensions BadgerDBWithCache
	dicts      BadgerDBWithCache
	trees      BadgerDBWithCache
	main       BadgerDBWithCache
	labels     *labels.Labels
	exemplars  *exemplars

	appSvc ApplicationMetadataSaver
	hc     *health.Controller

	// Maintenance tasks are executed exclusively to avoid competition:
	// extensive writing during GC is harmful and deteriorates the
	// overall performance. Same for write back, eviction, and retention
	// tasks.
	tasksMutex sync.Mutex
	tasksWG    sync.WaitGroup
	stop       chan struct{}
	putMutex   sync.Mutex
}

type storageOptions struct {
	badgerGCTaskInterval      time.Duration
	metricsUpdateTaskInterval time.Duration
	writeBackTaskInterval     time.Duration
	evictionTaskInterval      time.Duration
	retentionTaskInterval     time.Duration
	cacheTTL                  time.Duration
	gcSizeDiff                bytesize.ByteSize
}

// MetricsExporter exports values of particular stack traces sample from profiling
// data as a Prometheus metrics.
type MetricsExporter interface {
	// Evaluate evaluates metrics export rules against the input key and creates
	// prometheus counters for new time series, if required. Returned observer can
	// be used to evaluate and observe particular samples.
	//
	// If there are no matching rules, the function returns false.
	Evaluate(*PutInput) (SampleObserver, bool)
}

type SampleObserver interface {
	// Observe adds v to the matched counters if k satisfies node selector.
	// k is a sample stack trace where frames are delimited by semicolon.
	// v is the sample value.
	Observe(k []byte, v int)
}

// ApplicationMetadataSaver saves application metadata
type ApplicationMetadataSaver interface {
	CreateOrUpdate(ctx context.Context, application appmetadata.ApplicationMetadata) error
}

func New(c *Config, logger *logrus.Logger, reg prometheus.Registerer, hc *health.Controller, appSvc ApplicationMetadataSaver) (*Storage, error) {
	s := &Storage{
		config: c,
		storageOptions: &storageOptions{
			// Interval at which GC triggered if the db size has increased more
			// than by gcSizeDiff since the last probe.
			badgerGCTaskInterval: 5 * time.Minute,
			// DB size and cache size metrics are updated periodically.
			metricsUpdateTaskInterval: 10 * time.Second,
			writeBackTaskInterval:     time.Minute,
			evictionTaskInterval:      20 * time.Second,
			retentionTaskInterval:     10 * time.Minute,
			cacheTTL:                  2 * time.Minute,
			// gcSizeDiff specifies the minimal storage size difference that
			// causes garbage collection to trigger.
			gcSizeDiff: bytesize.GB,
		},

		hc:      hc,
		logger:  logger,
		metrics: newMetrics(reg),
		stop:    make(chan struct{}),
		appSvc:  appSvc,
	}

	if c.NewBadger == nil {
		c.NewBadger = s.newBadger
	}

	var err error
	if s.main, err = c.NewBadger("main", "", nil); err != nil {
		return nil, err
	}
	if s.dicts, err = c.NewBadger("dicts", dictionaryPrefix, dictionaryCodec{}); err != nil {
		return nil, err
	}
	if s.dimensions, err = c.NewBadger("dimensions", dimensionPrefix, dimensionCodec{}); err != nil {
		return nil, err
	}
	if s.segments, err = c.NewBadger("segments", segmentPrefix, segmentCodec{}); err != nil {
		return nil, err
	}
	if s.trees, err = c.NewBadger("trees", treePrefix, treeCodec{s}); err != nil {
		return nil, err
	}

	pdb, err := c.NewBadger("profiles", exemplarDataPrefix, nil)
	if err != nil {
		return nil, err
	}

	s.initExemplarsStorage(pdb)
	s.labels = labels.New(s.main.DBInstance())

	if err = s.migrate(); err != nil {
		return nil, err
	}

	s.periodicTask(s.writeBackTaskInterval, s.writeBackTask)

	if !s.config.inMemory {
		// TODO(kolesnikovae): Allow failure and skip evictionTask?
		memTotal, err := getMemTotal()
		if err != nil {
			return nil, err
		}

		s.periodicTask(s.evictionTaskInterval, s.evictionTask(memTotal))
		s.maintenanceTask(s.retentionTaskInterval, s.retentionTask)
		s.periodicTask(s.metricsUpdateTaskInterval, s.updateMetricsTask)
	}

	return s, nil
}

func (s *Storage) Close() error {
	// Stop all periodic and maintenance tasks.
	close(s.stop)
	s.logger.Debug("waiting for storage tasks to finish")
	s.tasksWG.Wait()
	s.logger.Debug("storage tasks finished")

	// Flush caches. Dictionaries DB has to close last because trees depends on it.
	// Exemplars DB does not have a cache but depends on Dictionaries DB as well:
	// there is no need to force synchronization, as exemplars storage listens to
	// the s.stop channel and stops synchronously.
	caches := []BadgerDBWithCache{
		s.trees,
		s.segments,
		s.dimensions,
	}
	wg := new(sync.WaitGroup)
	wg.Add(len(caches))
	for _, d := range caches {
		go func(d BadgerDBWithCache) {
			d.CacheInstance().Flush()
			wg.Done()
		}(d)
	}
	wg.Wait()

	// Flush dictionaries cache only when all the dependant caches are flushed.
	s.dicts.CacheInstance().Flush()

	// Close databases. Order does not matter.
	dbs := []BadgerDBWithCache{
		s.trees,
		s.segments,
		s.dimensions,
		s.exemplars.db,
		s.dicts,
		s.main, // Also stores labels.
	}
	wg = new(sync.WaitGroup)
	wg.Add(len(dbs))
	for _, d := range dbs {
		go func(d BadgerDBWithCache) {
			defer wg.Done()
			if err := d.DBInstance().Close(); err != nil {
				s.logger.WithField("name", d.Name()).WithError(err).Error("closing database")
			}
		}(d)
	}
	wg.Wait()
	return nil
}

func (s *Storage) DiskUsage() map[string]bytesize.ByteSize {
	m := make(map[string]bytesize.ByteSize)
	for _, d := range s.databases() {
		m[d.Name()] = d.Size()
	}
	return m
}

func (s *Storage) CacheStats() map[string]uint64 {
	m := make(map[string]uint64)
	for _, d := range s.databases() {
		if d.CacheInstance() != nil {
			m[d.Name()] = d.CacheSize()
		}
	}
	return m
}

func (s *Storage) withContext(fn func(context.Context)) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		select {
		case <-ctx.Done():
		case <-s.stop:
			cancel()
		}
	}()
	fn(ctx)
}

// maintenanceTask periodically runs f exclusively.
func (s *Storage) maintenanceTask(interval time.Duration, f func()) {
	s.periodicTask(interval, func() {
		s.tasksMutex.Lock()
		defer s.tasksMutex.Unlock()
		f()
	})
}

func (s *Storage) periodicTask(interval time.Duration, f func()) {
	s.tasksWG.Add(1)
	go func() {
		timer := time.NewTimer(interval)
		defer func() {
			timer.Stop()
			s.tasksWG.Done()
		}()
		select {
		case <-s.stop:
			return
		default:
			f()
		}
		for {
			select {
			case <-s.stop:
				return
			case <-timer.C:
				f()
				timer.Reset(interval)
			}
		}
	}()
}

func (s *Storage) evictionTask(memTotal uint64) func() {
	var m runtime.MemStats
	return func() {
		timer := prometheus.NewTimer(prometheus.ObserverFunc(s.metrics.evictionTaskDuration.Observe))
		defer timer.ObserveDuration()
		runtime.ReadMemStats(&m)
		used := float64(m.Alloc) / float64(memTotal)
		percent := s.config.cacheEvictVolume
		if used < s.config.cacheEvictThreshold {
			return
		}
		// Dimensions, dictionaries, and segments should not be evicted,
		// as they are almost 100% in use and will be loaded back, causing
		// more allocations. Unused items should be unloaded from cache by
		// TTL expiration. Although, these objects must be written to disk,
		// the order matters.
		//
		// It should be noted that in case of a crash or kill, data may become
		// inconsistent: we should unite databases and do this in a tx.
		// This is also applied to writeBack task.
		s.trees.Evict(percent)
		s.dicts.WriteBack()
		// s.dimensions.WriteBack()
		// s.segments.WriteBack()
		// GC does not really release OS memory, so relying on MemStats.Alloc
		// causes cache to evict the vast majority of items. debug.FreeOSMemory()
		// could be used instead, but this can be even more expensive.
		runtime.GC()
	}
}

func (s *Storage) writeBackTask() {
	timer := prometheus.NewTimer(prometheus.ObserverFunc(s.metrics.writeBackTaskDuration.Observe))
	defer timer.ObserveDuration()
	for _, d := range s.databases() {
		if d.CacheInstance() != nil {
			d.WriteBack()
		}
	}
}

func (s *Storage) updateMetricsTask() {
	for _, d := range s.databases() {
		s.metrics.dbSize.WithLabelValues(d.Name()).Set(float64(d.Size()))
		if d.CacheInstance() != nil {
			s.metrics.cacheSize.WithLabelValues(d.Name()).Set(float64(d.CacheSize()))
		}
	}
}

func (s *Storage) retentionTask() {
	rp := s.retentionPolicy()
	if !rp.LowerTimeBoundary().IsZero() {
		s.withContext(func(ctx context.Context) {
			s.enforceRetentionPolicy(ctx, rp)
		})
	}
}

func (s *Storage) exemplarsRetentionTask() {
	rp := s.retentionPolicy()
	if !rp.ExemplarsRetentionTime.IsZero() {
		s.withContext(func(ctx context.Context) {
			s.exemplars.enforceRetentionPolicy(ctx, rp)
		})
	}
}

func (s *Storage) retentionPolicy() *segment.RetentionPolicy {
	exemplarsRetention := s.config.retentionExemplars
	if exemplarsRetention == 0 {
		exemplarsRetention = s.config.retention
	}
	return segment.NewRetentionPolicy().
		SetAbsolutePeriod(s.config.retention).
		SetExemplarsRetentionPeriod(exemplarsRetention).
		SetLevels(
			s.config.retentionLevels.Zero,
			s.config.retentionLevels.One,
			s.config.retentionLevels.Two)
}

func (s *Storage) databases() []BadgerDBWithCache {
	return []BadgerDBWithCache{
		s.main,
		s.dimensions,
		s.segments,
		s.dicts,
		s.trees,
		s.exemplars.db,
	}
}

func (s *Storage) SegmentsInternals() (*badger.DB, *cache.Cache) {
	return s.segments.DBInstance(), s.segments.CacheInstance()
}
func (s *Storage) DimensionsInternals() (*badger.DB, *cache.Cache) {
	return s.dimensions.DBInstance(), s.dimensions.CacheInstance()
}
func (s *Storage) DictsInternals() (*badger.DB, *cache.Cache) {
	return s.dicts.DBInstance(), s.dicts.CacheInstance()
}
func (s *Storage) TreesInternals() (*badger.DB, *cache.Cache) {
	return s.trees.DBInstance(), s.trees.CacheInstance()
}
func (s *Storage) MainInternals() (*badger.DB, *cache.Cache) {
	return s.main.DBInstance(), s.main.CacheInstance()
}
func (s *Storage) ExemplarsInternals() (*badger.DB, func()) {
	return s.exemplars.db.DBInstance(), s.exemplars.Sync
}
