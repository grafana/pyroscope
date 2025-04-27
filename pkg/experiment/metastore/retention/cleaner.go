package retention

import (
	"context"
	"flag"
	"sync"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
)

// The maximum number of partitions to delete in one go.
// A partition is qualified by partition key, tenant, and shard ID.
const maxTruncatePartitions = 32

type Index interface {
	TruncatePartitions(ctx context.Context, before time.Time, max int) error
}

type Config struct {
	RetentionPeriod        time.Duration `yaml:"retention_period"`
	RetentionCheckInterval time.Duration `yaml:"retention_check_interval"`
}

func (c *Config) RegisterFlagsWithPrefix(prefix string, f *flag.FlagSet) {
	f.DurationVar(&c.RetentionPeriod, prefix+"retention-period", 0, "Data older than this period will be deleted from the storage. 0 to disable.")
	f.DurationVar(&c.RetentionCheckInterval, prefix+"retention-check-interval", time.Minute, "Interval for retention check. 0 to disable.")
}

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
		logger:  logger,
		config:  config,
		index:   index,
		started: false,
		cancel:  nil,
		m:       sync.Mutex{},
	}
}

func (c *Cleaner) Start() {
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
	if c.config.RetentionCheckInterval == 0 || c.config.RetentionPeriod == 0 {
		return
	}
	ticker := time.NewTicker(c.config.RetentionCheckInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			before := time.Now().Add(-c.config.RetentionPeriod)
			if err := c.index.TruncatePartitions(ctx, before, maxTruncatePartitions); err != nil {
				level.Error(c.logger).Log("msg", "failed to truncate partitions", "err", err)
			}
		}
	}
}
