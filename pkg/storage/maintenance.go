package storage

import (
	"runtime"
	"runtime/debug"
	"sync"
	"time"

	"github.com/pyroscope-io/pyroscope/pkg/util/bytesize"
)

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
}

func (s *Storage) writeBackTask() {
	for _, d := range s.databases() {
		if d.Cache != nil {
			d.WriteBack()
		}
	}
}

func (s *Storage) runGC(discardRatio float64) {
	m := new(sync.Mutex)
	reclaimed := make(map[string]interface{})
	s.goDB(func(x *db) {
		if x.runGC(discardRatio) {
			m.Lock()
			reclaimed[x.name] = true
			m.Unlock()
		}
	})

	s.logger.WithFields(reclaimed).Info("badger db garbage collection")
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
