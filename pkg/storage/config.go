package storage

import (
	"time"

	"github.com/sirupsen/logrus"

	"github.com/pyroscope-io/pyroscope/pkg/config"
	"github.com/pyroscope-io/pyroscope/pkg/storage/cache"
	"github.com/pyroscope-io/pyroscope/pkg/util/bytesize"
)

type Config struct {
	badgerLogLevel          logrus.Level
	badgerNoTruncate        bool
	badgerBasePath          string
	cacheEvictThreshold     float64
	cacheEvictVolume        float64
	maxNodesSerialization   int
	hideApplications        []string
	inMemory                bool
	retention               time.Duration
	retentionExemplars      time.Duration
	retentionLevels         config.RetentionLevels
	queueWorkers            int
	queueSize               int
	exemplarsBatchSize      int
	exemplarsBatchQueueSize int
	exemplarsBatchDuration  time.Duration

	// BadgerDB value log sampling threshold. When a log file is picked
	// for GC, it is first sampled. If the sample shows that we discard
	// at least discardRatio space of that file, it would be rewritten.
	// Setting it to higher value would result in fewer space reclaims,
	// while setting it to a lower value would result in more space
	// reclaims at the cost of increased activity on the LSM tree.
	badgerGCDiscardRatio float64
	// Interval at which GC triggered if the db size has increased more
	// than by badgerGCSizeDiff since the last probe.
	badgerGCTaskInterval time.Duration
	// badgerGCSizeDiff specifies the minimal storage size difference that
	// causes garbage collection to trigger.
	badgerGCSizeDiff   bytesize.ByteSize
	badgerValueLogSize bytesize.ByteSize
	// DB size and cache size metrics are updated periodically.
	metricsUpdateTaskInterval time.Duration
	writeBackTaskInterval     time.Duration
	evictionTaskInterval      time.Duration
	retentionTaskInterval     time.Duration
	// Cached items are evicted and written to disk on the TTL expiration.
	cacheTTL time.Duration

	NewBadger func(name string, p Prefix, codec cache.Codec) (BadgerDBWithCache, error)
}

func (c *Config) setDefaults() {
	if c.badgerGCDiscardRatio == 0 {
		c.badgerGCDiscardRatio = 0.7
	}
	if c.badgerGCTaskInterval == 0 {
		c.badgerGCTaskInterval = 5 * time.Minute
	}
	if c.badgerGCSizeDiff == 0 {
		c.badgerGCSizeDiff = 1 << 30
	}
	if c.badgerValueLogSize == 0 {
		c.badgerValueLogSize = 1 << 30
	}
	if c.metricsUpdateTaskInterval == 0 {
		c.metricsUpdateTaskInterval = 10 * time.Second
	}
	if c.writeBackTaskInterval == 0 {
		c.writeBackTaskInterval = time.Minute
	}
	if c.evictionTaskInterval == 0 {
		c.evictionTaskInterval = 20 * time.Second
	}
	if c.retentionTaskInterval == 0 {
		c.retentionTaskInterval = 10 * time.Minute
	}
	if c.cacheTTL == 0 {
		c.cacheTTL = 2 * time.Minute
	}
}

// NewConfig returns a new storage config from a server config
func NewConfig(server *config.Server) *Config {
	level := logrus.ErrorLevel
	if l, err := logrus.ParseLevel(server.BadgerLogLevel); err == nil {
		level = l
	}
	return &Config{
		badgerLogLevel:          level,
		badgerNoTruncate:        server.BadgerNoTruncate,
		badgerBasePath:          server.StoragePath,
		cacheEvictThreshold:     server.CacheEvictThreshold,
		cacheEvictVolume:        server.CacheEvictVolume,
		maxNodesSerialization:   server.MaxNodesSerialization,
		hideApplications:        server.HideApplications,
		inMemory:                false,
		retention:               server.Retention,
		retentionExemplars:      server.ExemplarsRetention,
		retentionLevels:         server.RetentionLevels,
		queueWorkers:            server.StorageQueueWorkers,
		queueSize:               server.StorageQueueSize,
		exemplarsBatchSize:      server.ExemplarsBatchSize,
		exemplarsBatchQueueSize: server.ExemplarsBatchQueueSize,
		exemplarsBatchDuration:  server.ExemplarsBatchDuration,

		badgerGCDiscardRatio:      server.BadgerGCDiscardRatio,
		badgerGCTaskInterval:      server.BadgerGCTaskInterval,
		badgerGCSizeDiff:          server.BadgerGCSizeDiff,
		badgerValueLogSize:        server.BadgerValueLogSize,
		metricsUpdateTaskInterval: server.MetricsUpdateTaskInterval,
		writeBackTaskInterval:     server.WriteBackTaskInterval,
		evictionTaskInterval:      server.EvictionTaskInterval,
		retentionTaskInterval:     server.RetentionTaskInterval,
		cacheTTL:                  server.CacheTTL,

		NewBadger: nil,
	}
}

// WithPath sets the storage base path
func (c *Config) WithPath(path string) *Config {
	c.badgerBasePath = path
	return c
}

// WithInMemory makes the storage in-memory.
func (c *Config) WithInMemory() *Config {
	c.inMemory = true
	return c
}
