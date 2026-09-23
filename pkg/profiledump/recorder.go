package profiledump

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"math/rand/v2"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/time/rate"
)

// encodingReservation covers JSON buffers, map sorting, and item overhead until
// release. Payload scratch is reserved separately.
const encodingReservation int64 = 4*MaxMetadataSize + 4096

// HTTP encoding needs an extra marshal buffer and byte workspace.
const httpEncodingReservation int64 = 2 * MaxMetadataSize

// PolicyProvider returns current immutable snapshots and must be concurrency-safe.
type PolicyProvider interface{ ProfileDebugDump(tenantID string) Policy }

// UploadFunc receives a full capture key and tenant for SSE, without another
// tenant prefix. It must honor ctx and release body before returning.
// The recorder never closes borrowed storage.
type UploadFunc func(ctx context.Context, tenantID, key string, body io.Reader) error

// Dependencies inject policies, uploads, and test controls. Now and WithTimeout
// must be concurrency-safe. Random calls are serialized, defaulting to rand/v2.
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
// Call Shutdown to stop it. A nil *Recorder is a cheap disabled implementation.
func NewRecorder(cfg RecorderConfig, deps Dependencies) (*Recorder, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if deps.Policies == nil || deps.Upload == nil {
		return nil, fmt.Errorf("recorder requires policies and upload")
	}
	if err := validateText("distributor_id", deps.DistributorID, MaxTextBytes, true); err != nil {
		// Invalid diagnostic IDs must not prevent startup.
		deps.DistributorID = "unknown"
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

// Capture prepares owned bytes synchronously and enqueues without waiting or I/O.
// Work uses the recorder lifecycle and carries only request trace context.
// Outcomes must not become ingestion errors.
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
	if c.SupportsSelectors() && !p.Matches(c.SelectorLabels) {
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
	out.Format = c.Metadata.NativeFormat
	out.SourceProtocol = c.Metadata.SourceProtocol
	out.DistributorID = r.deps.DistributorID
	out.PolicyFingerprint = p.Fingerprint()
	if c.StartSpan != nil {
		var finish func(Outcome)
		if runCaptureCallback(func() { ctx, finish = c.StartSpan(ctx) }) {
			out.Reason = DropPanic
			return
		}
		if finish != nil {
			// Telemetry failures must not change an enqueue/drop outcome.
			defer func() { runCaptureCallback(func() { finish(out) }) }()
		}
	}
	if c.Payload == nil {
		out.Reason = DropInvalid
		return
	}
	var size, scratch int64
	var err error
	if runCaptureCallback(func() { size, scratch, err = c.Payload.Size() }) {
		out.Reason = DropPanic
		return
	}
	if err != nil || size < 0 || scratch < 0 {
		out.Reason = DropSerialization
		return
	}
	out.PayloadSize = size
	key, id, err := NewObjectKey(tenant, now, c.Metadata.NativeFormat)
	if err != nil {
		out.Reason = DropInvalid
		return
	}
	out.CaptureID, out.ObjectKey = id.String(), key
	m := c.Metadata
	m.SchemaVersion, m.CapturedAt, m.TenantID = Version, now.UTC(), tenant
	m.CaptureID, m.PolicyFingerprint, m.ActivationSource = out.CaptureID, p.Fingerprint(), ActivationRuntimeOverride
	m.DistributorID, m.PayloadSize = r.deps.DistributorID, size
	if err = m.Validate(); err != nil {
		out.Reason = DropInvalid
		return
	}
	metadataSize := encodedMetadataSize(m)
	out.Size, err = envelopeSize(metadataSize, size, math.MaxInt64)
	if err != nil || out.Size > r.cfg.MaxObjectBytes {
		out.Reason = DropTooLarge
		return
	}
	var reservation int64
	reservation, out.Reason = r.reservationSize(out.Size, scratch, m.HTTP != nil)
	if out.Reason != "" {
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
	data := make([]byte, int(out.Size))
	// A fixed writer prevents metadata sizing drift from growing another buffer.
	header := &fixedWriter{dst: data[:HeaderSize+int(metadataSize)]}
	if err = encodeHeader(header, m, r.cfg.MaxObjectBytes); err != nil || int64(header.n) != int64(HeaderSize)+metadataSize {
		out.Reason = DropSerialization
		return
	}
	payload := data[header.n:]
	workspace := make([]byte, int(scratch))
	var written int
	if runCaptureCallback(func() { written, err = c.Payload.Write(r.ctx, payload, workspace) }) {
		out.Reason = DropPanic
		return
	}
	if err != nil || written != len(payload) {
		out.Reason = DropSerialization
		return
	}
	// Queue only owned data and trace context.
	item := uploadItem{data: data, tenant: strings.Clone(tenant), key: key, source: source, span: trace.SpanContextFromContext(ctx), reservation: reservation}
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

func (r *Recorder) reservationSize(objectSize, scratch int64, hasHTTPMetadata bool) (int64, DropReason) {
	overhead := encodingReservation
	if hasHTTPMetadata {
		overhead += httpEncodingReservation
	}
	// Subtract to avoid overflow from untrusted sizes.
	if scratch > r.cfg.MaxRetainedBytes-objectSize-overhead || scratch > int64(math.MaxInt) {
		return 0, DropByteBudget
	}
	return objectSize + scratch + overhead, ""
}

// runCaptureCallback contains callback panics outside recorder locks.
// Discard panic values because they may contain customer data.
func runCaptureCallback(fn func()) (panicked bool) {
	defer func() {
		if recover() != nil {
			panicked = true
		}
	}()
	fn()
	return false
}

// admitRate requires mu and preserves token history across policy changes.
func (r *Recorder) admitRate(tenant string, p Policy, now time.Time) DropReason {
	if len(tenant) > MaxTenantBytes {
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
// then cancels work and discards queued items. Active callbacks stay charged
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
