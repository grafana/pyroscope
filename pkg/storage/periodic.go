package storage

import (
	"context"
	"runtime"
	"time"

	"github.com/dgraph-io/badger/v2"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"
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

func defaultBadgerGCTask(db *badger.DB, logger logrus.FieldLogger) func() {
	return func() { runBadgerGC(db, logger) }
}

func runBadgerGC(db *badger.DB, logger logrus.FieldLogger) (reclaimed bool) {
	// TODO(kolesnikovae): implement size check - run GC only when
	//  used disk space (db.Size) has increased by some value?
	logger.Debug("starting badger garbage collection")
	// BadgerDB uses 2 compactors by default.
	if err := db.Flatten(2); err != nil {
		logger.WithError(err).Error("failed to flatten database")
	}
	for {
		switch err := db.RunValueLogGC(0.5); err {
		default:
			logger.WithError(err).Warn("failed to run GC")
			return false
		case badger.ErrNoRewrite:
			return false
		case nil:
			reclaimed = true
			continue
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
		// At eviction some trees are persisted. To ensure all
		// the symbols are present in the database, dictionaries
		// must be written first. Otherwise, in case of a crash,
		// dictionaries may be corrupted.
		s.dicts.WriteBack()
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

func (s *Storage) retentionTask(ctx context.Context) func() {
	timer := prometheus.NewTimer(prometheus.ObserverFunc(func(v float64) {
		logrus.Debugf("retention task %f seconds\n", v)
		s.retentionTimer.Observe(v)
	}))
	defer timer.ObserveDuration()
	return func() {
		if err := s.DeleteDataBefore(ctx, s.retentionPolicy()); err != nil {
			logrus.WithError(err).Warn("retention task failed")
		}
	}
}
