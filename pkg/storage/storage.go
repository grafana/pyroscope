package storage

import (
	"errors"
	"os"
	"path/filepath"
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

type storageOptions struct {
	metricsUpdateInterval time.Duration
	writeBackInterval     time.Duration
	evictInterval         time.Duration
	cacheTTL              time.Duration

	gcInterval       time.Duration
	gcSizeDiff       bytesize.ByteSize
	reclaimSizeRatio float64
}

func New(c *config.Server, logger *logrus.Logger, reg prometheus.Registerer) (*Storage, error) {
	s := &Storage{
		config: c,
		storageOptions: &storageOptions{
			metricsUpdateInterval: 10 * time.Second,
			writeBackInterval:     time.Minute, // 10 * time.Second,
			evictInterval:         20 * time.Second,
			cacheTTL:              2 * time.Minute,

			// Interval at which GC happen if the db size has increase more
			// than by gcSizeDiff since the last probe.
			gcInterval: 5 * time.Minute, // 10 * time.Second,
			// gcSizeDiff specifies the minimal storage size difference that
			// causes garbage collection to trigger.
			gcSizeDiff: 100 * bytesize.MB, // 0,
			// reclaimSizeRatio determines the share of the storage size limit
			// to be reclaimed when size-based retention policy enforced. The
			// volume to reclaim is calculated as follows:
			//   used - limit + limit*ratio.
			reclaimSizeRatio: 0.05,
		},

		logger:  logger,
		metrics: newMetrics(reg),
		stop:    make(chan struct{}),
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
		d := d
		go func() {
			d.Cache.Flush()
			wg.Done()
		}()
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
		// inconsistent: we should unite databases and do this in a tx.
		// This is also applied to writeBack task.
		s.trees.Evict(percent)
		s.dicts.WriteBack()
		s.dimensions.WriteBack()
		s.segments.WriteBack()
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

// watchDBSize keeps track of the database size and call f once it's size
// increases by diff. Function f must call garbage collection.
func (s *Storage) watchDBSize(diff bytesize.ByteSize, f func()) func() {
	return func() {
		n := s.calculateDBSize()
		d := n - s.size // Size diff since the last gc.
		fields := logrus.Fields{"current": n}
		if s.size != 0 {
			fields["diff"] = d
			fields["last-gc"] = s.size
		}
		s.logger.WithFields(fields).Info("db size watcher")
		if d > diff {
			// The value should be updated regardless of whether GC reclaimed
			// any space: if it did not, GC is to be called next time the diff
			// exceeds the allowed value.
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

// TODO(kolesnikovae): filepath.Walk is notoriously slow.
//  Consider use of https://github.com/karrick/godirwalk.
//  Although, every badger.DB calculates its size (reported
//  via Size) in the same way every minute.
func (s *Storage) calculateDBSize() bytesize.ByteSize {
	var size int64
	p := filepath.Join(s.config.StoragePath, "pyroscope")
	_ = filepath.Walk(p, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		switch filepath.Ext(path) {
		case ".sst", ".vlog":
			size += info.Size()
		}
		return nil
	})
	return bytesize.ByteSize(size)
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
