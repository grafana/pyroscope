package storage

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"time"

	"github.com/dgraph-io/badger/v2"
	"github.com/sirupsen/logrus"

	"github.com/prometheus/client_golang/prometheus"
)

func (s *Storage) periodicTask(interval time.Duration, cb func()) {
	timer := time.NewTimer(interval)
	defer func() {
		timer.Stop()
		s.wg.Done()
	}()
	for {
		select {
		case <-s.stop:
			return
		case <-timer.C:
			select {
			case <-s.stop:
				return
			default:
				s.maintenance.Lock()
				cb()
				s.maintenance.Unlock()
				timer.Reset(interval)
			}
		}
	}
}

func (s *Storage) badgerGCTask() {
	databases := []*badger.DB{
		s.dbTrees,
		s.dbDicts,
		s.dbDimensions,
		s.dbSegments,
		s.db,
	}
	var wg sync.WaitGroup
	for _, db := range databases {
		db := db
		wg.Add(1)
		go func() {
			defer wg.Done()
			runBadgerGC(logrus.New(), db)
		}()
	}
	wg.Wait()
}

func runBadgerGC(logger *logrus.Logger, db *badger.DB) {
	fmt.Println("> starting GC")
	lsm, vlog := db.Size()
	fmt.Println(">>> before", lsm, vlog)
	defer func() {
		lsm, vlog = db.Size()
		fmt.Println(">>> after", lsm, vlog)
	}()
	if err := db.Flatten(runtime.NumCPU()); err != nil {
		logger.WithError(err).Error("failed to flatten database")
	}

	logger.Debug("starting badger garbage collection")
	for {
		if err := db.RunValueLogGC(0.5); err != nil {
			return
		}
	}
}

func (s *Storage) evictionTask(memTotal uint64) func() {
	var m runtime.MemStats
	return func() {
		runtime.ReadMemStats(&m)
		used := float64(m.Alloc) / float64(memTotal)

		s.evictionsAllocBytes.Set(float64(m.Alloc))
		s.evictionsTotalBytes.Set(float64(memTotal))

		percent := s.config.CacheEvictVolume
		if used < s.config.CacheEvictThreshold {
			return
		}

		timer := prometheus.NewTimer(prometheus.ObserverFunc(func(v float64) {
			logrus.Debugf("eviction task took %f seconds\n", v)
			s.evictionsTimer.Observe(v)
		}))
		// TODO(kolesnikovae): at eviction some trees are persisted,
		//   in case of a crash, dictionaries may be corrupted.
		s.trees.Evict(percent)
		timer.ObserveDuration()
		// Do not evict those as it will cause even more allocations
		// to serialize and then load them back again.
		// s.dimensions.Evict(percent / 4)
		// s.dicts.Evict(percent / 4)
		// s.segments.Evict(percent / 2)
		runtime.GC()
	}
}

func (s *Storage) writeBackTask() {
	timer := prometheus.NewTimer(prometheus.ObserverFunc(func(v float64) {
		logrus.Debugf("writeback task took %f seconds\n", v)
		s.writeBackTimer.Observe(v)
	}))
	defer timer.ObserveDuration()
	s.dimensions.WriteBack()
	s.segments.WriteBack()
	s.dicts.WriteBack()
	s.trees.WriteBack()
}

func (s *Storage) retentionTask() {
	timer := prometheus.NewTimer(prometheus.ObserverFunc(func(v float64) {
		logrus.Debugf("retention task %f seconds\n", v)
		s.retentionTimer.Observe(v)
	}))
	defer timer.ObserveDuration()

	if err := s.DeleteDataBefore(context.TODO(), s.retentionPolicy()); err != nil {
		logrus.WithError(err).Warn("retention task failed")
	}

	// TODO(kolesnikovae): Wait for GC to clean value log files.
	if err := s.Reclaim(context.TODO(), s.retentionPolicy()); err != nil {
		logrus.WithError(err).Warn("retention task failed")
	}
}
