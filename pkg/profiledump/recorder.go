package profiledump

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/time/rate"
)

// itemReservation covers bounded keys, tenant identity and per-item bookkeeping.
const itemReservation int64 = 4096

// PolicyProvider supplies current immutable snapshots to concurrent admissions.
type PolicyProvider interface{ ProfileDebugDump(tenantID string) Policy }

// UploadFunc receives a full capture key and tenant for SSE, without another
// tenant prefix. The body is borrowed for the duration of the call.
// The recorder never closes borrowed storage.
type UploadFunc func(ctx context.Context, tenantID, key string, body io.Reader) error

// Dependencies inject policies, uploads, and test controls. Now and WithTimeout
// are called concurrently. Random calls are serialized, defaulting to rand/v2.
type Dependencies struct {
	Policies      PolicyProvider
	Upload        UploadFunc
	DistributorID string
	Registerer    prometheus.Registerer
	Now           func() time.Time
	Random        func() float64
	WithTimeout   func(context.Context, time.Duration) (context.Context, context.CancelFunc)
	// PruneTicks optionally replaces the maintenance ticker. The caller owns it.
	PruneTicks <-chan time.Time
}

type uploadItem struct {
	data        []byte
	tenant, key string
	source      string
	span        trace.SpanContext
	reservation int64
}

type Recorder struct {
	cfg                RecorderConfig
	deps               Dependencies
	metrics            recorderMetrics
	ctx                context.Context
	cancel             context.CancelFunc
	queue              chan uploadItem
	stop               chan struct{}
	done               chan struct{}
	mu                 sync.Mutex // admission, accounting, limiter state and queue closure
	stopping           bool
	preparing, workers int
	retained           int64
	process            *rate.Limiter
	tenants            map[string]*rate.Limiter
}

// NewRecorder starts a fixed pool and one limiter-maintenance goroutine.
// Shutdown stops the pool. A nil *Recorder disables capture.
func NewRecorder(cfg RecorderConfig, deps Dependencies) (*Recorder, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if deps.Policies == nil || deps.Upload == nil {
		return nil, fmt.Errorf("recorder requires policies and upload")
	}
	if err := validateText("distributor_id", deps.DistributorID, MaxTextBytes, true); err != nil {
		// Invalid diagnostic IDs fall back to a bounded placeholder.
		deps.DistributorID = unknownValue
	}
	if deps.Now == nil {
		deps.Now = time.Now
	}
	if deps.Random == nil {
		deps.Random = rand.Float64
	}
	if deps.WithTimeout == nil {
		deps.WithTimeout = context.WithTimeout
	}
	ctx, cancel := context.WithCancel(context.Background())
	r := &Recorder{cfg: cfg, deps: deps, metrics: newRecorderMetrics(deps.Registerer), ctx: ctx, cancel: cancel,
		queue: make(chan uploadItem, cfg.QueueCapacity), stop: make(chan struct{}), done: make(chan struct{}), workers: cfg.Workers + 1,
		process: rate.NewLimiter(rate.Limit(cfg.ProcessCapturesPerSecond), cfg.ProcessBurst), tenants: make(map[string]*rate.Limiter)}
	for i := 0; i < cfg.Workers; i++ {
		go r.worker()
	}
	go r.pruneLoop()
	return r, nil
}

// PolicyActive is an allocation-free hint to skip inactive tenants, without
// admission or metrics. Capture rechecks the policy and deadline.
func (r *Recorder) PolicyActive(tenant string) bool {
	return r != nil && r.deps.Policies.ProfileDebugDump(tenant).ActiveAt(r.deps.Now())
}

// Capture prepares an owned object and attempts a nonblocking enqueue.
// Work uses the recorder lifecycle and carries only request trace context.
func (r *Recorder) Capture(ctx context.Context, tenant string, c Candidate) (out Outcome) {
	if r == nil {
		return Outcome{Reason: DropDisabled}
	}
	source := metricSource(c.Metadata.SourceProtocol)
	defer func() { r.metrics.admission(source, out) }()
	now := r.deps.Now()
	p := r.deps.Policies.ProfileDebugDump(tenant)
	if p.Fingerprint() == "" {
		out.Reason = DropDisabled
		return
	}
	if !p.ActiveAt(now) {
		out.Reason = DropExpired
		return
	}
	if !p.Matches(c.SelectorLabels) {
		out.Reason = DropSelector
		return
	}
	r.mu.Lock()
	if r.stopping {
		r.mu.Unlock()
		out.Reason = DropShutdown
		return
	}
	if p.Probability() < 1 && r.deps.Random() >= p.Probability() {
		r.mu.Unlock()
		out.Reason = DropSampled
		return
	}
	out.Reason = r.admitRate(tenant, p, now)
	if out.Reason != "" {
		r.mu.Unlock()
		return
	}
	r.preparing++
	r.mu.Unlock()
	defer r.finishPreparation()
	ctx, finishSpan := startCaptureSpan(ctx)
	defer func() { finishSpan(out) }()
	prepared, out := r.prepareCapture(tenant, c, p, now)
	if out.Reason != "" {
		return out
	}
	reservation, ok := r.reservationSize(out.Size, int64(cap(prepared.metadataJSON)))
	if !ok {
		out.Reason = DropByteBudget
		return
	}
	r.mu.Lock()
	if r.stopping {
		r.mu.Unlock()
		out.Reason = DropShutdown
		return
	}
	if reservation > r.cfg.MaxRetainedBytes-r.retained {
		r.mu.Unlock()
		out.Reason = DropByteBudget
		return
	}
	r.retained += reservation
	r.metrics.retained.Add(float64(reservation))
	r.mu.Unlock()
	transferred := false
	defer func() {
		if !transferred {
			r.release(reservation)
		}
	}()
	data, err := encodeCapture(prepared, c.Payload)
	if err != nil {
		out.Reason = DropSerialization
		return
	}
	prepared.metadataJSON = nil
	item := uploadItem{data: data, tenant: strings.Clone(tenant), key: out.ObjectKey, source: source, span: trace.SpanContextFromContext(ctx), reservation: reservation}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.stopping {
		out.Reason = DropShutdown
		return
	}
	select {
	case r.queue <- item:
		transferred = true
		r.metrics.queueItems.Inc()
		r.metrics.queueBytes.Add(float64(out.Size))
		out.Enqueued = true
	default:
		out.Reason = DropQueueFull
	}
	return
}

func (r *Recorder) prepareCapture(tenant string, c Candidate, p Policy, now time.Time) (preparedEnvelope, Outcome) {
	var out Outcome
	out.Format = c.Metadata.NativeFormat
	out.SourceProtocol = c.Metadata.SourceProtocol
	out.DistributorID = r.deps.DistributorID
	out.PolicyFingerprint = p.Fingerprint()
	size := int64(len(c.Payload))
	out.PayloadSize = size
	key, id, err := NewObjectKey(tenant, now, c.Metadata.NativeFormat)
	if err != nil {
		out.Reason = DropInvalid
		return preparedEnvelope{}, out
	}
	out.CaptureID, out.ObjectKey = id.String(), key
	m := c.Metadata
	m.SchemaVersion, m.CapturedAt, m.TenantID = Version, now.UTC(), tenant
	m.CaptureID, m.PolicyFingerprint, m.ActivationSource = out.CaptureID, p.Fingerprint(), ActivationRuntimeOverride
	m.DistributorID, m.PayloadSize = r.deps.DistributorID, size
	if err = m.Validate(); err != nil {
		out.Reason = DropInvalid
		return preparedEnvelope{}, out
	}
	prepared, err := prepareEnvelope(m, r.cfg.MaxObjectBytes)
	if err != nil {
		out.Reason = DropTooLarge
		return preparedEnvelope{}, out
	}
	out.Size = prepared.objectSize
	return prepared, out
}

func encodeCapture(prepared preparedEnvelope, payload []byte) ([]byte, error) {
	data := make([]byte, int(prepared.objectSize))
	header := &fixedWriter{dst: data[:HeaderSize+len(prepared.metadataJSON)]}
	if err := prepared.writeHeader(header); err != nil {
		return nil, err
	}
	copy(data[header.n:], payload)
	return data, nil
}

// reservationSize includes the overlapping marshal result and final object.
func (r *Recorder) reservationSize(objectSize, metadataCapacity int64) (int64, bool) {
	remaining := r.cfg.MaxRetainedBytes
	for _, n := range [...]int64{objectSize, metadataCapacity, itemReservation} {
		if n < 0 || n > remaining {
			return 0, false
		}
		remaining -= n
	}
	return r.cfg.MaxRetainedBytes - remaining, true
}

// admitRate requires mu and preserves token history across policy changes.
func (r *Recorder) admitRate(tenant string, p Policy, now time.Time) DropReason {
	if validateText("tenant_id", tenant, MaxTenantBytes, true) != nil {
		return DropInvalid
	}
	l := r.tenants[tenant]
	if l == nil {
		if len(r.tenants) >= r.cfg.MaxTenantLimiters {
			r.pruneLocked(now)
		}
		if len(r.tenants) >= r.cfg.MaxTenantLimiters {
			return DropLimiterCapacity
		}
		l = rate.NewLimiter(rate.Limit(p.MaxCapturesPerSecond()), r.cfg.TenantBurst)
		r.tenants[strings.Clone(tenant)] = l
	} else if l.Limit() != rate.Limit(p.MaxCapturesPerSecond()) {
		l.SetLimitAt(now, rate.Limit(p.MaxCapturesPerSecond()))
	}
	if !l.AllowN(now, 1) {
		return DropTenantRate
	}
	if !r.process.AllowN(now, 1) {
		return DropProcessRate
	}
	return ""
}

func (r *Recorder) pruneLocked(now time.Time) {
	for tenant := range r.tenants {
		if !r.deps.Policies.ProfileDebugDump(tenant).ActiveAt(now) {
			delete(r.tenants, tenant)
		}
	}
}
func (r *Recorder) pruneLoop() {
	defer r.workerDone()
	ticks := r.deps.PruneTicks
	if ticks == nil {
		ticker := time.NewTicker(r.cfg.LimiterPruneInterval)
		defer ticker.Stop()
		ticks = ticker.C
	}
	for {
		select {
		case <-r.stop:
			return
		case _, ok := <-ticks:
			if !ok {
				return
			}
			r.mu.Lock()
			r.pruneLocked(r.deps.Now())
			r.mu.Unlock()
		}
	}
}

func (r *Recorder) release(n int64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.retained -= n
	r.metrics.retained.Sub(float64(n))
	r.maybeDone()
}
func (r *Recorder) finishPreparation() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.preparing--
	r.maybeDone()
}
func (r *Recorder) workerDone() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.workers--
	r.maybeDone()
}
func (r *Recorder) maybeDone() {
	// Shutdown may still own a queued reservation after workers exit.
	if r.stopping && r.preparing == 0 && r.workers == 0 && r.retained == 0 {
		r.cancel()
		close(r.done)
	}
}
func (r *Recorder) dequeued(item uploadItem) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.metrics.queueItems.Dec()
	r.metrics.queueBytes.Sub(float64(len(item.data)))
}
func (r *Recorder) worker() {
	defer r.workerDone()
	for item := range r.queue {
		r.dequeued(item)
		if r.ctx.Err() == nil {
			r.upload(item)
		} else {
			r.recordShutdownDrop(item)
		}
		reservation := item.reservation
		item = uploadItem{}
		r.release(reservation)
	}
}
func (r *Recorder) upload(item uploadItem) {
	ctx, cancel := r.deps.WithTimeout(trace.ContextWithSpanContext(r.ctx, item.span), r.cfg.UploadTimeout)
	defer cancel()
	started := r.deps.Now()
	err := r.deps.Upload(ctx, item.tenant, item.key, bytes.NewReader(item.data))
	r.metrics.uploadDuration.WithLabelValues(item.source).Observe(r.deps.Now().Sub(started).Seconds())
	if err != nil || ctx.Err() != nil {
		result := "error"
		switch {
		case errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded):
			result = "timeout"
		case errors.Is(err, context.Canceled) || errors.Is(ctx.Err(), context.Canceled):
			result = "canceled"
		}
		r.metrics.uploads.WithLabelValues(item.source, result).Inc()
		r.metrics.uploadErrors.WithLabelValues(item.source).Inc()
		return
	}
	r.metrics.uploads.WithLabelValues(item.source, "success").Inc()
	r.metrics.bytes.WithLabelValues(item.source, "uploaded").Add(float64(len(item.data)))
}

// Shutdown stops admission and drains until the first caller/drain deadline,
// then cancels work and discards queued items. Active uploads stay charged
// until they return. Done signals final release, which may outlast Shutdown.
func (r *Recorder) Shutdown(ctx context.Context) error {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	if !r.stopping {
		r.stopping = true
		close(r.stop)
		close(r.queue)
		clear(r.tenants)
	}
	r.mu.Unlock()
	drain, cancel := r.deps.WithTimeout(ctx, r.cfg.ShutdownDrain)
	defer cancel()
	select {
	case <-r.done:
		return nil
	case <-drain.Done():
		r.cancel()
		for item := range r.queue {
			r.dequeued(item)
			r.recordShutdownDrop(item)
			reservation := item.reservation
			item = uploadItem{}
			r.release(reservation)
		}
		return drain.Err()
	}
}

// Done closes only after all preparations, workers and reservations are gone.
// Until then, uploads may still be using the borrowed bucket.
func (r *Recorder) Done() <-chan struct{} { return r.done }

// fixedWriter rejects writes beyond the reserved buffer.
type fixedWriter struct {
	dst []byte
	n   int
}

func (w *fixedWriter) Write(p []byte) (int, error) {
	n := copy(w.dst[w.n:], p)
	w.n += n
	if n != len(p) {
		return n, io.ErrShortWrite
	}
	return n, nil
}

func (r *Recorder) recordShutdownDrop(item uploadItem) {
	r.metrics.dropped.WithLabelValues(item.source, string(DropShutdown)).Inc()
	r.metrics.bytes.WithLabelValues(item.source, "dropped").Add(float64(len(item.data)))
}
