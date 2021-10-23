package storage

import (
	"errors"
	"runtime"
	"runtime/debug"
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
	*dbMetrics

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

		logger:    logger,
		metrics:   newStorageMetrics(reg),
		dbMetrics: newCacheMetrics(reg),
		stop:      make(chan struct{}),
	}

	for _, option := range options {
		option(s)
	}

	var err error
	if s.main, err = s.newBadger("main", "", nil); err != nil {
		return nil, err
	}
	if s.dicts, err = s.newBadger("dicts", dictionaryPrefix, dictionaryCodec{}); err != nil {
		return nil, err
	}
	if s.dimensions, err = s.newBadger("dimensions", dimensionPrefix, dimensionCodec{}); err != nil {
		return nil, err
	}
	if s.segments, err = s.newBadger("segments", segmentPrefix, segmentCodec{}); err != nil {
		return nil, err
	}
	if s.trees, err = s.newBadger("trees", treePrefix, treeCodec{s}); err != nil {
		return nil, err
	}

	s.labels = labels.New(s.main.DB)

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
	go s.periodicTask(s.writeBackInterval, s.updateMetricsTask)

	return s, nil
}

func (s *Storage) Close() error {
	// Stop all periodic and maintenance tasks.
	close(s.stop)
	s.wg.Wait()
	// Dictionaries DB has to close last because trees depend on it.
	s.goDB(func(d *db) {
		if d != s.dicts {
			d.close()
		}
	})
	s.dicts.close()
	return nil
}

func (s *Storage) DiskUsage() map[string]bytesize.ByteSize {
	m := make(map[string]bytesize.ByteSize)
	for _, d := range s.databases() {
		m[d.name] = dbSize(d)
	}
	return m
}

func (s *Storage) CacheStats() map[string]uint64 {
	m := make(map[string]uint64)
	for _, d := range s.databases() {
		if d.Cache != nil {
			m[d.name+"_size"] = s.dimensions.Cache.Size()
		}
	}
	return m
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
		// as they are almost 100% in use. Unused items should be unloaded
		// from cache by TTL expiration. Although, these objects must be
		// written to disk (order matters).
		//
		// It should be noted that in case of a crash or kill, data may become
		// inconsistent: we should unite databases and do this in a transaction.
		// This is also applied to writeBack task.
		s.dimensions.WriteBack()
		s.segments.WriteBack()
		s.dicts.WriteBack()
		s.trees.Evict(percent)
		debug.FreeOSMemory()
	}
}

func (s *Storage) updateMetricsTask() {
	// TODO(kolesnikovae): update disk and cache size metrics.
	// s.logger.WithFields(s.DiskUsage()).Debug("disk stats")
	// s.logger.WithFields(s.CacheStats()).Debug("cache stats")
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
		n := dbSize(s.databases()...)
		if s.size-n > diff {
			f()
		}
		s.size = n
	}
}
