package storage

import (
	"runtime"
	"time"

	"github.com/pyroscope-io/pyroscope/pkg/util/metrics"
)

func (s *Storage) periodicTask(interval time.Duration, cb func()) {
	ticker := time.NewTimer(interval)
	defer ticker.Stop()

	for range ticker.C {
		closing := func() bool {
			s.closingMutex.RLock()
			defer s.closingMutex.RUnlock()

			return s.closing
		}()

		if closing {
			return
		}

		cb()

		ticker.Reset(interval)
	}
}

func (s *Storage) startEvictTimer(interval time.Duration) error {
	// load the total memory of the server
	memTotal, err := getMemTotal()
	if err != nil {
		return err
	}

	go s.periodicTask(interval, func() {
		// read the allocated memory used by application
		var m runtime.MemStats
		runtime.ReadMemStats(&m)

		used := float64(m.Alloc) / float64(memTotal)

		metrics.Gauge("evictions_alloc_bytes", m.Alloc)
		metrics.Gauge("evictions_total_bytes", memTotal)
		metrics.Gauge("evictions_used_perc", used)

		percent := s.cfg.CacheEvictVolume
		if used > s.cfg.CacheEvictThreshold {
			metrics.Timing("evictions_timer", func() {
				metrics.Count("evictions_count", 1)

				s.dimensions.Evict(percent / 4)
				s.dicts.Evict(percent / 4)
				s.segments.Evict(percent / 2)
				s.trees.Evict(percent)

				// force gc after eviction
				runtime.GC()
			})
		}

	})

	return nil
}

func (s *Storage) startWriteBackTimer(interval time.Duration) error {
	go s.periodicTask(interval, func() {
		metrics.Timing("write_back_timer", func() {
			metrics.Count("write_back_count", 1)
			s.dimensions.WriteBack()
			s.segments.WriteBack()
			s.dicts.WriteBack()
			s.trees.WriteBack()
			runtime.GC()
		})
	})

	return nil
}

func (s *Storage) startRetentionTimer(interval time.Duration) error {
	if s.cfg.Retention == 0 {
		return nil
	}

	go s.periodicTask(interval, func() {
		metrics.Timing("retention_timer", func() {
			metrics.Count("retention_count", 1)
			s.DeleteDataBefore(s.lifetimeBasedRetentionThreshold())
		})
	})

	return nil
}
