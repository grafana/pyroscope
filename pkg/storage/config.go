package storage

import (
	"time"

	"github.com/pyroscope-io/pyroscope/pkg/config"
	"github.com/sirupsen/logrus"
)

type Config struct {
	badgerLogLevel        logrus.Level
	badgerNoTruncate      bool
	badgerBasePath        string
	cacheEvictThreshold   float64
	cacheEvictVolume      float64
	maxNodesSerialization int
	hideApplications      []string
	inMemory              bool
	retention             time.Duration
	retentionExemplars    time.Duration
	retentionLevels       config.RetentionLevels
}

// NewConfig returns a new storage config from a server config
func NewConfig(server *config.Server) *Config {
	level := logrus.ErrorLevel
	if l, err := logrus.ParseLevel(server.BadgerLogLevel); err == nil {
		level = l
	}
	return &Config{
		badgerLogLevel:        level,
		badgerBasePath:        server.StoragePath,
		badgerNoTruncate:      server.BadgerNoTruncate,
		cacheEvictThreshold:   server.CacheEvictThreshold,
		cacheEvictVolume:      server.CacheEvictVolume,
		maxNodesSerialization: server.MaxNodesSerialization,
		retention:             server.Retention,
		retentionExemplars:    server.ExemplarsRetention,
		retentionLevels:       server.RetentionLevels,
		hideApplications:      server.HideApplications,
		inMemory:              false,
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
