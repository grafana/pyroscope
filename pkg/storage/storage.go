package storage

import (
	"errors"
	"runtime"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"

	"github.com/pyroscope-io/pyroscope/pkg/config"
	"github.com/pyroscope-io/pyroscope/pkg/storage/labels"
	"github.com/pyroscope-io/pyroscope/pkg/util/bytesize"
)

var (
	errRetention = errors.New("could not write because of retention settings")
	errClosed    = errors.New("storage closed")
)

var (
	maxTime  = time.Unix(1<<62, 999999999)
	zeroTime time.Time
)

type Storage struct {
	config *config.Server
	*storageOptions

	logger *logrus.Logger
	*metrics

	segments   *db
	dimensions *db
	dicts      *db
	trees      *db
	main       *db
	labels     *labels.Labels

	size bytesize.ByteSize

	// Maintenance tasks are executed exclusively to avoid competition:
	// extensive writing during GC is harmful and deteriorates the
	// overall performance. Same for write back, eviction, and retention
	// tasks.
	maintenance sync.Mutex
	stop        chan struct{}
	wg          sync.WaitGroup

	putMutex sync.Mutex
}

func New(c *config.Server, logger *logrus.Logger, reg prometheus.Registerer, options ...Option) (*Storage, error) {
	s := &Storage{
		config:         c,
		storageOptions: defaultOptions(),

		logger:  logger,
		metrics: newMetrics(reg),
		stop:    make(chan struct{}),
	}

	for _, option := range options {
		option(s)
	}

	badgerDB, err := s.openBadgerDB("pyroscope")
	if err != nil {
		return nil, err
	}

	s.main = s.newDB(badgerDB, "main", "", nil)
	s.labels = labels.New(s.main.DB)

	s.dicts = s.newDB(badgerDB, "dicts", dictionaryPrefix, dictionaryCodec{})
	s.dimensions = s.newDB(badgerDB, "dimensions", dimensionPrefix, dimensionCodec{})
	s.segments = s.newDB(badgerDB, "segments", segmentPrefix, segmentCodec{})
	s.trees = s.newDB(badgerDB, "trees", treePrefix, treeCodec{s})

	if err = s.migrate(); err != nil {
		return nil, err
	}

	// TODO(kolesnikovae): Allow failure and skip evictionTask?
	memTotal, err := getMemTotal()
	if err != nil {
		return nil, err
	}

	// TODO(kolesnikovae): Make it possible to run CollectGarbage
	//  without starting any other maintenance tasks at server start.
	s.wg.Add(4)
	go s.maintenanceTask(s.gcInterval, s.watchDBSize(s.gcSizeDiff, s.CollectGarbage))
	go s.maintenanceTask(s.evictInterval, s.evictionTask(memTotal))
	go s.maintenanceTask(s.writeBackInterval, s.writeBackTask)
	go s.periodicTask(s.metricsUpdateInterval, s.updateMetricsTask)

	return s, nil
}

func (s *Storage) Close() error {
	// Stop all periodic and maintenance tasks.
	close(s.stop)
	s.logger.Debug("waiting for storage tasks to finish")
	s.wg.Wait()
	s.logger.Debug("storage tasks finished")
	// Dictionaries DB has to close last because trees depend on it.
	dbs := []*db{
		s.dimensions,
		s.segments,
		s.trees,
		s.dicts,
	}
	wg := new(sync.WaitGroup)
	wg.Add(len(dbs))
	for _, d := range dbs {
		go func(db *db) {
			db.Cache.Flush()
			wg.Done()
		}(d)
	}
	wg.Wait()
	return s.main.DB.Close()
}

func (s *Storage) maintenanceTask(interval time.Duration, f func()) {
	s.periodicTask(interval, func() {
		s.maintenance.Lock()
		defer s.maintenance.Unlock()
		f()
	})
}

func (s *Storage) periodicTask(interval time.Duration, f func()) {
	timer := time.NewTimer(interval)
	defer func() {
		timer.Stop()
		s.wg.Done()
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
}

func (s *Storage) evictionTask(memTotal uint64) func() {
	var m runtime.MemStats
	return func() {
		runtime.ReadMemStats(&m)
		used := float64(m.Alloc) / float64(memTotal)
		percent := s.config.CacheEvictVolume
		if used < s.config.CacheEvictThreshold {
			return
		}

		// Dimensions, dictionaries, and segments should not be evicted,
		// as they are almost 100% in use and will be loaded back, causing
		// more allocations. Unused items should be unloaded from cache by
		// TTL expiration. Although, these objects must be written to disk,
		// order matters.
		//
		// It should be noted that in case of a crash or kill, data may become
		// inconsistent: we should unite databases and do this in a transaction.
		// This is also applied to writeBack task.
		s.dimensions.WriteBack()
		s.segments.WriteBack()
		s.dicts.WriteBack()
		s.trees.Evict(percent)
		// debug.FreeOSMemory()
		runtime.GC()
	}
}

func (s *Storage) writeBackTask() {
	for _, d := range s.databases() {
		if d.Cache != nil {
			d.WriteBack()
		}
	}
}

func (s *Storage) watchDBSize(diff bytesize.ByteSize, f func()) func() {
	return func() {
		n := s.main.size()
		s.logger.
			WithField("used", n).
			WithField("last-gc", s.size).
			Info("db size watcher")
		if s.size == 0 || s.size-n > diff {
			s.size = n
			f()
		}
	}
}

func (s *Storage) updateMetricsTask() {
	for _, d := range s.databases() {
		s.metrics.dbSize.WithLabelValues(d.name).Set(float64(d.size()))
		if d.Cache != nil {
			s.metrics.cacheSize.WithLabelValues(d.name).Set(float64(d.Cache.Size()))
		}
	}
}

func (s *Storage) databases() []*db {
	// Order matters.
	return []*db{
		s.main,
		s.dimensions,
		s.segments,
		s.dicts,
		s.trees,
	}
}

func (s *Storage) DiskUsage() map[string]bytesize.ByteSize {
	m := make(map[string]bytesize.ByteSize)
	for _, d := range s.databases() {
		m[d.name] = d.size()
	}
	return m
}

func (s *Storage) CacheStats() map[string]uint64 {
	m := make(map[string]uint64)
	for _, d := range s.databases() {
		if d.Cache != nil {
			m[d.name] = d.Cache.Size()
		}
	}
	return m
}
