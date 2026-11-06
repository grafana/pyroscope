package profiledump

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"strings"
	"sync"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/grafana/dskit/services"
	"github.com/prometheus/client_golang/prometheus"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/sync/semaphore"
	"golang.org/x/time/rate"

	pushv1 "github.com/grafana/pyroscope/api/gen/proto/go/push/v1"
	"github.com/grafana/pyroscope/v2/pkg/model"
)

const (
	defaultQueueCapacity     = 16
	defaultWorkers           = 2
	defaultProcessBurst      = 10
	processCapturesPerSecond = 10
	recorderUploadTimeout    = 10 * time.Second
)

const (
	sourceConnect = "connect"
	formatPprof   = "pprof"
)

type dropReason string

const (
	dropSelector    dropReason = "selector"
	dropSampled     dropReason = "sampled"
	dropProcessRate dropReason = "process_rate"
	dropTooLarge    dropReason = "too_large"
	dropByteBudget  dropReason = "byte_budget"
	dropInvalid     dropReason = "invalid"
	dropQueueFull   dropReason = "queue_full"
)

// PolicyProvider supplies current immutable snapshots to concurrent admissions.
type PolicyProvider interface {
	ProfileDump(tenantID string) Policy
}

// UploadFunc receives a full capture key and tenant for SSE, without another
// tenant prefix. The body is borrowed for the duration of the call.
// The recorder never closes borrowed storage.
type UploadFunc func(ctx context.Context, tenantID, key string, body io.Reader) error

type uploadItem struct {
	payload, metadataJSON    []byte
	tenant, key, metadataKey string
	span                     trace.SpanContext
}

type Recorder struct {
	*services.BasicService
	cfg           RecorderConfig
	policies      PolicyProvider
	uploadFn      UploadFunc
	distributorID string
	logger        log.Logger
	metrics       recorderMetrics
	queue         chan uploadItem
	capacity      *semaphore.Weighted
	process       *rate.Limiter
	now           func() time.Time
	random        func() float64
	workers       int
}

// NewRecorder validates and allocates without starting background work.
// Start the service before capture. A nil *Recorder disables capture.
func NewRecorder(cfg RecorderConfig, policies PolicyProvider, upload UploadFunc, distributorID string, logger log.Logger, reg prometheus.Registerer) (*Recorder, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if policies == nil || upload == nil {
		return nil, fmt.Errorf("recorder requires policies and upload")
	}
	r := &Recorder{
		cfg: cfg, policies: policies, uploadFn: upload, distributorID: distributorID,
		logger: logger, now: time.Now, random: rand.Float64,
		workers: defaultWorkers, queue: make(chan uploadItem, defaultQueueCapacity),
		capacity: semaphore.NewWeighted(cfg.MaxRetainedBytes),
		process:  rate.NewLimiter(processCapturesPerSecond, defaultProcessBurst),
	}
	r.metrics = newRecorderMetrics(reg)
	r.BasicService = services.NewBasicService(nil, r.run, nil)
	return r, nil
}

func (r *Recorder) run(ctx context.Context) error {
	var workers sync.WaitGroup
	for range r.workers {
		workers.Add(1)
		go func() {
			defer workers.Done()
			r.worker(ctx)
		}()
	}
	// Shutdown abandons queued captures, whose buffers remain bounded by max_retained_bytes.
	workers.Wait()
	return nil
}

// CaptureSeries selects once per incoming series, then attempts each profile
// independently against resource limits. It borrows the series until returning,
// by which time accepted asynchronous work owns its payload and metadata bytes.
func (r *Recorder) CaptureSeries(ctx context.Context, tenant string, series *pushv1.RawProfileSeries) {
	if r == nil || series == nil || len(series.Samples) == 0 {
		return
	}
	captureTime := r.now()
	policy := r.policies.ProfileDump(tenant)
	if !policy.ActiveAt(captureTime) {
		return
	}
	if r.State() != services.Running || r.ServiceContext().Err() != nil {
		return
	}
	reason := r.selectSeries(policy, model.Labels(series.Labels))
	var labelSize int64
	if reason == "" {
		// The enclosing message adds per-entry framing, so nil and empty labels count.
		labelSize = int64((&pushv1.RawProfileSeries{Labels: series.Labels}).SizeVT())
	}
	for _, sample := range series.Samples {
		if sample == nil {
			continue
		}
		if reason != "" {
			r.metrics.admission(reason)
			continue
		}
		// Bound protobuf input size before constructing JSON metadata.
		// Temporary JSON allocations are not covered by the retained-byte budget.
		if labelSize > r.cfg.MaxCaptureBytes || int64(sample.SizeVT()) > r.cfg.MaxCaptureBytes-labelSize {
			r.metrics.admission(dropTooLarge)
			continue
		}
		if drop := r.admitProfile(); drop != "" {
			r.metrics.admission(drop)
			continue
		}
		r.capture(ctx, tenant, Metadata{
			SourceProtocol:    sourceConnect,
			Format:            formatPprof,
			Labels:            series.Labels,
			OriginalProfileID: sample.ID,
		}, sample.RawProfile, captureTime)
	}
}

// selectSeries matches and samples an active policy snapshot.
func (r *Recorder) selectSeries(p Policy, labels model.Labels) dropReason {
	if !p.Matches(labels) {
		return dropSelector
	}
	if p.Probability() < 1 && r.random() >= p.Probability() {
		return dropSampled
	}
	return ""
}

// admitProfile checks rate before constructing profile metadata.
// Out-of-order timestamps from concurrent admissions can cause the limiter to allow extra captures.
func (r *Recorder) admitProfile() dropReason {
	admissionTime := r.now()
	if !r.process.AllowN(admissionTime, 1) {
		return dropProcessRate
	}
	return ""
}

// capture attempts a nonblocking capture after selection and rate admission.
// Work uses the recorder lifecycle and carries only request trace context.
func (r *Recorder) capture(ctx context.Context, tenant string, metadata Metadata, payload []byte, captureTime time.Time) (reason dropReason) {
	defer func() { r.metrics.admission(reason) }()
	item, reason := r.prepareCapture(tenant, metadata, payload, captureTime)
	if reason != "" {
		return reason
	}
	transferred := false
	defer func() {
		if !transferred {
			r.release(item)
		}
	}()
	item.span = trace.SpanContextFromContext(ctx)
	// Never block ingestion waiting for an upload worker.
	select {
	case r.queue <- item:
		transferred = true
		return ""
	default:
		return dropQueueFull
	}
}

// prepareCapture checks the serialized size and acquires capacity before copying the payload.
func (r *Recorder) prepareCapture(tenant string, m Metadata, payload []byte, now time.Time) (uploadItem, dropReason) {
	payloadSize := int64(len(payload))
	key, id, err := NewObjectKey(tenant, now)
	if err != nil {
		return uploadItem{}, dropInvalid
	}
	m.SchemaVersion, m.CapturedAt, m.TenantID = schemaVersion, now.UTC(), tenant
	m.CaptureID = id.String()
	m.DistributorID, m.PayloadSize = r.distributorID, payloadSize
	metadataJSON, err := json.Marshal(m)
	if err != nil {
		return uploadItem{}, dropInvalid
	}
	if payloadSize > r.cfg.MaxCaptureBytes || int64(len(metadataJSON)) > r.cfg.MaxCaptureBytes-payloadSize {
		return uploadItem{}, dropTooLarge
	}
	if !r.capacity.TryAcquire(payloadSize + int64(len(metadataJSON))) {
		return uploadItem{}, dropByteBudget
	}
	ownedPayload := make([]byte, len(payload))
	copy(ownedPayload, payload)

	metadataKey := strings.TrimSuffix(key, ".pprof") + ".json"
	// Avoid retaining a larger tenant header after metadata stripping.
	return uploadItem{key: key, metadataKey: metadataKey, metadataJSON: metadataJSON, payload: ownedPayload, tenant: strings.Clone(tenant)}, ""
}

func (r *Recorder) release(item uploadItem) {
	size := int64(len(item.payload)) + int64(len(item.metadataJSON))
	r.capacity.Release(size)
}
func (r *Recorder) worker(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case item := <-r.queue:
			if ctx.Err() != nil {
				r.release(item)
				return
			}
			r.upload(ctx, item)
			r.release(item)
		}
	}
}
func (r *Recorder) upload(ctx context.Context, item uploadItem) {
	ctx, cancel := context.WithTimeout(trace.ContextWithSpanContext(ctx, item.span), recorderUploadTimeout)
	defer cancel()
	operation, key := "payload", item.key
	err := r.uploadFn(ctx, item.tenant, key, bytes.NewReader(item.payload))
	if err == nil && ctx.Err() == nil {
		operation, key = "sidecar", item.metadataKey
		err = r.uploadFn(ctx, item.tenant, key, bytes.NewReader(item.metadataJSON))
	}
	// Only the recorder lifecycle cancels this context before its deadline.
	if errors.Is(ctx.Err(), context.Canceled) {
		return
	}
	if err != nil || ctx.Err() != nil {
		result := "error"
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			result = "timeout"
		}
		if err == nil {
			err = ctx.Err()
		}
		level.Error(r.logger).Log("msg", "failed to upload profile dump", "operation", operation, "tenant", item.tenant, "key", key, "err", err)
		r.metrics.uploads.WithLabelValues(result).Inc()
		return
	}
	r.metrics.uploads.WithLabelValues("success").Inc()
}
