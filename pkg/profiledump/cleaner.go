package profiledump

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/grafana/dskit/services"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/thanos-io/objstore"
)

const (
	cleanupSweepInterval        = time.Hour
	cleanupDeleteTimeout        = 10 * time.Second
	cleanupMaxPartitionKeyBytes = 4096
)

// Depth identifies the directory being listed, relative to ObjectPrefix.
const (
	cleanupRootDepth = iota
	cleanupTenantDepth
	cleanupDateDepth
	cleanupHourDepth
)

// CleanupBucket is borrowed storage for key-based retention.
type CleanupBucket interface {
	Iter(context.Context, string, func(string) error, ...objstore.IterOption) error
	Delete(context.Context, string) error
	IsObjNotFoundErr(error) bool
}

// Cleaner sweeps capture files sequentially with conservative hourly retention.
type Cleaner struct {
	services.Service
	cfg           CleanerConfig
	bucket        CleanupBucket
	logger        log.Logger
	now           func() time.Time
	sweepInterval time.Duration
	deleteTimeout time.Duration
	success       prometheus.Gauge
}

func NewCleaner(cfg CleanerConfig, bucket CleanupBucket, logger log.Logger, reg prometheus.Registerer, now func() time.Time) (*Cleaner, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if bucket == nil {
		return nil, errors.New("cleanup bucket is required")
	}
	if now == nil {
		now = time.Now
	}
	m := promauto.With(reg)
	c := &Cleaner{cfg: cfg, bucket: bucket, logger: logger, now: now,
		sweepInterval: cleanupSweepInterval, deleteTimeout: cleanupDeleteTimeout,
		success: m.NewGauge(prometheus.GaugeOpts{Name: "pyroscope_profile_dump_cleanup_last_success_timestamp_seconds", Help: "Completion time of the last full cleanup sweep without storage errors, as a Unix timestamp in seconds."}),
	}
	c.Service = services.NewBasicService(nil, c.running, nil)
	return c, nil
}

func (c *Cleaner) running(ctx context.Context) error {
	for ctx.Err() == nil {
		c.sweep(ctx)
		timer := time.NewTimer(c.sweepInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
		case <-timer.C:
		}
	}
	return nil
}

// sweep records success only if traversal completes without storage errors
// and the context has not been canceled.
func (c *Cleaner) sweep(ctx context.Context) {
	cutoff := c.now().UTC().Add(-c.cfg.Retention)
	if err := c.walk(ctx, ObjectPrefix, cleanupRootDepth, cutoff); err != nil {
		level.Error(c.logger).Log("msg", "failed to clean up profile dumps", "err", err)
	} else if ctx.Err() == nil {
		c.success.Set(float64(c.now().UnixNano()) / 1e9)
	}
}

// walk lists files within eligible hour partitions, continuing after storage
// failures and returning the first error encountered.
func (c *Cleaner) walk(ctx context.Context, prefix string, depth int, cutoff time.Time) error {
	if ctx.Err() != nil {
		return nil
	}
	var firstErr error
	var opts []objstore.IterOption
	if depth == cleanupHourDepth {
		opts = append(opts, objstore.WithRecursiveIter())
	}
	err := c.bucket.Iter(ctx, prefix, func(key string) error {
		// Always return nil, including during cancellation, so provider listing
		// producers can drain. The service waits for Iter to return before stopping.
		if ctx.Err() != nil {
			return nil
		}
		if depth < cleanupHourDepth {
			if cleanupPartition(prefix, key, depth, cutoff) {
				if err := c.walk(ctx, key, depth+1, cutoff); err != nil && firstErr == nil {
					firstErr = err
				}
			}
			return nil
		}
		// Capture owns every object beneath a valid eligible hour, including nested
		// objects and malformed filenames. Partition validation bounds deletion.
		if !strings.HasPrefix(key, prefix) {
			return nil
		}
		if ctx.Err() != nil {
			return nil
		}
		deleteCtx, cancel := context.WithTimeout(ctx, c.deleteTimeout)
		err := c.bucket.Delete(deleteCtx, key)
		cancel()
		if err != nil && ctx.Err() == nil && !c.bucket.IsObjNotFoundErr(err) && firstErr == nil {
			firstErr = fmt.Errorf("delete profile dump %q: %w", key, err)
		}
		return nil
	}, opts...)
	// A not-found listing error can mean the bucket itself is inaccessible.
	if err != nil && ctx.Err() == nil && firstErr == nil {
		firstErr = fmt.Errorf("list profile dumps under %q: %w", prefix, err)
	}
	return firstErr
}

func cleanupPartition(prefix, key string, depth int, cutoff time.Time) bool {
	if len(key) > cleanupMaxPartitionKeyBytes || !strings.HasPrefix(key, prefix) || !strings.HasSuffix(key, "/") {
		return false
	}
	segment := strings.TrimSuffix(strings.TrimPrefix(key, prefix), "/")
	if strings.Contains(segment, "/") {
		return false
	}
	if depth == cleanupRootDepth {
		return ValidateTenant(segment) == nil
	}
	parts := strings.Split(strings.TrimSuffix(strings.TrimPrefix(key, ObjectPrefix), "/"), "/")
	date := strings.Join(parts[1:], "/")
	layout := "2006-01-02"
	if depth == cleanupDateDepth {
		layout += "/15"
	}
	start, err := time.Parse(layout, date)
	if err != nil || start.Format(layout) != date || start.Year() < 1970 {
		return false
	}
	if depth == cleanupDateDepth {
		return !start.Add(time.Hour).After(cutoff)
	}
	return !start.After(cutoff)
}
