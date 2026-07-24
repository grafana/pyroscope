package compactor

import (
	"flag"
	"time"
)

type Config struct {
	Levels []LevelConfig

	// MaxJobBytes bounds the total estimated input size (bytes) of a
	// compaction job, for levels above 0. A job completes once either
	// MaxBlocks or MaxJobBytes is reached, whichever happens first.
	// Level 0 is always exempt - see (*Config).maxBytes. 0 disables the
	// size-based limit.
	MaxJobBytes uint64 `yaml:"compaction_max_job_bytes" category:"advanced" doc:""`

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
		MaxJobBytes: 2 << 30,

		CleanupBatchSize:   2,
		CleanupDelay:       15 * time.Minute,
		CleanupJobMaxLevel: 1,
		CleanupJobMinLevel: 0,
	}
}

func (c *Config) RegisterFlagsWithPrefix(prefix string, f *flag.FlagSet) {
	// NOTE(kolesnikovae): I'm not sure if making this configurable
	// is a good idea; however, we might want to add a flag to tune
	// the parameters based on e.g., segment size or max duration.
	*c = DefaultConfig()
	f.Uint64Var(&c.MaxJobBytes, prefix+"compaction-max-job-bytes", c.MaxJobBytes,
		"Maximum estimated total input size, in bytes, of a compaction job for levels above 0. "+
			"A job completes once either the block count or this byte limit is reached, whichever happens "+
			"first; a single block already larger than this limit still forms a valid, single-block job. "+
			"Does not apply to level 0. 0 disables the size-based limit.")
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

// maxBytes returns the job byte limit for level l;
// level 0 and unconfigured levels return 0 (no limit).
func (c *Config) maxBytes(l uint32) uint64 {
	if l == 0 || l >= uint32(len(c.Levels)) {
		return 0
	}
	return c.MaxJobBytes
}
