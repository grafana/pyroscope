package writepath

import (
	"context"
	"fmt"
	"math/rand"
	"net"
	"net/http"
	"sync"
	"time"

	"connectrpc.com/connect"

	"github.com/go-kit/log"
	"github.com/grafana/dskit/services"
	"github.com/opentracing/opentracing-go"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"

	pushv1 "github.com/grafana/pyroscope/api/gen/proto/go/push/v1"
	segmentwriterv1 "github.com/grafana/pyroscope/api/gen/proto/go/segmentwriter/v1"
	"github.com/grafana/pyroscope/pkg/distributor/arrow"
	distributormodel "github.com/grafana/pyroscope/pkg/distributor/model"
	"github.com/grafana/pyroscope/pkg/tenant"
	"github.com/grafana/pyroscope/pkg/util"
	"github.com/grafana/pyroscope/pkg/util/connectgrpc"
	httputil "github.com/grafana/pyroscope/pkg/util/http"
)

type SegmentWriterClient interface {
	Push(context.Context, *segmentwriterv1.PushRequest) (*segmentwriterv1.PushResponse, error)
}

type IngesterClient interface {
	Push(context.Context, *distributormodel.ProfileSeries) (*connect.Response[pushv1.PushResponse], error)
}

type IngesterFunc func(
	context.Context,
	*distributormodel.ProfileSeries,
) (*connect.Response[pushv1.PushResponse], error)

func (f IngesterFunc) Push(
	ctx context.Context,
	req *distributormodel.ProfileSeries,
) (*connect.Response[pushv1.PushResponse], error) {
	return f(ctx, req)
}

type Overrides interface {
	WritePathOverrides(tenantID string) Config
}

type Router struct {
	service  services.Service
	inflight sync.WaitGroup

	logger    log.Logger
	overrides Overrides
	metrics   *metrics

	ingester      IngesterClient
	segwriter     IngesterClient
	arrowFlightSW SegmentWriterClient // Arrow Flight segmentwriter client
}

func NewRouter(
	logger log.Logger,
	registerer prometheus.Registerer,
	overrides Overrides,
	ingester IngesterClient,
	segwriter IngesterClient,
	arrowFlightSW SegmentWriterClient,
) *Router {
	r := &Router{
		logger:        logger,
		overrides:     overrides,
		metrics:       newMetrics(registerer),
		ingester:      ingester,
		segwriter:     segwriter,
		arrowFlightSW: arrowFlightSW,
	}
	r.service = services.NewBasicService(r.starting, r.running, r.stopping)
	return r
}

func (m *Router) Service() services.Service { return m.service }

func (m *Router) starting(context.Context) error { return nil }

func (m *Router) stopping(_ error) error {
	// We expect that no requests are routed after the stopping call.
	m.inflight.Wait()
	return nil
}

func (m *Router) running(ctx context.Context) error {
	<-ctx.Done()
	return nil
}

func (m *Router) Send(ctx context.Context, req *distributormodel.ProfileSeries) error {
	sp, ctx := opentracing.StartSpanFromContext(ctx, "Router.Send")
	defer sp.Finish()
	config := m.overrides.WritePathOverrides(req.TenantID)
	switch config.WritePath {
	case SegmentWriterPath:
		return m.send(m.segwriterRoute(true))(ctx, req)
	case CombinedPath:
		return m.sendToBoth(ctx, req, &config)
	default:
		return m.send(m.ingesterRoute())(ctx, req)
	}
}

func (m *Router) ingesterRoute() *route {
	return &route{
		path:    IngesterPath,
		primary: true, // Ingester is always the primary route.
		client:  m.ingester,
	}
}

func (m *Router) segwriterRoute(primary bool) *route {
	config := m.overrides.WritePathOverrides("")

	// Use Arrow Flight if enabled and available
	if config.ArrowFlight.Enabled && m.arrowFlightSW != nil {
		logger := log.With(m.logger, "component", "arrow-flight-segmentwriter")
		logger.Log("msg", "using Arrow Flight segmentwriter client")
		return &route{
			path:    SegmentWriterPath,
			primary: primary,
			client:  &arrowFlightIngesterWrapper{m.arrowFlightSW},
		}
	}

	// Fallback to ConnectRPC
	return &route{
		path:    SegmentWriterPath,
		primary: primary,
		client:  m.segwriter,
	}
}

// arrowFlightIngesterWrapper wraps SegmentWriterClient to implement IngesterClient interface
type arrowFlightIngesterWrapper struct {
	SegmentWriterClient
}

func (w *arrowFlightIngesterWrapper) Push(ctx context.Context, req *distributormodel.ProfileSeries) (*connect.Response[pushv1.PushResponse], error) {
	// Generate ProfileId - using UUID like the distributor does
	profileID := make([]byte, 16)
	// Use a simple random generation for now
	// In production, this would use uuid.New()
	for i := range profileID {
		profileID[i] = byte(rand.Intn(256))
	}

	// Calculate shard from labels using a simple hash
	// This mirrors the logic in segmentwriter/client but simplified
	// In production, this should use the actual distributor logic
	var shard uint32 = 1 // Default to shard 1 (not 0 which is sentinel)
	if len(req.Labels) > 0 {
		// Hash the labels to determine shard
		// For now, use a simple hash of the service name or first label
		for _, label := range req.Labels {
			if label.Name == "service_name" || label.Name == "__name__" {
				// Simple hash: sum of bytes mod some number of shards
				// Assuming 8 shards for simplicity
				hash := uint32(0)
				for _, c := range label.Value {
					hash = hash*31 + uint32(c)
				}
				shard = (hash % 8) + 1 // Shard IDs start at 1
				break
			}
		}
	}

	// Convert ProfileSeries to PushRequest for Arrow Flight
	pushReq := &segmentwriterv1.PushRequest{
		TenantId:    req.TenantID,
		Labels:      req.Labels,
		Annotations: req.Annotations,
		ProfileId:   profileID,
		Shard:       shard, // Set the computed shard
	}

	// Convert profile to Arrow format
	if req.Profile != nil && req.Profile.Profile != nil {
		pool := arrow.NewMemoryPool()

		// Debug: Check what's in the profile BEFORE conversion
		samplesCount := len(req.Profile.Profile.Sample)
		locationsCount := len(req.Profile.Profile.Location)
		stringsCount := len(req.Profile.Profile.StringTable)

		arrowData, err := arrow.ProfileToArrow(req.Profile.Profile, pool)
		if err != nil {
			// Log the error - this is critical!
			return nil, fmt.Errorf("failed to convert profile to Arrow format: %w", err)
		}

		// Debug: Check what Arrow produced
		samplesSize := len(arrowData.SamplesBatch)
		locationsSize := len(arrowData.LocationsBatch)
		stringsSize := len(arrowData.StringsBatch)

		if samplesCount == 0 || samplesSize == 0 {
			return nil, fmt.Errorf("profile conversion produced empty data: input samples=%d locations=%d strings=%d, output samples_bytes=%d locations_bytes=%d strings_bytes=%d",
				samplesCount, locationsCount, stringsCount, samplesSize, locationsSize, stringsSize)
		}

		pushReq.ArrowProfile = arrowData
	} else {
		// No profile data available
		return nil, fmt.Errorf("no profile data in ProfileSeries")
	}

	_, err := w.SegmentWriterClient.Push(ctx, pushReq)
	if err != nil {
		return nil, err
	}

	// Convert response
	return connect.NewResponse(&pushv1.PushResponse{}), nil
}

// isArrowFlightAvailable checks if Arrow Flight server is available
func (m *Router) isArrowFlightAvailable(logger log.Logger) bool {
	config := m.overrides.WritePathOverrides("")
	addr := fmt.Sprintf("%s:%d", config.ArrowFlight.Address, config.ArrowFlight.Port)

	// Quick connection test
	conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if err != nil {
		logger.Log("msg", "Arrow Flight server not available", "addr", addr, "error", err)
		return false
	}
	conn.Close()

	logger.Log("msg", "Arrow Flight server available", "addr", addr)
	return true
}

func (m *Router) sendToBoth(ctx context.Context, req *distributormodel.ProfileSeries, config *Config) error {
	r := rand.Float64() // [0.0, 1.0)
	shouldIngester := config.IngesterWeight > 0.0 && config.IngesterWeight >= r
	shouldSegwriter := config.SegmentWriterWeight > 0.0 && config.SegmentWriterWeight >= r

	// Client sees errors and latency of the primary write
	// path, secondary write path does not affect the client.
	var ingester, segwriter *route
	if shouldIngester {
		// If the request is sent to ingester (regardless of anything),
		// the response is returned to the client immediately after the old
		// write path returns. Failure of the new write path should be logged
		// and counted in metrics but NOT returned to the client.
		ingester = m.ingesterRoute()
		if !shouldSegwriter {
			return m.send(ingester)(ctx, req)
		}
	}
	if shouldSegwriter {
		segwriter = m.segwriterRoute(!shouldIngester)
		if segwriter.primary && !config.AsyncIngest {
			// The request is sent to segment-writer exclusively, and the client
			// must block until the response returns.
			// Failure of the new write is returned to the client.
			// Failure of the old write path is NOT returned to the client.
			return m.send(segwriter)(ctx, req)
		}
		// Request to the segment writer will be sent asynchronously.
	}

	// No write routes. This is possible if the write path is configured
	// to "combined" and both weights are set to 0.0.
	if ingester == nil && segwriter == nil {
		return nil
	}

	if segwriter != nil && ingester != nil {
		// The request is to be sent to both asynchronously, therefore we're
		// cloning it. We do not wait for the secondary request to complete.
		// On shutdown, however, we will wait for all inflight requests.
		segwriter.client = m.sendClone(ctx, req.Clone(), segwriter.client, config)
	}

	if segwriter != nil {
		m.sendAsync(ctx, req, segwriter)
	}

	if ingester != nil {
		select {
		case err := <-m.sendAsync(ctx, req, ingester):
			return err
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	return nil
}

type sendFunc func(context.Context, *distributormodel.ProfileSeries) error

type route struct {
	path    WritePath // IngesterPath | SegmentWriterPath
	client  IngesterClient
	primary bool
}

func (m *Router) sendClone(ctx context.Context, req *distributormodel.ProfileSeries, client IngesterClient, config *Config) IngesterFunc {
	return func(context.Context, *distributormodel.ProfileSeries) (*connect.Response[pushv1.PushResponse], error) {
		localCtx, cancel := context.WithTimeout(context.Background(), config.SegmentWriterTimeout)
		localCtx = tenant.InjectTenantID(localCtx, req.TenantID)
		if sp := opentracing.SpanFromContext(ctx); sp != nil {
			localCtx = opentracing.ContextWithSpan(localCtx, sp)
		}
		defer cancel()
		return client.Push(localCtx, req)
	}
}

func (m *Router) sendAsync(ctx context.Context, req *distributormodel.ProfileSeries, r *route) <-chan error {
	c := make(chan error, 1)
	m.inflight.Add(1)
	go func() {
		defer m.inflight.Done()
		c <- m.send(r)(ctx, req)
	}()
	return c
}

func (m *Router) send(r *route) sendFunc {
	return func(ctx context.Context, req *distributormodel.ProfileSeries) (err error) {
		start := time.Now()
		defer func() {
			if p := recover(); p != nil {
				err = util.PanicError(p)
			}
			// Note that the upstream expects "connect" codes.
			code := http.StatusOK // HTTP status code.
			if err != nil {
				var connectErr *connect.Error
				if ok := errors.As(err, &connectErr); ok {
					// connect errors are passed as is, we only
					// identify the HTTP status code.
					code = int(connectgrpc.CodeToHTTP(connectErr.Code()))
				} else {
					// We identify the HTTP status code based on the
					// error and then convert the error to connect error.
					code, _ = httputil.ClientHTTPStatusAndError(err)
					err = connect.NewError(connectgrpc.HTTPToCode(int32(code)), err)
				}
			}
			m.metrics.durationHistogram.
				WithLabelValues(newDurationHistogramDims(r, code)...).
				Observe(time.Since(start).Seconds())
		}()
		_, err = r.client.Push(ctx, req)
		return err
	}
}
