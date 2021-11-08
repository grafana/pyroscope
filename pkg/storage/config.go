package storage

import (
	"time"

	"github.com/pyroscope-io/pyroscope/pkg/config"
)

type Config struct {
	badgerLogLevel        string
	badgerNoTruncate      bool
	badgerBasePath        string
	cacheEvictThreshold   float64
	cacheEvictVolume      float64
	maxNodesSerialization int
	retention             time.Duration
	hideApplications      []string
}

// NewConfig returns a new storage config from a server config
func NewConfig(server *config.Server) *Config {
	return &Config{
		badgerLogLevel:        server.BadgerLogLevel,
		badgerBasePath:        server.StoragePath,
		badgerNoTruncate:      server.BadgerNoTruncate,
		cacheEvictThreshold:   server.CacheEvictThreshold,
		cacheEvictVolume:      server.CacheEvictVolume,
		maxNodesSerialization: server.MaxNodesSerialization,
		retention:             server.Retention,
		hideApplications:      server.HideApplications,
	}
}

// WithPath sets the storage base path
func (c *Config) WithPath(path string) *Config {
	c.badgerBasePath = path
	return c
}
