// SPDX-License-Identifier: AGPL-3.0-only
// Provenance-includes-location: https://github.com/cortexproject/cortex/blob/master/pkg/querier/worker/scheduler_processor.go
// Provenance-includes-license: Apache-2.0
// Provenance-includes-copyright: The Cortex Authors.

package worker

import (
	"context"
	"fmt"
	"math/rand"
	"net/http"
	"strings"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/gogo/status"
	"github.com/grafana/dskit/backoff"
	"github.com/grafana/dskit/grpcclient"
	"github.com/grafana/dskit/middleware"
	"github.com/grafana/dskit/ring"
	"github.com/grafana/dskit/ring/client"
	"github.com/grafana/dskit/services"
	"github.com/grafana/dskit/user"
	otgrpc "github.com/opentracing-contrib/go-grpc"
	"github.com/opentracing/opentracing-go"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"go.uber.org/atomic"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health/grpc_health_v1"

	"github.com/grafana/pyroscope/pkg/frontend/frontendpb"
	querier_stats "github.com/grafana/pyroscope/pkg/querier/stats"
	"github.com/grafana/pyroscope/pkg/scheduler/schedulerpb"
	util_log "github.com/grafana/pyroscope/pkg/util"
	"github.com/grafana/pyroscope/pkg/util/httpgrpc"
	"github.com/grafana/pyroscope/pkg/util/httpgrpcutil"
)

var processorBackoffConfig = backoff.Config{
	MinBackoff: 250 * time.Millisecond,
	MaxBackoff: 2 * time.Second,
}

func newSchedulerProcessor(cfg Config, handler RequestHandler, log log.Logger, reg prometheus.Registerer) (*schedulerProcessor, []services.Service) {
	p := &schedulerProcessor{
		log:             log,
		handler:         handler,
		maxMessageSize:  cfg.GRPCClientConfig.MaxSendMsgSize,
		querierID:       cfg.QuerierID,
		grpcConfig:      cfg.GRPCClientConfig,
		maxLoopDuration: cfg.MaxLoopDuration,

		schedulerClientFactory: func(conn *grpc.ClientConn) schedulerpb.SchedulerForQuerierClient {
			return schedulerpb.NewSchedulerForQuerierClient(conn)
		},

		frontendClientRequestDuration: promauto.With(reg).NewHistogramVec(prometheus.HistogramOpts{
			Name:    "pyroscope_querier_query_frontend_request_duration_seconds",
			Help:    "Time spend doing requests to frontend.",
			Buckets: prometheus.ExponentialBuckets(0.001, 4, 6),
		}, []string{"operation", "status_code"}),
	}

	frontendClientsGauge := promauto.With(reg).NewGauge(prometheus.GaugeOpts{
		Name: "pyroscope_querier_query_frontend_clients",
		Help: "The current number of clients connected to query-frontend.",
	})

	poolConfig := client.PoolConfig{
		CheckInterval:      5 * time.Second,
		HealthCheckEnabled: true,
		HealthCheckTimeout: 1 * time.Second,
	}

	p.frontendPool = client.NewPool("frontend", poolConfig, nil, p.frontendClientFactory(), frontendClientsGauge, log)
	return p, []services.Service{p.frontendPool}
}

// Handles incoming queries from query-scheduler.
type schedulerProcessor struct {
	log             log.Logger
	handler         RequestHandler
	grpcConfig      grpcclient.Config
	maxMessageSize  int
	querierID       string
	maxLoopDuration time.Duration

	frontendPool                  *client.Pool
	frontendClientRequestDuration *prometheus.HistogramVec

	schedulerClientFactory func(conn *grpc.ClientConn) schedulerpb.SchedulerForQuerierClient
}

// notifyShutdown implements processor.
func (sp *schedulerProcessor) notifyShutdown(ctx context.Context, conn *grpc.ClientConn, address string) {
	client := sp.schedulerClientFactory(conn)

	req := &schedulerpb.NotifyQuerierShutdownRequest{QuerierID: sp.querierID}
	if _, err := client.NotifyQuerierShutdown(ctx, req); err != nil {
		// Since we're shutting down there's nothing we can do except logging it.
		level.Warn(sp.log).Log("msg", "failed to notify querier shutdown to query-scheduler", "address", address, "err", err)
	}
}

func (sp *schedulerProcessor) processQueriesOnSingleStream(workerCtx context.Context, conn *grpc.ClientConn, address string) {
	schedulerClient := sp.schedulerClientFactory(conn)

	// Run the querier loop (and so all the queries) in a dedicated context that we call the "execution context".
	// The execution context is cancelled once the workerCtx is cancelled AND there's no inflight query executing.
	execCtx, execCancel, inflightQuery := newExecutionContext(workerCtx, sp.log)
	defer execCancel()

	backoff := backoff.New(execCtx, processorBackoffConfig)
	for backoff.Ongoing() {
		func() {
			if err := sp.querierLoop(execCtx, schedulerClient, address, inflightQuery); err != nil {
				// Do not log an error is the query-scheduler is shutting down.
				if s, ok := status.FromError(err); !ok ||
					(!strings.Contains(s.Message(), schedulerpb.ErrSchedulerIsNotRunning.Error()) &&
						!strings.Contains(s.Message(), context.Canceled.Error()) &&
						!strings.Contains(s.Message(), "stream terminated")) {
					level.Error(sp.log).Log("msg", "error processing requests from scheduler", "err", err, "addr", address)
				}
				if strings.Contains(err.Error(), context.Canceled.Error()) || strings.Contains(err.Error(), "stream terminated") {
					backoff.Reset()
					return
				}
				backoff.Wait()
				return
			}

			backoff.Reset()
		}()
	}
}

// process loops processing requests on an established stream.
func (sp *schedulerProcessor) querierLoop(parentCtx context.Context, schedulerClient schedulerpb.SchedulerForQuerierClient, address string, inflightQuery *atomic.Bool) error {
	loopCtx, loopCancel := context.WithCancel(parentCtx)
	defer loopCancel()

	if sp.maxLoopDuration > 0 {
		go func() {
			timer := time.NewTimer(jitter(sp.maxLoopDuration, 0.3))
			defer timer.Stop()

			select {
			case <-timer.C:
				level.Debug(sp.log).Log("msg", "waiting for inflight queries to complete")
				for inflightQuery.Load() {
					select {
					case <-parentCtx.Done():
						// In the meanwhile, the execution context has been explicitly canceled, so we should just terminate.
						return
					default:
						// Wait and check again inflight queries.
						time.Sleep(100 * time.Millisecond)
					}
				}
				level.Debug(sp.log).Log("msg", "refreshing scheduler connection")
				loopCancel()
			case <-parentCtx.Done():
				return
			}
		}()
	}

	c, err := schedulerClient.QuerierLoop(loopCtx)
	if err == nil {
		err = c.Send(&schedulerpb.QuerierToScheduler{QuerierID: sp.querierID})
	}

	if err != nil {
		level.Warn(sp.log).Log("msg", "error contacting scheduler", "err", err, "addr", address)
		return err
	}

	for {
		request, err := c.Recv()
		if err != nil {
			return err
		}

		inflightQuery.Store(true)

		// Handle the request on a "background" goroutine, so we go back to
		// blocking on c.Recv().  This allows us to detect the stream closing
		// and cancel the query.  We don't actually handle queries in parallel
		// here, as we're running in lock step with the server - each Recv is
		// paired with a Send.
		go func() {
			defer inflightQuery.Store(false)

			// We need to inject user into context for sending response back.
			ctx := user.InjectOrgID(c.Context(), request.UserID)

			tracer := opentracing.GlobalTracer()
			// Ignore errors here. If we cannot get parent span, we just don't create new one.
			parentSpanContext, _ := httpgrpcutil.GetParentSpanForRequest(tracer, request.HttpRequest)
			if parentSpanContext != nil {
				queueSpan, spanCtx := opentracing.StartSpanFromContextWithTracer(ctx, tracer, "querier_processor_runRequest", opentracing.ChildOf(parentSpanContext))
				defer queueSpan.Finish()

				ctx = spanCtx
			}
			logger := util_log.LoggerWithContext(ctx, sp.log)

			sp.runRequest(ctx, logger, request.QueryID, request.FrontendAddress, request.StatsEnabled, request.HttpRequest)

			// Report back to scheduler that processing of the query has finished.
			if err := c.Send(&schedulerpb.QuerierToScheduler{}); err != nil {
				level.Error(logger).Log("msg", "error notifying scheduler about finished query", "err", err, "addr", address)
			}
		}()
	}
}

func jitter(d time.Duration, factor float64) time.Duration {
	maxJitter := time.Duration(float64(d) * factor)
	return d - time.Duration(rand.Int63n(int64(maxJitter)))
}

func (sp *schedulerProcessor) runRequest(ctx context.Context, logger log.Logger, queryID uint64, frontendAddress string, statsEnabled bool, request *httpgrpc.HTTPRequest) {
	var stats *querier_stats.Stats
	if statsEnabled {
		stats, ctx = querier_stats.ContextWithEmptyStats(ctx)
	}

	response, err := sp.handler.Handle(ctx, request)
	if err != nil {
		var ok bool
		response, ok = httpgrpc.HTTPResponseFromError(err)
		if !ok {
			response = &httpgrpc.HTTPResponse{
				Code: http.StatusInternalServerError,
				Body: []byte(err.Error()),
			}
		}
	}

	// Ensure responses that are too big are not retried.
	if len(response.Body) >= sp.maxMessageSize {
		level.Error(logger).Log("msg", "response larger than max message size", "size", len(response.Body), "maxMessageSize", sp.maxMessageSize)

		errMsg := fmt.Sprintf("response larger than the max message size (%d vs %d)", len(response.Body), sp.maxMessageSize)
		response = &httpgrpc.HTTPResponse{
			Code: http.StatusRequestEntityTooLarge,
			Body: []byte(errMsg),
		}
	}

	c, err := sp.frontendPool.GetClientFor(frontendAddress)
	if err == nil {
		// Response is empty and uninteresting.
		_, err = c.(frontendpb.FrontendForQuerierClient).QueryResult(ctx, &frontendpb.QueryResultRequest{
			QueryID:      queryID,
			HttpResponse: response,
			Stats:        stats,
		})
	}
	if err != nil {
		level.Error(logger).Log("msg", "error notifying frontend about finished query", "err", err, "frontend", frontendAddress)
	}
}

type frontendClientFactory struct {
	opts func() ([]grpc.DialOption, error)
}

func newFrontendClientFactory(opts func() ([]grpc.DialOption, error)) *frontendClientFactory {
	return &frontendClientFactory{opts: opts}
}

func (f *frontendClientFactory) FromInstance(inst ring.InstanceDesc) (client.PoolClient, error) {
	opts, err := f.opts()
	if err != nil {
		return nil, err
	}

	conn, err := grpc.Dial(inst.Addr, opts...)
	if err != nil {
		return nil, err
	}

	return &frontendClient{
		FrontendForQuerierClient: frontendpb.NewFrontendForQuerierClient(conn),
		HealthClient:             grpc_health_v1.NewHealthClient(conn),
		conn:                     conn,
	}, nil
}

func (sp *schedulerProcessor) frontendClientFactory() client.PoolFactory {
	return newFrontendClientFactory(func() ([]grpc.DialOption, error) {
		return sp.grpcConfig.DialOption([]grpc.UnaryClientInterceptor{
			otgrpc.OpenTracingClientInterceptor(opentracing.GlobalTracer()),
			middleware.ClientUserHeaderInterceptor,
			middleware.UnaryClientInstrumentInterceptor(sp.frontendClientRequestDuration),
		}, nil)
	})
}

type frontendClient struct {
	frontendpb.FrontendForQuerierClient
	grpc_health_v1.HealthClient
	conn *grpc.ClientConn
}

func (fc *frontendClient) Close() error {
	return fc.conn.Close()
}
