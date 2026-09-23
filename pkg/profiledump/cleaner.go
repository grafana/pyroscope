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
	cleanupDayDepth      = 4 // namespace, tenant, year, month, day
	maxCleanupEntryBytes = 4096
)

// CleanupBucket borrows storage for key-based expiry, without reads or Close.
type CleanupBucket interface {
	Iter(context.Context, string, func(string) error, ...objstore.IterOption) error
	Delete(context.Context, string) error
	IsObjNotFoundErr(error) bool
}

// Cleaner traverses sequentially with at most five frames, one per key level.
// Work begins only when the service starts.
type Cleaner struct {
	services.Service
	cfg                CleanerConfig
	bucket             CleanupBucket
	now                func() time.Time
	stack              []cleanupFrame
	horizon            time.Time
	cycleFailed        bool
	deleted, malformed prometheus.Counter
	errors             *prometheus.CounterVec
	passes             *prometheus.CounterVec
	success            prometheus.Gauge
}

type cleanupFrame struct {
	prefix   string
	iterator *cleanupIterator
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
		deleted:   m.NewCounter(prometheus.CounterOpts{Name: "pyroscope_profile_dump_cleanup_deleted_total", Help: "Delete calls returning success; provider-recognized not-found responses are excluded. Idempotent provider successes may include concurrent deletes."}),
		malformed: m.NewCounter(prometheus.CounterOpts{Name: "pyroscope_profile_dump_cleanup_malformed_total", Help: "Malformed or unexpected listing entries encountered, including prefixes. Repeated visits may count again."}),
		errors:    m.NewCounterVec(prometheus.CounterOpts{Name: "pyroscope_profile_dump_cleanup_errors_total", Help: "Cleanup failures by operation; shutdown cancellation and work-budget exhaustion are excluded."}, []string{"operation"}),
		passes:    m.NewCounterVec(prometheus.CounterOpts{Name: "pyroscope_profile_dump_cleanup_passes_total", Help: "Cleanup passes by result: complete, partial, error, or canceled. Complete can finish a traversal begun in earlier passes."}, []string{"result"}),
		success:   m.NewGauge(prometheus.GaugeOpts{Name: "pyroscope_profile_dump_cleanup_last_success_timestamp_seconds", Help: "Completion time of the last full namespace traversal with no storage errors across its passes. Partial passes do not update this gauge."}),
	}
	c.Service = services.NewBasicService(nil, c.running, nil)
	return c, nil
}

func (c *Cleaner) running(ctx context.Context) error {
	defer c.closeTraversal()
	for ctx.Err() == nil {
		c.sweep(ctx)
		timer := time.NewTimer(c.cfg.SweepInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
		case <-timer.C:
		}
	}
	return nil
}

// sweep is called only by the service goroutine (or synchronously in tests).
// Capture times equal to the cutoff are retained: eligibility requires age > TTL.
func (c *Cleaner) sweep(serviceCtx context.Context) {
	cutoff := c.now().UTC().Add(-c.cfg.Retention)
	if len(c.stack) == 0 {
		c.stack = append(c.stack, cleanupFrame{prefix: ObjectPrefix})
		c.horizon = cutoff
		c.cycleFailed = false
	}
	// Freeze the horizon for progress, but honor clock rollback.
	if c.horizon.Before(cutoff) {
		cutoff = c.horizon
	}
	passCtx, cancel := context.WithTimeout(serviceCtx, c.cfg.SweepTimeout)
	defer cancel()
	remaining := c.cfg.MaxEntries
	for len(c.stack) > 0 {
		if passCtx.Err() != nil || remaining == 0 {
			break
		}
		depth := len(c.stack) - 1
		frame := &c.stack[depth]
		if frame.iterator == nil {
			frame.iterator = newCleanupIterator(serviceCtx, c.bucket, frame.prefix, c.cfg.SweepTimeout)
		}
		key, finished, err := frame.iterator.next(passCtx)
		if finished {
			c.finishListing(serviceCtx, err)
			continue
		}
		if err != nil {
			break
		}
		remaining--
		if len(key) > maxCleanupEntryBytes {
			c.malformed.Inc()
			continue
		}
		if depth < cleanupDayDepth {
			valid, newer := cleanupPartition(frame.prefix, key, depth, cutoff)
			if !valid {
				c.malformed.Inc()
				continue
			}
			if !newer {
				c.stack = append(c.stack, cleanupFrame{prefix: key})
			}
			continue
		}
		c.deleteExpiredCapture(passCtx, serviceCtx, frame.prefix, key, cutoff)
	}
	c.recordSweepResult(serviceCtx)
}

func (c *Cleaner) finishListing(serviceCtx context.Context, err error) {
	if err != nil && serviceCtx.Err() == nil && !c.bucket.IsObjNotFoundErr(err) {
		c.errors.WithLabelValues("list").Inc()
		if errors.Is(err, context.DeadlineExceeded) {
			c.errors.WithLabelValues("timeout").Inc()
		}
		c.cycleFailed = true
	}
	depth := len(c.stack) - 1
	c.stack[depth].iterator.stop()
	c.stack = c.stack[:depth]
}

func (c *Cleaner) deleteExpiredCapture(passCtx, serviceCtx context.Context, prefix, key string, cutoff time.Time) {
	// Delete only valid keys in this day, without path normalization.
	parsed, err := ParseObjectKey(key)
	if !strings.HasPrefix(key, prefix) || err != nil {
		c.malformed.Inc()
		return
	}
	if !parsed.CaptureTime.Before(cutoff) || passCtx.Err() != nil {
		return
	}
	err = c.bucket.Delete(passCtx, key)
	if err == nil {
		c.deleted.Inc()
	} else if !c.bucket.IsObjNotFoundErr(err) && serviceCtx.Err() == nil {
		c.errors.WithLabelValues("delete").Inc()
		if errors.Is(passCtx.Err(), context.DeadlineExceeded) {
			c.errors.WithLabelValues("timeout").Inc()
		}
		c.cycleFailed = true
	}
}

func (c *Cleaner) recordSweepResult(serviceCtx context.Context) {
	result := "partial"
	switch {
	case serviceCtx.Err() != nil:
		result = "canceled"
	case len(c.stack) == 0:
		result = "complete"
		if !c.cycleFailed {
			c.success.Set(float64(c.now().UnixNano()) / 1e9)
		}
	}
	if c.cycleFailed && result != "canceled" {
		result = "error"
	}
	c.passes.WithLabelValues(result).Inc()
}

// Cancel all iterators before joining. Provider limitations are in CLEANER.md.
func (c *Cleaner) closeTraversal() {
	for _, frame := range c.stack {
		if frame.iterator != nil {
			frame.iterator.cancel()
		}
	}
	for _, frame := range c.stack {
		if frame.iterator != nil {
			frame.iterator.stop()
		}
	}
	c.stack = nil
}

// cleanupPartition validates one directory without normalizing its path.
func cleanupPartition(prefix, key string, depth int, cutoff time.Time) (valid, newer bool) {
	if !strings.HasPrefix(key, prefix) || !strings.HasSuffix(key, "/") {
		return false, false
	}
	segment := strings.TrimSuffix(strings.TrimPrefix(key, prefix), "/")
	if strings.Contains(segment, "/") {
		return false, false
	}
	if depth == 0 {
		_, err := DecodeTenant(segment)
		return err == nil, false
	}
	parts := strings.Split(strings.TrimSuffix(strings.TrimPrefix(key, ObjectPrefix), "/"), "/")
	date := strings.Join(parts[1:], "/")
	layout := []string{"", "2006", "2006/01", "2006/01/02"}[depth]
	start, err := time.Parse(layout, date)
	if err != nil || start.Format(layout) != date || start.Year() < 1970 {
		return false, false
	}
	return true, start.After(cutoff)
}
