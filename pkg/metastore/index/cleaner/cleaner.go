package cleaner

import (
	"context"
	"errors"
	"flag"
	"sync"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"

	"github.com/grafana/pyroscope/pkg/metastore/index/cleaner/retention"
	"github.com/grafana/pyroscope/pkg/metastore/raftnode"
)

type Index interface {
	TruncateIndex(context.Context, retention.Policy) error
}

type Config struct {
	CleanupMaxPartitions int           `yaml:"cleanup_max_partitions"`
	CleanupGracePeriod   time.Duration `yaml:"cleanup_grace_period"`
	CleanupInterval      time.Duration `yaml:"cleanup_interval"`
}

func (c *Config) RegisterFlagsWithPrefix(prefix string, f *flag.FlagSet) {
	f.DurationVar(&c.CleanupInterval, prefix+"cleanup-interval", 0, "Interval for index cleanup check. 0 to disable.")
	f.DurationVar(&c.CleanupGracePeriod, prefix+"cleanup-grace-period", time.Hour*6, "After a partition is eligible for deletion, it will be kept for this period before actually being evaluated. The period should cover the time difference between the block creation time and the data timestamps. Blocks are only deleted if all data in the block has passed the retention period, and the grace period delays the moment when the partition is evaluated for deletion.")
	f.IntVar(&c.CleanupMaxPartitions, prefix+"cleanup-max-partitions", 32, "Maximum number of partitions to cleanup at once. A partition is qualified by partition key, tenant, and shard.")
}

// Cleaner is responsible for periodically cleaning up
// the index by applying retention policies. As of now,
// it only applies the time-based retention policy.
type Cleaner struct {
	logger    log.Logger
	overrides retention.Overrides
	config    Config
	index     Index

	started bool
	cancel  context.CancelFunc
	mu      sync.Mutex
}

func NewCleaner(logger log.Logger, overrides retention.Overrides, config Config, index Index) *Cleaner {
	return &Cleaner{
		logger:    logger,
		overrides: overrides,
		config:    config,
		index:     index,
	}
}

func (c *Cleaner) Start() {
	if c.config.CleanupInterval == 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
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
	c.mu.Lock()
	defer c.mu.Unlock()
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
			rp := retention.NewTimeBasedRetentionPolicy(
				log.With(c.logger, "component", "retention-policy"),
				c.overrides,
				c.config.CleanupMaxPartitions,
				c.config.CleanupGracePeriod,
				time.Now(),
			)
			switch err := c.index.TruncateIndex(ctx, rp); {
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
