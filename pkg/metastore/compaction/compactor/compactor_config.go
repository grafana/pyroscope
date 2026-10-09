package compactor

import (
	"flag"
	"fmt"
	"math"
	"time"
)

type Config struct {
	Levels []LevelConfig

	MaxCompactionLevel uint `yaml:"max_compaction_level" category:"experimental"`

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
		MaxCompactionLevel: 3,
		Levels: []LevelConfig{
			{MaxBlocks: 20, MaxAge: int64(1 * 36 * time.Second)},
			{MaxBlocks: 10, MaxAge: int64(2 * 360 * time.Second)},
			{MaxBlocks: 10, MaxAge: int64(3 * 3600 * time.Second)},
			{MaxBlocks: 10, MaxAge: int64(24 * time.Hour)},
		},

		CleanupBatchSize:   2,
		CleanupDelay:       15 * time.Minute,
		CleanupJobMaxLevel: 1,
		CleanupJobMinLevel: 0,
	}
}

func (c *Config) RegisterFlagsWithPrefix(prefix string, f *flag.FlagSet) {
	*c = DefaultConfig()
	f.UintVar(&c.MaxCompactionLevel, prefix+"max-compaction-level", 3, "Maximum output compaction level (minimum 3). Higher levels reuse the final batching policy. Values above 3 take effect after the configurable-compaction-levels FSM version activates on all metastore replicas.")
}

// acceptsLevel controls planning only. Admission is decided by the leader
// and replicated in the Raft log, independently of local configuration.
func (c *Config) acceptsLevel(l uint32) bool {
	return l < c.maxLevel()
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

// Levels describes batching policies by source level. Higher source levels
// reuse the final policy without allocating configuration per level.
func (c *Config) maxBlocks(l uint32) uint {
	if len(c.Levels) == 0 {
		return 0
	}
	return c.Levels[min(l, uint32(len(c.Levels)-1))].MaxBlocks
}

func (c *Config) maxAge(l uint32) int64 {
	if len(c.Levels) == 0 {
		return 0
	}
	return c.Levels[min(l, uint32(len(c.Levels)-1))].MaxAge
}

func (c *Config) maxLevel() uint32 {
	// Keep zero-valued internal configurations compatible with the default.
	return max(3, uint32(c.MaxCompactionLevel))
}

func (c *Config) Validate() error {
	if c.MaxCompactionLevel < 3 || c.MaxCompactionLevel > math.MaxUint32 {
		return fmt.Errorf("metastore.max-compaction-level must be between 3 and %d", uint64(math.MaxUint32))
	}
	return nil
}
