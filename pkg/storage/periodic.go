package storage

import (
	"runtime"
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
				cb()
				timer.Reset(interval)
			}
		}
	}
}

func (*Storage) badgerGCTask(db *badger.DB) func() {
	return func() {
		logrus.Debug("starting badger garbage collection")
		for {
			if err := db.RunValueLogGC(0.7); err != nil {
				return
			}
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
		if used > s.config.CacheEvictThreshold {
			func() {
				timer := prometheus.NewTimer(prometheus.ObserverFunc(func(v float64) {
					logrus.Debugf("eviction task took %f seconds\n", v)
					s.evictionsTimer.Observe(v)
				}))
				defer timer.ObserveDuration()

				s.dimensions.Evict(percent / 4)
				s.dicts.Evict(percent / 4)
				s.segments.Evict(percent / 2)
				s.trees.Evict(percent)
				runtime.GC()
			}()
		}
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

	if err := s.DeleteDataBefore(s.lifetimeBasedRetentionThreshold()); err != nil {
		logrus.WithError(err).Warn("retention task failed")
	}
}
