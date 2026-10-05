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

	"github.com/grafana/dskit/services"
	"github.com/prometheus/client_golang/prometheus"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/time/rate"
)

// itemReservation covers bounded keys, tenant identity and per-item bookkeeping.
const itemReservation int64 = 4096

const (
	shutdownDrain            = 15 * time.Second
	defaultQueueCapacity     = 16
	defaultWorkers           = 2
	defaultTenantBurst       = 1
	defaultProcessBurst      = 2
	defaultMaxTenantLimiters = 1024
)

// PolicyProvider supplies current immutable snapshots to concurrent admissions.
type PolicyProvider interface{ ProfileDebugDump(tenantID string) Policy }

// UploadFunc receives a full capture key and tenant for SSE, without another
// tenant prefix. The body is borrowed for the duration of the call.
// The recorder never closes borrowed storage.
type UploadFunc func(ctx context.Context, tenantID, key string, body io.Reader) error

// Dependencies inject policies, uploads, and test controls. Now is called
// concurrently. Random calls are serialized, defaulting to rand/v2.
type Dependencies struct {
	Policies      PolicyProvider
	Upload        UploadFunc
	DistributorID string
	Registerer    prometheus.Registerer
	Now           func() time.Time
	Random        func() float64
}

type uploadItem struct {
	payload, metadataJSON    []byte
	tenant, key, metadataKey string
	source                   string
	span                     trace.SpanContext
	reservation              int64
}

type Recorder struct {
	*services.BasicService
	cfg      RecorderConfig
	deps     Dependencies
	metrics  recorderMetrics
	queue    chan uploadItem
	mu       sync.Mutex     // admission, accounting, limiter state and queue closure
	pending  sync.WaitGroup // preparations and upload workers
	retained int64
	process  *rate.Limiter
	tenants  map[string]*rate.Limiter

	// Fixed in production. Package tests adjust these before starting the service.
	workers, tenantBurst, maxTenantLimiters int
}

// NewRecorder validates and allocates without starting background work.
// Start the service before capture. A nil *Recorder disables capture.
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
	r := &Recorder{workers: defaultWorkers, tenantBurst: defaultTenantBurst, maxTenantLimiters: defaultMaxTenantLimiters, cfg: cfg, deps: deps, metrics: newRecorderMetrics(deps.Registerer),
		queue:   make(chan uploadItem, defaultQueueCapacity),
		process: rate.NewLimiter(rate.Limit(cfg.ProcessCapturesPerSecond), defaultProcessBurst), tenants: make(map[string]*rate.Limiter)}
	r.BasicService = services.NewBasicService(nil, r.run, nil)
	return r, nil
}

func (r *Recorder) run(ctx context.Context) error {
	// Uploads carry only trace context from ingestion, and may drain after stop.
	uploadCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	for range r.workers {
		r.pending.Add(1)
		go func() {
			defer r.pending.Done()
			r.worker(uploadCtx)
		}()
	}
	<-ctx.Done()
	r.mu.Lock()
	// Admission checks the canceled service context under this same lock.
	// No preparation can be added or item sent after this barrier.
	close(r.queue)
	clear(r.tenants)
	r.mu.Unlock()

	timer := time.AfterFunc(shutdownDrain, cancel)
	defer timer.Stop()
	// Workers alone consume the queue. Noncooperative uploads can delay this
	// join and retain bounded queued buffers until a worker returns.
	r.pending.Wait()
	return nil
}

// accepting requires mu, also used by the queue closure and preparation barrier.
func (r *Recorder) accepting() bool {
	return r.State() == services.Running && r.ServiceContext().Err() == nil
}

// PolicyActive is an allocation-free hint to skip inactive tenants, without
// admission or metrics. Capture rechecks the policy and deadline.
func (r *Recorder) PolicyActive(tenant string) bool {
	return r != nil && r.deps.Policies.ProfileDebugDump(tenant).ActiveAt(r.deps.Now())
}

// Capture prepares an owned native pair and attempts a nonblocking enqueue.
// Work uses the recorder lifecycle and carries only request trace context.
func (r *Recorder) Capture(ctx context.Context, tenant string, c Candidate) (out Outcome) {
	return r.capture(ctx, tenant, c, nil)
}

func (r *Recorder) capture(ctx context.Context, tenant string, c Candidate, series *SeriesCapture) (out Outcome) {
	if r == nil {
		return Outcome{Reason: DropDisabled}
	}
	source := metricSource(c.Metadata.SourceProtocol)
	defer func() { r.metrics.admission(source, out) }()
	captureTime := r.deps.Now()
	p := r.deps.Policies.ProfileDebugDump(tenant)
	if p.Fingerprint() == "" {
		out.Reason = DropDisabled
		return
	}
	if !p.ActiveAt(captureTime) {
		out.Reason = DropExpired
		return
	}
	matched := false
	if series == nil {
		matched = p.Matches(c.SelectorLabels)
	} else {
		if series.fingerprint != p.Fingerprint() {
			series.matched = p.Matches(series.labels)
			series.fingerprint = p.Fingerprint()
		}
		matched = series.matched
	}
	if !matched {
		out.Reason = DropSelector
		return
	}
	r.mu.Lock()
	if !r.accepting() {
		r.mu.Unlock()
		out.Reason = DropShutdown
		return
	}
	if p.Probability() < 1 && r.deps.Random() >= p.Probability() {
		r.mu.Unlock()
		out.Reason = DropSampled
		return
	}
	// Read time under the admission lock so concurrent captures cannot reorder it.
	admissionTime := r.deps.Now()
	out.Reason = r.admitRate(tenant, p, admissionTime)
	if out.Reason != "" {
		r.mu.Unlock()
		return
	}
	r.pending.Add(1)
	r.mu.Unlock()
	defer r.pending.Done()
	ctx, finishSpan := startCaptureSpan(ctx)
	defer func() { finishSpan(out) }()
	prepared, out := r.prepareCapture(tenant, c, p, captureTime)
	if out.Reason != "" {
		return out
	}
	reservation, ok := r.reservationSize(out.PayloadSize, int64(cap(prepared.metadataJSON)))
	if !ok {
		out.Reason = DropByteBudget
		return
	}
	r.mu.Lock()
	if !r.accepting() {
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
	item := prepared
	item.payload = make([]byte, len(c.Payload))
	copy(item.payload, c.Payload)
	item.tenant = strings.Clone(tenant)
	item.source, item.span, item.reservation = source, trace.SpanContextFromContext(ctx), reservation
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.accepting() {
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

func (r *Recorder) prepareCapture(tenant string, c Candidate, p Policy, now time.Time) (uploadItem, Outcome) {
	var out Outcome
	out.Format = c.Metadata.NativeFormat
	out.SourceProtocol = c.Metadata.SourceProtocol
	out.DistributorID = r.deps.DistributorID
	out.PolicyFingerprint = p.Fingerprint()
	size := int64(len(c.Payload))
	out.PayloadSize = size
	key, id, err := NewNativeObjectKey(tenant, now)
	if err != nil {
		out.Reason = DropInvalid
		return uploadItem{}, out
	}
	out.CaptureID, out.ObjectKey = id.String(), key
	m := c.Metadata
	m.SchemaVersion, m.CapturedAt, m.TenantID = NativeSchemaVersion, now.UTC(), tenant
	m.CaptureID, m.PolicyFingerprint = out.CaptureID, p.Fingerprint()
	m.DistributorID, m.PayloadSize = r.deps.DistributorID, size
	if err = m.Validate(key); err != nil {
		out.Reason = DropInvalid
		return uploadItem{}, out
	}
	metadataJSON, err := MarshalNativeMetadata(key, m)
	if err != nil {
		out.Reason = DropTooLarge
		return uploadItem{}, out
	}
	out.Size, err = nativeCaptureSize(size, int64(len(metadataJSON)), r.cfg.MaxObjectBytes)
	if err != nil {
		out.Reason = DropTooLarge
		return uploadItem{}, out
	}
	keys, err := ParseNativeObjectKey(key)
	if err != nil {
		out.Reason = DropInvalid
		return uploadItem{}, out
	}
	return uploadItem{key: keys.PayloadKey, metadataKey: keys.MetadataKey, metadataJSON: metadataJSON}, out
}

// nativeCaptureSize checks the sum without overflowing, including at int64 limits.
func nativeCaptureSize(payloadSize, metadataSize, limit int64) (int64, error) {
	if payloadSize < 0 || metadataSize < 0 || metadataSize > limit || payloadSize > limit-metadataSize {
		return 0, fmt.Errorf("native capture exceeds size limit")
	}
	return payloadSize + metadataSize, nil
}

// reservationSize covers both owned buffers and bounded item overhead.
func (r *Recorder) reservationSize(payloadSize, metadataCapacity int64) (int64, bool) {
	remaining := r.cfg.MaxRetainedBytes
	for _, n := range [...]int64{payloadSize, metadataCapacity, itemReservation} {
		if n < 0 || n > remaining {
			return 0, false
		}
		remaining -= n
	}
	return r.cfg.MaxRetainedBytes - remaining, true
}

// admitRate requires mu and preserves token history across policy changes.
func (r *Recorder) admitRate(tenant string, p Policy, admissionTime time.Time) DropReason {
	if ValidateNativeTenant(tenant) != nil {
		return DropInvalid
	}
	l := r.tenants[tenant]
	if l == nil {
		if len(r.tenants) >= r.maxTenantLimiters {
			r.pruneLocked(admissionTime)
		}
		if len(r.tenants) >= r.maxTenantLimiters {
			return DropLimiterCapacity
		}
		l = rate.NewLimiter(rate.Limit(p.MaxCapturesPerSecond()), r.tenantBurst)
		r.tenants[strings.Clone(tenant)] = l
	} else if l.Limit() != rate.Limit(p.MaxCapturesPerSecond()) {
		l.SetLimitAt(admissionTime, rate.Limit(p.MaxCapturesPerSecond()))
	}
	if !l.AllowN(admissionTime, 1) {
		return DropTenantRate
	}
	if !r.process.AllowN(admissionTime, 1) {
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
func (r *Recorder) release(n int64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.retained -= n
	r.metrics.retained.Sub(float64(n))
}
func (r *Recorder) dequeued(item uploadItem) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.metrics.queueItems.Dec()
	r.metrics.queueBytes.Sub(float64(len(item.payload)) + float64(len(item.metadataJSON)))
}
func (r *Recorder) worker(ctx context.Context) {
	for item := range r.queue {
		r.dequeued(item)
		if ctx.Err() == nil {
			r.upload(ctx, item)
		} else {
			r.recordShutdownDrop(item)
		}
		reservation := item.reservation
		item = uploadItem{}
		r.release(reservation)
	}
}
func (r *Recorder) upload(ctx context.Context, item uploadItem) {
	ctx, cancel := context.WithTimeout(trace.ContextWithSpanContext(ctx, item.span), r.cfg.UploadTimeout)
	defer cancel()
	started := r.deps.Now()
	err := r.deps.Upload(ctx, item.tenant, item.key, bytes.NewReader(item.payload))
	if err == nil && ctx.Err() == nil {
		err = r.deps.Upload(ctx, item.tenant, item.metadataKey, bytes.NewReader(item.metadataJSON))
	}
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
		return
	}
	r.metrics.uploads.WithLabelValues(item.source, "success").Inc()
	r.metrics.bytes.WithLabelValues(item.source, "uploaded").Add(float64(len(item.payload)) + float64(len(item.metadataJSON)))
}

func (r *Recorder) recordShutdownDrop(item uploadItem) {
	r.metrics.dropped.WithLabelValues(item.source, string(DropShutdown)).Inc()
	r.metrics.bytes.WithLabelValues(item.source, "dropped").Add(float64(len(item.payload)) + float64(len(item.metadataJSON)))
}
