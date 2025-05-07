package compactor

import (
	"flag"
	"time"
)

type Config struct {
	Levels []LevelConfig

	CleanupBatchSize   int32
	CleanupDelay       time.Duration
	CleanupJobMinLevel int32
	CleanupJobMaxLevel int32
}

type LevelConfig struct {
	MaxBlocks uint
	MaxAge    int64
}

func DefaultConfig() Config {
	return Config{
		Levels: []LevelConfig{
			{MaxBlocks: 20, MaxAge: int64(1 * 36 * time.Second)},
			{MaxBlocks: 10, MaxAge: int64(2 * 360 * time.Second)},
			{MaxBlocks: 10, MaxAge: int64(3 * 3600 * time.Second)},
		},

		CleanupBatchSize:   2,
		CleanupDelay:       15 * time.Minute,
		CleanupJobMaxLevel: 1,
		CleanupJobMinLevel: 0,
	}
}

func (c *Config) RegisterFlagsWithPrefix(string, *flag.FlagSet) {
	// NOTE(kolesnikovae): I'm not sure if making this configurable
	// is a good idea; however, we might want to add a flag to tune
	// the parameters based on e.g., segment size or max duration.
	*c = DefaultConfig()
}

// exceedsSize is called after the block has been added to the batch.
// If the function returns true, the batch is flushed to the global
// queue and becomes available for compaction.
func (c *Config) exceedsMaxSize(b *batch) bool {
	return uint(b.size) >= c.maxBlocks(b.staged.key.level)
}

// exceedsAge reports whether the batch update time is older than the
// maximum age for the level threshold. The function is used in two
// cases: if the batch is not flushed to the global queue and is the
// oldest one, or if the batch is flushed (and available to the planner)
// but the job plan is not complete yet.
func (c *Config) exceedsMaxAge(b *batch, now int64) bool {
	if m := c.maxAge(b.staged.key.level); m > 0 {
		age := now - b.createdAt
		return age > m
	}
	return false
}

func (c *Config) maxBlocks(l uint32) uint {
	if l < uint32(len(c.Levels)) {
		return c.Levels[l].MaxBlocks
	}
	return 0
}

func (c *Config) maxAge(l uint32) int64 {
	if l < uint32(len(c.Levels)) {
		return c.Levels[l].MaxAge
	}
	return 0
}

func (c *Config) maxLevel() uint32 {
	// Assuming that there is at least one level.
	return uint32(len(c.Levels) - 1)
}
