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

// CleanupBucket is borrowed storage for key-based retention.
type CleanupBucket interface {
	Iter(context.Context, string, func(string) error, ...objstore.IterOption) error
	Delete(context.Context, string) error
	IsObjNotFoundErr(error) bool
}

// Cleaner traverses tenant/date partitions with at most five live iterators.
type Cleaner struct {
	services.Service
	cfg                         CleanerConfig
	bucket                      CleanupBucket
	now                         func() time.Time
	stack                       []cleanupFrame
	horizon                     time.Time
	cycleFailed                 bool
	deleted, missing, malformed prometheus.Counter
	errors                      *prometheus.CounterVec
	passes                      *prometheus.CounterVec
	success                     prometheus.Gauge
	successCutoff               prometheus.Gauge
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
		deleted:       m.NewCounter(prometheus.CounterOpts{Name: "pyroscope_profile_dump_cleanup_deleted_total", Help: "Delete calls returning success. Provider-recognized not-found responses are excluded. Idempotent provider successes may include concurrent deletes."}),
		missing:       m.NewCounter(prometheus.CounterOpts{Name: "pyroscope_profile_dump_cleanup_missing_total", Help: "Delete calls returning provider-recognized not-found, including concurrent deletes."}),
		malformed:     m.NewCounter(prometheus.CounterOpts{Name: "pyroscope_profile_dump_cleanup_malformed_total", Help: "Malformed or unexpected listing entries encountered, including prefixes. Repeated visits may count again."}),
		errors:        m.NewCounterVec(prometheus.CounterOpts{Name: "pyroscope_profile_dump_cleanup_errors_total", Help: "Cleanup failures by operation. Shutdown cancellation and ordinary partial-pass boundaries are excluded."}, []string{"operation"}),
		passes:        m.NewCounterVec(prometheus.CounterOpts{Name: "pyroscope_profile_dump_cleanup_passes_total", Help: "Cleanup passes by result: complete, partial, error, or canceled. Complete can finish a traversal begun in earlier passes."}, []string{"result"}),
		success:       m.NewGauge(prometheus.GaugeOpts{Name: "pyroscope_profile_dump_cleanup_last_success_timestamp_seconds", Help: "Completion time of the last full namespace traversal with no storage errors across its passes. Partial passes do not update this gauge."}),
		successCutoff: m.NewGauge(prometheus.GaugeOpts{Name: "pyroscope_profile_dump_cleanup_last_success_cutoff_timestamp_seconds", Help: "Capture-time cutoff of the last full traversal without storage errors. Objects uploaded behind its cursor may await the next traversal."}),
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

func (c *Cleaner) sweep(parent context.Context) {
	cutoff := c.now().UTC().Add(-c.cfg.Retention)
	if len(c.stack) == 0 {
		c.stack = append(c.stack, cleanupFrame{prefix: ObjectPrefix})
		c.horizon = cutoff
		c.cycleFailed = false
	}
	// New captures must not keep extending this traversal.
	// Clock rollback can lower its cutoff.
	if c.horizon.Before(cutoff) {
		cutoff = c.horizon
	}
	c.horizon = cutoff
	ctx, cancel := context.WithTimeout(parent, c.cfg.SweepTimeout)
	defer cancel()
	remaining := c.cfg.MaxEntries
	for len(c.stack) > 0 {
		if ctx.Err() != nil || remaining == 0 {
			break
		}
		depth := len(c.stack) - 1
		frame := &c.stack[depth]
		if frame.iterator == nil {
			frame.iterator = newCleanupIterator(parent, c.bucket, frame.prefix, c.cfg.SweepTimeout)
		}
		key, finished, err := frame.iterator.next(ctx)
		if finished {
			c.finishListing(parent, err)
			continue
		}
		if err != nil {
			break
		}
		remaining--
		c.processEntry(parent, ctx, key, cutoff)
	}
	c.observePass(parent)
}

func (c *Cleaner) finishListing(parent context.Context, err error) {
	depth := len(c.stack) - 1
	frame := &c.stack[depth]
	if err != nil && parent.Err() == nil && !c.bucket.IsObjNotFoundErr(err) {
		c.errors.WithLabelValues("list").Inc()
		if errors.Is(err, context.DeadlineExceeded) {
			c.errors.WithLabelValues("timeout").Inc()
		}
		c.cycleFailed = true
	}
	frame.iterator.stop()
	c.stack = c.stack[:depth]
}

func (c *Cleaner) processEntry(parent, ctx context.Context, key string, cutoff time.Time) {
	depth := len(c.stack) - 1
	prefix := c.stack[depth].prefix
	if len(key) > 4096 {
		c.malformed.Inc()
		return
	}
	if depth < 4 {
		valid, newer := cleanupPartition(prefix, key, depth, cutoff)
		if !valid {
			c.malformed.Inc()
			return
		}
		if !newer {
			c.stack = append(c.stack, cleanupFrame{prefix: key})
		}
		return
	}
	parsed, parseErr := ParseObjectKey(key)
	if !strings.HasPrefix(key, prefix) || parseErr != nil {
		c.malformed.Inc()
		return
	}
	if !parsed.CaptureTime.Before(cutoff) {
		return
	}
	if ctx.Err() != nil {
		// This consumed entry needs another traversal before coverage is complete.
		c.cycleFailed = true
		if parent.Err() == nil {
			c.errors.WithLabelValues("timeout").Inc()
		}
		return
	}
	err := c.bucket.Delete(ctx, key)
	if err == nil {
		c.deleted.Inc()
	} else if c.bucket.IsObjNotFoundErr(err) {
		c.missing.Inc()
	} else if parent.Err() == nil {
		c.errors.WithLabelValues("delete").Inc()
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			c.errors.WithLabelValues("timeout").Inc()
		}
		c.cycleFailed = true
	}
}

func (c *Cleaner) observePass(parent context.Context) {
	result := "partial"
	if c.cycleFailed {
		result = "error"
	}
	switch {
	case parent.Err() != nil:
		result = "canceled"
	case len(c.stack) == 0:
		if !c.cycleFailed {
			result = "complete"
			c.success.Set(float64(c.now().UnixNano()) / 1e9)
			c.successCutoff.Set(float64(c.horizon.UnixNano()) / 1e9)
		}
	}
	c.passes.WithLabelValues(result).Inc()
}

// Cancel every listing before a slow provider can block shutdown.
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
