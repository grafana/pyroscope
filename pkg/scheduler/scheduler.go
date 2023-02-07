// SPDX-License-Identifier: AGPL-3.0-only
// Provenance-includes-location: https://github.com/cortexproject/cortex/blob/master/pkg/scheduler/scheduler.go
// Provenance-includes-license: Apache-2.0
// Provenance-includes-copyright: The Cortex Authors.

package scheduler

import (
	"context"
	"flag"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/bufbuild/connect-go"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/grafana/dskit/grpcclient"
	"github.com/grafana/dskit/ring"
	"github.com/grafana/dskit/services"
	"github.com/grafana/dskit/tenant"
	otgrpc "github.com/opentracing-contrib/go-grpc"
	"github.com/opentracing/opentracing-go"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/weaveworks/common/middleware"
	"github.com/weaveworks/common/user"
	"google.golang.org/grpc"

	"github.com/grafana/phlare/pkg/frontend/frontendpb"
	"github.com/grafana/phlare/pkg/scheduler/queue"
	"github.com/grafana/phlare/pkg/scheduler/schedulerdiscovery"
	"github.com/grafana/phlare/pkg/scheduler/schedulerpb"
	"github.com/grafana/phlare/pkg/util"
	"github.com/grafana/phlare/pkg/util/httpgrpc"
	"github.com/grafana/phlare/pkg/util/httpgrpcutil"
	"github.com/grafana/phlare/pkg/util/validation"
)

// Scheduler is responsible for queueing and dispatching queries to Queriers.
type Scheduler struct {
	services.Service

	cfg Config
	log log.Logger

	limits Limits

	connectedFrontendsMu sync.Mutex
	connectedFrontends   map[string]*connectedFrontend

	requestQueue *queue.RequestQueue
	activeUsers  *util.ActiveUsersCleanupService

	pendingRequestsMu sync.Mutex
	pendingRequests   map[requestKey]*schedulerRequest // Request is kept in this map even after being dispatched to querier. It can still be canceled at that time.

	// The ring is used to let other components discover query-scheduler replicas.
	// The ring is optional.
	schedulerLifecycler *ring.BasicLifecycler

	// Subservices manager.
	subservices        *services.Manager
	subservicesWatcher *services.FailureWatcher

	// Metrics.
	queueLength              *prometheus.GaugeVec
	discardedRequests        *prometheus.CounterVec
	cancelledRequests        *prometheus.CounterVec
	connectedQuerierClients  prometheus.GaugeFunc
	connectedFrontendClients prometheus.GaugeFunc
	queueDuration            prometheus.Histogram
	inflightRequests         prometheus.Summary

	schedulerpb.UnimplementedSchedulerForFrontendServer
	schedulerpb.UnimplementedSchedulerForQuerierServer
}

type requestKey struct {
	frontendAddr string
	queryID      uint64
}

type connectedFrontend struct {
	connections int

	// This context is used for running all queries from the same frontend.
	// When last frontend connection is closed, context is canceled.
	ctx    context.Context
	cancel context.CancelFunc
}

type Config struct {
	MaxOutstandingPerTenant int                       `yaml:"max_outstanding_requests_per_tenant"`
	QuerierForgetDelay      time.Duration             `yaml:"querier_forget_delay" category:"experimental"`
	GRPCClientConfig        grpcclient.Config         `yaml:"grpc_client_config" doc:"description=This configures the gRPC client used to report errors back to the query-frontend."`
	ServiceDiscovery        schedulerdiscovery.Config `yaml:",inline"`
}

func (cfg *Config) RegisterFlags(f *flag.FlagSet, logger log.Logger) {
	f.IntVar(&cfg.MaxOutstandingPerTenant, "query-scheduler.max-outstanding-requests-per-tenant", 100, "Maximum number of outstanding requests per tenant per query-scheduler. In-flight requests above this limit will fail with HTTP response status code 429.")
	f.DurationVar(&cfg.QuerierForgetDelay, "query-scheduler.querier-forget-delay", 0, "If a querier disconnects without sending notification about graceful shutdown, the query-scheduler will keep the querier in the tenant's shard until the forget delay has passed. This feature is useful to reduce the blast radius when shuffle-sharding is enabled.")
	cfg.GRPCClientConfig.RegisterFlagsWithPrefix("query-scheduler.grpc-client-config", f)
	cfg.ServiceDiscovery.RegisterFlags(f, logger)
}

func (cfg *Config) Validate() error {
	return cfg.ServiceDiscovery.Validate()
}

// NewScheduler creates a new Scheduler.
func NewScheduler(cfg Config, limits Limits, log log.Logger, registerer prometheus.Registerer) (*Scheduler, error) {
	var err error

	s := &Scheduler{
		cfg:    cfg,
		log:    log,
		limits: limits,

		pendingRequests:    map[requestKey]*schedulerRequest{},
		connectedFrontends: map[string]*connectedFrontend{},
		subservicesWatcher: services.NewFailureWatcher(),
	}

	s.queueLength = promauto.With(registerer).NewGaugeVec(prometheus.GaugeOpts{
		Name: "cortex_query_scheduler_queue_length",
		Help: "Number of queries in the queue.",
	}, []string{"user"})

	s.cancelledRequests = promauto.With(registerer).NewCounterVec(prometheus.CounterOpts{
		Name: "cortex_query_scheduler_cancelled_requests_total",
		Help: "Total number of query requests that were cancelled after enqueuing.",
	}, []string{"user"})
	s.discardedRequests = promauto.With(registerer).NewCounterVec(prometheus.CounterOpts{
		Name: "cortex_query_scheduler_discarded_requests_total",
		Help: "Total number of query requests discarded.",
	}, []string{"user"})
	s.requestQueue = queue.NewRequestQueue(cfg.MaxOutstandingPerTenant, cfg.QuerierForgetDelay, s.queueLength, s.discardedRequests)

	s.queueDuration = promauto.With(registerer).NewHistogram(prometheus.HistogramOpts{
		Name:    "cortex_query_scheduler_queue_duration_seconds",
		Help:    "Time spend by requests in queue before getting picked up by a querier.",
		Buckets: prometheus.DefBuckets,
	})
	s.connectedQuerierClients = promauto.With(registerer).NewGaugeFunc(prometheus.GaugeOpts{
		Name: "cortex_query_scheduler_connected_querier_clients",
		Help: "Number of querier worker clients currently connected to the query-scheduler.",
	}, s.requestQueue.GetConnectedQuerierWorkersMetric)
	s.connectedFrontendClients = promauto.With(registerer).NewGaugeFunc(prometheus.GaugeOpts{
		Name: "cortex_query_scheduler_connected_frontend_clients",
		Help: "Number of query-frontend worker clients currently connected to the query-scheduler.",
	}, s.getConnectedFrontendClientsMetric)

	s.inflightRequests = promauto.With(registerer).NewSummary(prometheus.SummaryOpts{
		Name:       "cortex_query_scheduler_inflight_requests",
		Help:       "Number of inflight requests (either queued or processing) sampled at a regular interval. Quantile buckets keep track of inflight requests over the last 60s.",
		Objectives: map[float64]float64{0.5: 0.05, 0.75: 0.02, 0.8: 0.02, 0.9: 0.01, 0.95: 0.01, 0.99: 0.001},
		MaxAge:     time.Minute,
		AgeBuckets: 6,
	})

	s.activeUsers = util.NewActiveUsersCleanupWithDefaultValues(s.cleanupMetricsForInactiveUser)
	subservices := []services.Service{s.requestQueue, s.activeUsers}

	// Init the ring only if the ring-based service discovery mode is used.
	if cfg.ServiceDiscovery.Mode == schedulerdiscovery.ModeRing {
		s.schedulerLifecycler, err = schedulerdiscovery.NewRingLifecycler(cfg.ServiceDiscovery.SchedulerRing, log, registerer)
		if err != nil {
			return nil, err
		}

		subservices = append(subservices, s.schedulerLifecycler)
	}

	s.subservices, err = services.NewManager(subservices...)
	if err != nil {
		return nil, err
	}

	s.Service = services.NewBasicService(s.starting, s.running, s.stopping)
	return s, nil
}

// Limits needed for the Query Scheduler - interface used for decoupling.
type Limits interface {
	// MaxQueriersPerUser returns max queriers to use per tenant, or 0 if shuffle sharding is disabled.
	MaxQueriersPerUser(user string) int
}

type schedulerRequest struct {
	frontendAddress string
	userID          string
	queryID         uint64
	request         *httpgrpc.HTTPRequest
	statsEnabled    bool

	enqueueTime time.Time

	ctx       context.Context
	ctxCancel context.CancelFunc
	queueSpan opentracing.Span

	// This is only used for testing.
	parentSpanContext opentracing.SpanContext
}

// FrontendLoop handles connection from frontend.
func (s *Scheduler) FrontendLoop(ctx context.Context, frontend *connect.BidiStream[schedulerpb.FrontendToScheduler, schedulerpb.SchedulerToFrontend]) error {
	frontendAddress, frontendCtx, err := s.frontendConnected(frontend)
	if err != nil {
		return err
	}
	defer s.frontendDisconnected(frontendAddress)

	// Response to INIT. If scheduler is not running, we skip for-loop, send SHUTTING_DOWN and exit this method.
	if s.State() == services.Running {
		if err := frontend.Send(&schedulerpb.SchedulerToFrontend{Status: schedulerpb.SchedulerToFrontendStatus_OK}); err != nil {
			return err
		}
	}

	// We stop accepting new queries in Stopping state. By returning quickly, we disconnect frontends, which in turns
	// cancels all their queries.
	for s.State() == services.Running {
		msg, err := frontend.Receive()
		if err != nil {
			// No need to report this as error, it is expected when query-frontend performs SendClose() (as frontendSchedulerWorker does).
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}

		if s.State() != services.Running {
			break // break out of the loop, and send SHUTTING_DOWN message.
		}

		var resp *schedulerpb.SchedulerToFrontend

		switch msg.GetType() {
		case schedulerpb.FrontendToSchedulerType_ENQUEUE:
			err = s.enqueueRequest(frontendCtx, frontendAddress, msg)
			switch {
			case err == nil:
				resp = &schedulerpb.SchedulerToFrontend{Status: schedulerpb.SchedulerToFrontendStatus_OK}
			case errors.Is(err, queue.ErrTooManyRequests):
				resp = &schedulerpb.SchedulerToFrontend{Status: schedulerpb.SchedulerToFrontendStatus_TOO_MANY_REQUESTS_PER_TENANT}
			default:
				resp = &schedulerpb.SchedulerToFrontend{Status: schedulerpb.SchedulerToFrontendStatus_ERROR, Error: err.Error()}
			}

		case schedulerpb.FrontendToSchedulerType_CANCEL:
			s.cancelRequestAndRemoveFromPending(frontendAddress, msg.QueryID)
			resp = &schedulerpb.SchedulerToFrontend{Status: schedulerpb.SchedulerToFrontendStatus_OK}

		default:
			level.Error(s.log).Log("msg", "unknown request type from frontend", "addr", frontendAddress, "type", msg.GetType())
			return errors.New("unknown request type")
		}

		err = frontend.Send(resp)
		// Failure to send response results in ending this connection.
		if err != nil {
			return err
		}
	}

	// Report shutdown back to frontend, so that it can retry with different scheduler. Also stop the frontend loop.
	return frontend.Send(&schedulerpb.SchedulerToFrontend{Status: schedulerpb.SchedulerToFrontendStatus_SHUTTING_DOWN})
}

func (s *Scheduler) frontendConnected(frontend *connect.BidiStream[schedulerpb.FrontendToScheduler, schedulerpb.SchedulerToFrontend]) (string, context.Context, error) {
	msg, err := frontend.Receive()
	if err != nil {
		return "", nil, err
	}
	if msg.Type != schedulerpb.FrontendToSchedulerType_INIT || msg.FrontendAddress == "" {
		return "", nil, errors.New("no frontend address")
	}

	s.connectedFrontendsMu.Lock()
	defer s.connectedFrontendsMu.Unlock()

	cf := s.connectedFrontends[msg.FrontendAddress]
	if cf == nil {
		cf = &connectedFrontend{
			connections: 0,
		}
		cf.ctx, cf.cancel = context.WithCancel(context.Background())
		s.connectedFrontends[msg.FrontendAddress] = cf
	}

	cf.connections++
	return msg.FrontendAddress, cf.ctx, nil
}

func (s *Scheduler) frontendDisconnected(frontendAddress string) {
	s.connectedFrontendsMu.Lock()
	defer s.connectedFrontendsMu.Unlock()

	cf := s.connectedFrontends[frontendAddress]
	cf.connections--
	if cf.connections == 0 {
		delete(s.connectedFrontends, frontendAddress)
		cf.cancel()
	}
}

func (s *Scheduler) enqueueRequest(frontendContext context.Context, frontendAddr string, msg *schedulerpb.FrontendToScheduler) error {
	// Create new context for this request, to support cancellation.
	ctx, cancel := context.WithCancel(frontendContext)
	shouldCancel := true
	defer func() {
		if shouldCancel {
			cancel()
		}
	}()

	// Extract tracing information from headers in HTTP request. FrontendContext doesn't have the correct tracing
	// information, since that is a long-running request.
	tracer := opentracing.GlobalTracer()
	parentSpanContext, err := httpgrpcutil.GetParentSpanForRequest(tracer, msg.HttpRequest)
	if err != nil {
		return err
	}

	userID := msg.GetUserID()

	req := &schedulerRequest{
		frontendAddress: frontendAddr,
		userID:          msg.UserID,
		queryID:         msg.QueryID,
		request:         msg.HttpRequest,
		statsEnabled:    msg.StatsEnabled,
	}

	now := time.Now()

	req.parentSpanContext = parentSpanContext
	req.queueSpan, req.ctx = opentracing.StartSpanFromContextWithTracer(ctx, tracer, "queued", opentracing.ChildOf(parentSpanContext))
	req.enqueueTime = now
	req.ctxCancel = cancel

	// aggregate the max queriers limit in the case of a multi tenant query
	tenantIDs, err := tenant.TenantIDsFromOrgID(userID)
	if err != nil {
		return err
	}
	maxQueriers := validation.SmallestPositiveNonZeroIntPerTenant(tenantIDs, s.limits.MaxQueriersPerUser)

	s.activeUsers.UpdateUserTimestamp(userID, now)
	return s.requestQueue.EnqueueRequest(userID, req, maxQueriers, func() {
		shouldCancel = false

		s.pendingRequestsMu.Lock()
		s.pendingRequests[requestKey{frontendAddr: frontendAddr, queryID: msg.QueryID}] = req
		s.pendingRequestsMu.Unlock()
	})
}

// This method doesn't do removal from the queue.
func (s *Scheduler) cancelRequestAndRemoveFromPending(frontendAddr string, queryID uint64) {
	s.pendingRequestsMu.Lock()
	defer s.pendingRequestsMu.Unlock()

	key := requestKey{frontendAddr: frontendAddr, queryID: queryID}
	req := s.pendingRequests[key]
	if req != nil {
		req.ctxCancel()
	}

	delete(s.pendingRequests, key)
}

// BidiStreamCloser is a wrapper around BidiStream that allows to close it.
// Once closed, it will return io.EOF on Receive and Send.
type BidiStreamCloser[Req, Res any] struct {
	stream *connect.BidiStream[Req, Res]
	lock   sync.Mutex
}

func (c *BidiStreamCloser[Req, Res]) Close() {
	c.lock.Lock()
	defer c.lock.Unlock()

	if c.stream != nil {
		c.stream = nil
	}
}

func (c *BidiStreamCloser[Req, Res]) Receive() (*Req, error) {
	c.lock.Lock()
	defer c.lock.Unlock()

	if c.stream == nil {
		return nil, io.EOF
	}

	return c.stream.Receive()
}

func (b *BidiStreamCloser[Req, Res]) Send(msg *Res) error {
	b.lock.Lock()
	defer b.lock.Unlock()

	if b.stream == nil {
		return io.EOF
	}
	return b.stream.Send(msg)
}

// QuerierLoop is started by querier to receive queries from scheduler.
func (s *Scheduler) QuerierLoop(ctx context.Context, bidi *connect.BidiStream[schedulerpb.QuerierToScheduler, schedulerpb.SchedulerToQuerier]) error {
	querier := &BidiStreamCloser[schedulerpb.QuerierToScheduler, schedulerpb.SchedulerToQuerier]{
		stream: bidi,
	}
	defer querier.Close()
	resp, err := querier.Receive()
	if err != nil {
		return err
	}

	querierID := resp.GetQuerierID()

	s.requestQueue.RegisterQuerierConnection(querierID)
	defer s.requestQueue.UnregisterQuerierConnection(querierID)

	lastUserIndex := queue.FirstUser()

	// In stopping state scheduler is not accepting new queries, but still dispatching queries in the queues.
	for s.isRunningOrStopping() {
		req, idx, err := s.requestQueue.GetNextRequestForQuerier(ctx, lastUserIndex, querierID)
		if err != nil {
			// Return a more clear error if the queue is stopped because the query-scheduler is not running.
			if errors.Is(err, queue.ErrStopped) && !s.isRunning() {
				return schedulerpb.ErrSchedulerIsNotRunning
			}

			return err
		}
		lastUserIndex = idx

		r := req.(*schedulerRequest)

		s.queueDuration.Observe(time.Since(r.enqueueTime).Seconds())
		r.queueSpan.Finish()

		/*
		  We want to dequeue the next unexpired request from the chosen tenant queue.
		  The chance of choosing a particular tenant for dequeueing is (1/active_tenants).
		  This is problematic under load, especially with other middleware enabled such as
		  querier.split-by-interval, where one request may fan out into many.
		  If expired requests aren't exhausted before checking another tenant, it would take
		  n_active_tenants * n_expired_requests_at_front_of_queue requests being processed
		  before an active request was handled for the tenant in question.
		  If this tenant meanwhile continued to queue requests,
		  it's possible that it's own queue would perpetually contain only expired requests.
		*/

		if r.ctx.Err() != nil {
			// Remove from pending requests.
			s.cancelRequestAndRemoveFromPending(r.frontendAddress, r.queryID)

			lastUserIndex = lastUserIndex.ReuseLastUser()
			continue
		}

		if err := s.forwardRequestToQuerier(querier, r); err != nil {
			return err
		}
	}

	return schedulerpb.ErrSchedulerIsNotRunning
}

func (s *Scheduler) NotifyQuerierShutdown(ctx context.Context, req *connect.Request[schedulerpb.NotifyQuerierShutdownRequest]) (*connect.Response[schedulerpb.NotifyQuerierShutdownResponse], error) {
	level.Info(s.log).Log("msg", "received shutdown notification from querier", "querier", req.Msg.GetQuerierID())
	s.requestQueue.NotifyQuerierShutdown(req.Msg.GetQuerierID())

	return connect.NewResponse(&schedulerpb.NotifyQuerierShutdownResponse{}), nil
}

func (s *Scheduler) forwardRequestToQuerier(querier *BidiStreamCloser[schedulerpb.QuerierToScheduler, schedulerpb.SchedulerToQuerier], req *schedulerRequest) error {
	// Make sure to cancel request at the end to cleanup resources.
	defer s.cancelRequestAndRemoveFromPending(req.frontendAddress, req.queryID)

	// Handle the stream sending & receiving on a goroutine so we can
	// monitoring the contexts in a select and cancel things appropriately.
	errCh := make(chan error, 1)
	go func() {
		err := querier.Send(&schedulerpb.SchedulerToQuerier{
			UserID:          req.userID,
			QueryID:         req.queryID,
			FrontendAddress: req.frontendAddress,
			HttpRequest:     req.request,
			StatsEnabled:    req.statsEnabled,
		})
		if err != nil {
			errCh <- err
			return
		}

		_, err = querier.Receive()
		errCh <- err
	}()

	select {
	case <-req.ctx.Done():
		// If the upstream request is cancelled (eg. frontend issued CANCEL or closed connection),
		// we need to cancel the downstream req. Only way we can do that is to close the stream (by returning error here).
		// Querier is expecting this semantics.
		s.cancelledRequests.WithLabelValues(req.userID).Inc()
		return req.ctx.Err()

	case err := <-errCh:
		// Is there was an error handling this request due to network IO,
		// then error out this upstream request _and_ stream.

		if err != nil {
			s.forwardErrorToFrontend(req.ctx, req, err)
		}
		return err
	}
}

func (s *Scheduler) forwardErrorToFrontend(ctx context.Context, req *schedulerRequest, requestErr error) {
	opts, err := s.cfg.GRPCClientConfig.DialOption([]grpc.UnaryClientInterceptor{
		otgrpc.OpenTracingClientInterceptor(opentracing.GlobalTracer()),
		middleware.ClientUserHeaderInterceptor,
	},
		nil)
	if err != nil {
		level.Warn(s.log).Log("msg", "failed to create gRPC options for the connection to frontend to report error", "frontend", req.frontendAddress, "err", err, "requestErr", requestErr)
		return
	}

	conn, err := grpc.DialContext(ctx, req.frontendAddress, opts...)
	if err != nil {
		level.Warn(s.log).Log("msg", "failed to create gRPC connection to frontend to report error", "frontend", req.frontendAddress, "err", err, "requestErr", requestErr)
		return
	}

	defer func() {
		_ = conn.Close()
	}()

	client := frontendpb.NewFrontendForQuerierClient(conn)

	userCtx := user.InjectOrgID(ctx, req.userID)
	_, err = client.QueryResult(userCtx, &frontendpb.QueryResultRequest{
		QueryID: req.queryID,
		HttpResponse: &httpgrpc.HTTPResponse{
			Code: http.StatusInternalServerError,
			Body: []byte(requestErr.Error()),
		},
	})

	if err != nil {
		level.Warn(s.log).Log("msg", "failed to forward error to frontend", "frontend", req.frontendAddress, "err", err, "requestErr", requestErr)
		return
	}
}

func (s *Scheduler) isRunning() bool {
	st := s.State()
	return st == services.Running
}

func (s *Scheduler) isRunningOrStopping() bool {
	st := s.State()
	return st == services.Running || st == services.Stopping
}

func (s *Scheduler) starting(ctx context.Context) error {
	s.subservicesWatcher.WatchManager(s.subservices)

	if err := services.StartManagerAndAwaitHealthy(ctx, s.subservices); err != nil {
		return errors.Wrap(err, "unable to start scheduler subservices")
	}

	return nil
}

func (s *Scheduler) running(ctx context.Context) error {
	// We observe inflight requests frequently and at regular intervals, to have a good
	// approximation of max inflight requests over percentiles of time. We also do it with
	// a ticker so that we keep tracking it even if we have no new queries but stuck inflight
	// requests (e.g. queriers are all crashing).
	inflightRequestsTicker := time.NewTicker(250 * time.Millisecond)
	defer inflightRequestsTicker.Stop()

	for {
		select {
		case <-inflightRequestsTicker.C:
			s.pendingRequestsMu.Lock()
			inflight := len(s.pendingRequests)
			s.pendingRequestsMu.Unlock()

			s.inflightRequests.Observe(float64(inflight))
		case <-ctx.Done():
			return nil
		case err := <-s.subservicesWatcher.Chan():
			return errors.Wrap(err, "scheduler subservice failed")
		}
	}
}

// Close the Scheduler.
func (s *Scheduler) stopping(_ error) error {
	// This will also stop the requests queue, which stop accepting new requests and errors out any pending requests.
	return services.StopManagerAndAwaitStopped(context.Background(), s.subservices)
}

func (s *Scheduler) cleanupMetricsForInactiveUser(user string) {
	s.queueLength.DeleteLabelValues(user)
	s.discardedRequests.DeleteLabelValues(user)
	s.cancelledRequests.DeleteLabelValues(user)
}

func (s *Scheduler) getConnectedFrontendClientsMetric() float64 {
	s.connectedFrontendsMu.Lock()
	defer s.connectedFrontendsMu.Unlock()

	count := 0
	for _, workers := range s.connectedFrontends {
		count += workers.connections
	}

	return float64(count)
}

func (s *Scheduler) RingHandler(w http.ResponseWriter, req *http.Request) {
	if s.schedulerLifecycler != nil {
		s.schedulerLifecycler.ServeHTTP(w, req)
		return
	}

	ringDisabledPage := `
		<!DOCTYPE html>
		<html>
			<head>
				<meta charset="UTF-8">
				<title>Query-scheduler Status</title>
			</head>
			<body>
				<h1>Query-scheduler Status</h1>
				<p>Query-scheduler hash ring is disabled.</p>
			</body>
		</html>`
	util.WriteHTMLResponse(w, ringDisabledPage)
}
