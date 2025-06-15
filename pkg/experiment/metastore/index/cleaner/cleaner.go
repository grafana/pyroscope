package cleaner

import (
	"context"
	"errors"
	"flag"
	"sync"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"

	"github.com/grafana/pyroscope/pkg/experiment/metastore/index/cleaner/retention"
	"github.com/grafana/pyroscope/pkg/experiment/metastore/raftnode"
)

// The maximum number of partitions to delete in one go.
// A partition is qualified by partition key, tenant, and shard ID.
const maxTruncatePartitions = 128

type Index interface {
	TruncateIndex(context.Context, retention.Policy) error
}

type Config struct {
	CleanupInterval      time.Duration `yaml:"cleanup_interval"`
	CleanupMaxPartitions int           `yaml:"cleanup_max_partitions"`
}

func (c *Config) RegisterFlagsWithPrefix(prefix string, f *flag.FlagSet) {
	f.DurationVar(&c.CleanupInterval, prefix+"cleanup-interval", 0, "Interval for index cleanup check. 0 to disable.")
	f.IntVar(&c.CleanupMaxPartitions, prefix+"cleanup-max-partitions", maxTruncatePartitions, "Maximum number of partitions to cleanup at once. A partition is qualified by partition key, tenant, and shard.")
}

// Cleaner is responsible for periodically cleaning up
// the index by applying retention policies. As of now,
// it only applies the time-based retention policy.
type Cleaner struct {
	logger log.Logger
	config Config
	index  Index

	started bool
	cancel  context.CancelFunc
	m       sync.Mutex
}

func NewCleaner(logger log.Logger, config Config, index Index) *Cleaner {
	return &Cleaner{
		logger: logger,
		config: config,
		index:  index,
	}
}

func (c *Cleaner) Start() {
	if c.config.CleanupInterval == 0 {
		return
	}
	c.m.Lock()
	defer c.m.Unlock()
	if c.started {
		c.logger.Log("msg", "index cleaner already started")
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	c.cancel = cancel
	c.started = true
	go c.loop(ctx)
	c.logger.Log("msg", "index cleaner started")
}

func (c *Cleaner) Stop() {
	if c.config.CleanupInterval == 0 {
		return
	}
	c.m.Lock()
	defer c.m.Unlock()
	if !c.started {
		c.logger.Log("msg", "index cleaner already stopped")
		return
	}
	if c.cancel != nil {
		c.cancel()
	}
	c.started = false
	c.logger.Log("msg", "index cleaner stopped")
}

func (c *Cleaner) loop(ctx context.Context) {
	ticker := time.NewTicker(c.config.CleanupInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			switch err := c.index.TruncateIndex(ctx, new(retention.TimeBasedRetentionPolicy)); {
			case err == nil:
			case errors.Is(err, context.Canceled):
				return
			case raftnode.IsRaftLeadershipError(err):
				level.Warn(c.logger).Log("msg", "leadership change; cleanup interrupted", "err", err)
			default:
				level.Error(c.logger).Log("msg", "cleanup attempt failed", "err", err)
			}
		}
	}
}
