package writepath

import (
	"context"
	"math/rand"
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
	distributormodel "github.com/grafana/pyroscope/pkg/distributor/model"
	"github.com/grafana/pyroscope/pkg/tenant"
	"github.com/grafana/pyroscope/pkg/util"
	"github.com/grafana/pyroscope/pkg/util/connectgrpc"
	"github.com/grafana/pyroscope/pkg/util/delayhandler"
	httputil "github.com/grafana/pyroscope/pkg/util/http"
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

type Router struct {
	service  services.Service
	inflight sync.WaitGroup

	logger  log.Logger
	metrics *metrics

	ingester  IngesterClient
	segwriter IngesterClient
}

func NewRouter(
	logger log.Logger,
	registerer prometheus.Registerer,
	ingester IngesterClient,
	segwriter IngesterClient,
) *Router {
	r := &Router{
		logger:    logger,
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

func (m *Router) Send(ctx context.Context, req *distributormodel.PushRequest, config Config) error {
	if config.AsyncIngest {
		delayhandler.CancelDelay(ctx)
	}
	switch config.WritePath {
	case SegmentWriterPath:
		return m.sendToSegmentWriterOnly(ctx, req, &config)
	case CombinedPath:
		return m.sendToBoth(ctx, req, &config)
	default:
		return m.sendToIngesterOnly(ctx, req)
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
	return &route{
		path:    SegmentWriterPath,
		primary: primary,
		client:  m.segwriter,
	}
}

func (m *Router) sendToBoth(ctx context.Context, req *distributormodel.PushRequest, config *Config) error {
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
		segwriter.client = m.detachedClient(ctx, req.Clone(), segwriter.client, config)
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

func (m *Router) sendToSegmentWriterOnly(ctx context.Context, req *distributormodel.PushRequest, config *Config) error {
	r := m.segwriterRoute(true)
	if !config.AsyncIngest {
		return m.send(r)(ctx, req)
	}
	r.client = m.detachedClient(ctx, req, r.client, config)
	m.sendAsync(ctx, req, r)
	return nil
}

func (m *Router) sendToIngesterOnly(ctx context.Context, req *distributormodel.PushRequest) error {
	// NOTE(kolesnikovae): If we also want to support async requests to ingesters,
	// we should implement it here and in sendToBoth.
	return m.send(m.ingesterRoute())(ctx, req)
}

type sendFunc func(context.Context, *distributormodel.PushRequest) error

type route struct {
	path    WritePath // IngesterPath | SegmentWriterPath
	client  IngesterClient
	primary bool
}

// detachedClient creates a new IngesterFunc that wraps the call with a local context
// that has a timeout and tenant ID injected so it can be used for asynchronous requests.
func (m *Router) detachedClient(ctx context.Context, req *distributormodel.PushRequest, client IngesterClient, config *Config) IngesterFunc {
	return func(context.Context, *distributormodel.PushRequest) (*connect.Response[pushv1.PushResponse], error) {
		localCtx, cancel := context.WithTimeout(context.Background(), config.SegmentWriterTimeout)
		localCtx = tenant.InjectTenantID(localCtx, req.TenantID)
		if sp := opentracing.SpanFromContext(ctx); sp != nil {
			localCtx = opentracing.ContextWithSpan(localCtx, sp)
		}
		defer cancel()
		return client.Push(localCtx, req)
	}
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
