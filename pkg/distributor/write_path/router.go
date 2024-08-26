package writepath

import (
	"bytes"
	"context"
	"fmt"
	"math/rand"
	"sync"
	"time"

	"connectrpc.com/connect"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/google/uuid"
	"github.com/grafana/dskit/services"
	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/sync/errgroup"

	pushv1 "github.com/grafana/pyroscope/api/gen/proto/go/push/v1"
	segmentwriterv1 "github.com/grafana/pyroscope/api/gen/proto/go/segmentwriter/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	distributormodel "github.com/grafana/pyroscope/pkg/distributor/model"
	phlaremodel "github.com/grafana/pyroscope/pkg/model"
	"github.com/grafana/pyroscope/pkg/util"
)

type SegmentWriterClient interface {
	Push(context.Context, *segmentwriterv1.PushRequest) (*segmentwriterv1.PushResponse, error)
}

type IngesterClient interface {
	Push(context.Context, *distributormodel.PushRequest) (*connect.Response[pushv1.PushResponse], error)
}

type IngesterFunc func(
	context.Context,
	*distributormodel.PushRequest,
) (*connect.Response[pushv1.PushResponse], error)

func (f IngesterFunc) Push(
	ctx context.Context,
	req *distributormodel.PushRequest,
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

	ingester  IngesterClient
	segwriter SegmentWriterClient
}

func NewRouter(
	logger log.Logger,
	registerer prometheus.Registerer,
	overrides Overrides,
	ingester IngesterClient,
	segwriter SegmentWriterClient,
) *Router {
	r := &Router{
		logger:    logger,
		overrides: overrides,
		metrics:   newMetrics(registerer),
		ingester:  ingester,
		segwriter: segwriter,
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

func (m *Router) Send(ctx context.Context, req *distributormodel.PushRequest) error {
	config := m.overrides.WritePathOverrides(req.TenantID)
	switch config.WritePath {
	case SegmentWriterPath:
		return m.send(m.segwriterRoute(true))(ctx, req)
	case CombinedPath:
		return m.sendToBoth(ctx, req, config)
	default:
		return m.send(m.ingesterRoute())(ctx, req)
	}
}

func (m *Router) ingesterRoute() *route {
	return &route{
		path:    IngesterPath,
		primary: true, // Ingester is always the primary route.
		send: func(ctx context.Context, request *distributormodel.PushRequest) error {
			_, err := m.ingester.Push(ctx, request)
			return err
		},
	}
}

func (m *Router) segwriterRoute(primary bool) *route {
	return &route{
		path:    SegmentWriterPath,
		primary: primary,
		send: func(ctx context.Context, req *distributormodel.PushRequest) error {
			// Prepare the requests: we're trying to avoid allocating extra
			// memory for serialized profiles by reusing the source request
			// capacities, iff the request won't be sent to ingester.
			requests := convertRequest(req, !primary)
			return m.sendRequestsToSegmentWriter(ctx, requests)
		},
	}
}

func (m *Router) sendToBoth(ctx context.Context, req *distributormodel.PushRequest, config Config) error {
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
		if segwriter.primary {
			// If the request is sent to segment-writer exclusively:
			// the response returns to the client when the new write path
			// returns.
			// Failure of the new write is returned to the client.
			// Failure of the old write path is NOT returned to the client.
			return m.send(segwriter)(ctx, req)
		}
	}

	// No write routes. This is possible if the write path is configured
	// to "combined" and both weights are set to 0.0.
	if ingester == nil && segwriter == nil {
		return nil
	}

	// If we ended up here, ingester is the primary route,
	// and segment-writer is the secondary route.
	c := m.sendAsync(ctx, req, ingester)
	// We do not wait for the secondary request to complete.
	// On shutdown, however, we will wait for all inflight
	// requests to complete.
	m.sendAsync(ctx, req, segwriter)

	select {
	case err := <-c:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

type sendFunc func(context.Context, *distributormodel.PushRequest) error

type route struct {
	path    WritePath // IngesterPath | SegmentWriterPath
	send    sendFunc
	primary bool
}

func (m *Router) sendAsync(ctx context.Context, req *distributormodel.PushRequest, r *route) <-chan error {
	c := make(chan error, 1)
	m.inflight.Add(1)
	go func() {
		defer m.inflight.Done()
		c <- m.send(r)(ctx, req)
	}()
	return c
}

func (m *Router) send(r *route) sendFunc {
	return func(ctx context.Context, req *distributormodel.PushRequest) (err error) {
		start := time.Now()
		defer func() {
			if p := recover(); p != nil {
				err = util.PanicError(p)
			}
			dims := newDurationHistogramDims(r, err)
			if err != nil {
				_ = level.Warn(m.logger).Log(
					"msg", "write path request failed",
					"path", dims.path,
					"primary", dims.primary,
					"status", dims.status,
					"err", err,
				)
			}
			m.metrics.durationHistogram.
				WithLabelValues(dims.slice()...).
				Observe(time.Since(start).Seconds())
		}()
		return r.send(ctx, req)
	}
}

func (m *Router) sendRequestsToSegmentWriter(ctx context.Context, requests []*segmentwriterv1.PushRequest) error {
	// In all the known cases, we only have a single profile.
	// We should avoid batching multiple profiles into a single request:
	// overhead of handling multiple profiles in a single request is
	// substantial: we need to allocate memory for all profiles at once,
	// and wait for multiple requests routed to different shards to complete
	// is generally a bad idea because it's hard to reason about latencies,
	// retries, and error handling.
	if len(requests) == 1 {
		_, err := m.segwriter.Push(ctx, requests[0])
		return err
	}
	// Fallback. We should minimize probability of this branch.
	g, ctx := errgroup.WithContext(ctx)
	for _, r := range requests {
		r := r
		g.Go(func() error {
			_, err := m.segwriter.Push(ctx, r)
			return err
		})
	}
	return g.Wait()
}

func convertRequest(req *distributormodel.PushRequest, copy bool) []*segmentwriterv1.PushRequest {
	r := make([]*segmentwriterv1.PushRequest, 0, len(req.Series)*2)
	for _, s := range req.Series {
		for _, p := range s.Samples {
			r = append(r, convertProfile(p, s.Labels, req.TenantID, copy))
		}
	}
	return r
}

func convertProfile(
	sample *distributormodel.ProfileSample,
	labels []*typesv1.LabelPair,
	tenantID string,
	copy bool,
) *segmentwriterv1.PushRequest {
	var b *bytes.Buffer
	if copy {
		b = bytes.NewBuffer(make([]byte, 0, cap(sample.RawProfile)))
	} else {
		b = bytes.NewBuffer(sample.RawProfile[:0])
	}
	if _, err := sample.Profile.WriteTo(b); err != nil {
		panic(fmt.Sprintf("failed to marshal profile: %v", err))
	}
	profileID := uuid.New()
	return &segmentwriterv1.PushRequest{
		TenantId: tenantID,
		// Note that labels are always copied because
		// the API allows multiple profiles to refer to
		// the same label set.
		Labels:    phlaremodel.Labels(labels).Clone(),
		Profile:   b.Bytes(),
		ProfileId: profileID[:],
	}
}
