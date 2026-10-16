package profiledump

import (
	"context"
	"errors"
	"strings"
	"time"

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

// Depth identifies the directory being listed, relative to NativeObjectPrefix.
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

// Cleaner sweeps native capture files sequentially with conservative hourly retention.
type Cleaner struct {
	services.Service
	cfg           CleanerConfig
	bucket        CleanupBucket
	now           func() time.Time
	sweepInterval time.Duration
	deleteTimeout time.Duration
	deleted       prometheus.Counter
	errors        *prometheus.CounterVec
	success       prometheus.Gauge
}

func NewCleaner(cfg CleanerConfig, bucket CleanupBucket, reg prometheus.Registerer, now func() time.Time) (*Cleaner, error) {
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
	c := &Cleaner{cfg: cfg, bucket: bucket, now: now,
		sweepInterval: cleanupSweepInterval, deleteTimeout: cleanupDeleteTimeout,
		deleted: m.NewCounter(prometheus.CounterOpts{Name: "pyroscope_profile_dump_cleanup_deleted_total", Help: "Delete calls returning success. Provider-recognized not-found responses are excluded. Idempotent provider successes may include concurrent deletes."}),
		errors:  m.NewCounterVec(prometheus.CounterOpts{Name: "pyroscope_profile_dump_cleanup_errors_total", Help: "Cleanup storage failures by operation: list or delete. Shutdown cancellation is excluded."}, []string{"operation"}),
		success: m.NewGauge(prometheus.GaugeOpts{Name: "pyroscope_profile_dump_cleanup_last_success_timestamp_seconds", Help: "Completion time of the last namespace sweep without storage errors. Listings are not snapshots."}),
	}
	c.errors.WithLabelValues("list")
	c.errors.WithLabelValues("delete")
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

func (c *Cleaner) sweep(ctx context.Context) {
	cutoff := c.now().UTC().Add(-c.cfg.Retention)
	if c.walk(ctx, NativeObjectPrefix, cleanupRootDepth, cutoff) && ctx.Err() == nil {
		c.success.Set(float64(c.now().UnixNano()) / 1e9)
	}
}

// walk lists tenant, date, and hour partitions before recursively listing files
// in an eligible hour. All state belongs to this sweep, including failures.
func (c *Cleaner) walk(ctx context.Context, prefix string, depth int, cutoff time.Time) bool {
	if ctx.Err() != nil {
		return false
	}
	success := true
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
			if cleanupPartition(prefix, key, depth, cutoff) && !c.walk(ctx, key, depth+1, cutoff) {
				success = false
			}
			return nil
		}
		parsed, err := ParseNativeObjectKey(key)
		if err != nil || !strings.HasPrefix(key, prefix) || !parsed.CaptureTime.Before(cutoff) {
			return nil
		}
		if ctx.Err() != nil {
			return nil
		}
		deleteCtx, cancel := context.WithTimeout(ctx, c.deleteTimeout)
		err = c.bucket.Delete(deleteCtx, key)
		cancel()
		if err == nil {
			c.deleted.Inc()
		} else if ctx.Err() == nil && !c.bucket.IsObjNotFoundErr(err) {
			c.errors.WithLabelValues("delete").Inc()
			success = false
		}
		return nil
	}, opts...)
	// A not-found listing error can mean the bucket itself is inaccessible.
	if err != nil && ctx.Err() == nil {
		c.errors.WithLabelValues("list").Inc()
		success = false
	}
	return success && ctx.Err() == nil
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
		return ValidateNativeTenant(segment) == nil
	}
	parts := strings.Split(strings.TrimSuffix(strings.TrimPrefix(key, NativeObjectPrefix), "/"), "/")
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
