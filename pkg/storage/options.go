package storage

import (
	"time"

	"github.com/pyroscope-io/pyroscope/pkg/util/bytesize"
)

type Option func(*Storage)

type storageOptions struct {
	writeBackInterval time.Duration
	evictInterval     time.Duration
	cacheTTL          time.Duration

	gcInterval       time.Duration
	gcSizeDiff       bytesize.ByteSize
	reclaimSizeRatio float64
}

func defaultOptions() *storageOptions {
	return &storageOptions{
		writeBackInterval: time.Minute,
		evictInterval:     20 * time.Second,
		cacheTTL:          2 * time.Minute,

		gcInterval:       5 * time.Minute,
		gcSizeDiff:       bytesize.GB,
		reclaimSizeRatio: 0.05,
	}
}

func WithWriteBackInterval(interval time.Duration) Option {
	return func(s *Storage) {
		s.writeBackInterval = interval
	}
}

func WithEvictionInterval(interval time.Duration) Option {
	return func(s *Storage) {
		s.evictInterval = interval
	}
}

func WithCacheTTL(cacheTTL time.Duration) Option {
	return func(s *Storage) {
		s.cacheTTL = cacheTTL
	}
}

func WithGCInterval(interval time.Duration) Option {
	return func(s *Storage) {
		s.gcInterval = interval
	}
}

// WithMinSizeDiffGC specifies the minimal storage size difference that causes
// garbage collection to trigger.
//
// Default value is 1GB.
func WithMinSizeDiffGC(sizeDiff bytesize.ByteSize) Option {
	return func(s *Storage) {
		s.gcSizeDiff = sizeDiff
	}
}

// WithReclaimSizeRatio specifies the share of the storage size limit to be
// reclaimed when size-based retention policy enforced. The volume to reclaim
// is calculated as follows: used - limit + limit*ratio.
//
// Default value is 5%.
// TODO(kolesnikovae): It may make sense to allow setting an absolute upper boundary.
func WithReclaimSizeRatio(ratio float64) Option {
	return func(s *Storage) {
		s.reclaimSizeRatio = ratio
	}
}
