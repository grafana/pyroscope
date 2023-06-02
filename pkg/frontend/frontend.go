// SPDX-License-Identifier: AGPL-3.0-only
// Provenance-includes-location: https://github.com/cortexproject/cortex/blob/master/pkg/frontend/v2/frontend.go
// Provenance-includes-license: Apache-2.0
// Provenance-includes-copyright: The Cortex Authors.

package frontend

import (
	"context"
	"flag"
	"fmt"
	"math/rand"
	"net/http"
	"sync"
	"time"

	"github.com/bufbuild/connect-go"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/grafana/dskit/flagext"
	"github.com/grafana/dskit/grpcclient"
	"github.com/grafana/dskit/netutil"
	"github.com/grafana/dskit/services"
	"github.com/opentracing/opentracing-go"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"go.uber.org/atomic"

	"github.com/grafana/dskit/tenant"

	"github.com/grafana/phlare/pkg/frontend/frontendpb"
	"github.com/grafana/phlare/pkg/querier/stats"
	"github.com/grafana/phlare/pkg/scheduler/schedulerdiscovery"
	"github.com/grafana/phlare/pkg/util/httpgrpc"
	"github.com/grafana/phlare/pkg/util/httpgrpcutil"
)

// Config for a Frontend.
type Config struct {
	SchedulerAddress  string            `yaml:"scheduler_address" doc:"hidden"`
	DNSLookupPeriod   time.Duration     `yaml:"scheduler_dns_lookup_period" category:"advanced" doc:"hidden"`
	WorkerConcurrency int               `yaml:"scheduler_worker_concurrency" category:"advanced"`
	GRPCClientConfig  grpcclient.Config `yaml:"grpc_client_config" doc:"description=Configures the gRPC client used to communicate between the query-frontends and the query-schedulers."`

	// Used to find local IP address, that is sent to scheduler and querier-worker.
	InfNames []string `yaml:"instance_interface_names" category:"advanced" doc:"default=[<private network interfaces>]"`

	// If set, address is not computed from interfaces.
	Addr string `yaml:"address" category:"advanced"`
	Port int    `yaml:"-"`

	// This configuration is injected internally.
	QuerySchedulerDiscovery schedulerdiscovery.Config `yaml:"-"`
	MaxLoopDuration         time.Duration             `yaml:"-"`
}

func (cfg *Config) RegisterFlags(f *flag.FlagSet, logger log.Logger) {
	f.IntVar(&cfg.WorkerConcurrency, "query-frontend.scheduler-worker-concurrency", 5, "Number of concurrent workers forwarding queries to single query-scheduler.")

	cfg.InfNames = netutil.PrivateNetworkInterfacesWithFallback([]string{"eth0", "en0"}, logger)
	f.Var((*flagext.StringSlice)(&cfg.InfNames), "query-frontend.instance-interface-names", "List of network interface names to look up when finding the instance IP address. This address is sent to query-scheduler and querier, which uses it to send the query response back to query-frontend.")
	f.StringVar(&cfg.Addr, "query-frontend.instance-addr", "", "IP address to advertise to the querier (via scheduler) (default is auto-detected from network interfaces).")

	cfg.GRPCClientConfig.RegisterFlagsWithPrefix("query-frontend.grpc-client-config", f)
}

func (cfg *Config) Validate(log log.Logger) error {
	if cfg.QuerySchedulerDiscovery.Mode == schedulerdiscovery.ModeRing && cfg.SchedulerAddress != "" {
		return fmt.Errorf("scheduler address cannot be specified when query-scheduler service discovery mode is set to '%s'", cfg.QuerySchedulerDiscovery.Mode)
	}

	return cfg.GRPCClientConfig.Validate(log)
}

// Frontend implements GrpcRoundTripper. It queues HTTP requests,
// dispatches them to backends via gRPC, and handles retries for requests which failed.
type Frontend struct {
	services.Service

	cfg Config
	log log.Logger

	lastQueryID atomic.Uint64

	// frontend workers will read from this channel, and send request to scheduler.
	requestsCh chan *frontendRequest

	limits                  Limits
	schedulerWorkers        *frontendSchedulerWorkers
	schedulerWorkersWatcher *services.FailureWatcher
	requests                *requestsInProgress
	frontendpb.UnimplementedFrontendForQuerierServer
}

type Limits interface {
	QuerySplitDuration(string) time.Duration
	MaxQueryParallelism(string) int
	MaxQueryLength(tenantID string) time.Duration
	MaxQueryLookback(tenantID string) time.Duration
}

type frontendRequest struct {
	queryID      uint64
	request      *httpgrpc.HTTPRequest
	userID       string
	statsEnabled bool

	cancel context.CancelFunc

	enqueue  chan enqueueResult
	response chan *frontendpb.QueryResultRequest
}

type enqueueStatus int

const (
	// Sent to scheduler successfully, and frontend should wait for response now.
	waitForResponse enqueueStatus = iota

	// Failed to forward request to scheduler, frontend will try again.
	failed
)

type enqueueResult struct {
	status enqueueStatus

	cancelCh chan<- uint64 // Channel that can be used for request cancellation. If nil, cancellation is not possible.
}

// NewFrontend creates a new frontend.
func NewFrontend(cfg Config, limits Limits, log log.Logger, reg prometheus.Registerer) (*Frontend, error) {
	requestsCh := make(chan *frontendRequest)

	schedulerWorkers, err := newFrontendSchedulerWorkers(cfg, fmt.Sprintf("%s:%d", cfg.Addr, cfg.Port), requestsCh, log, reg)
	if err != nil {
		return nil, err
	}

	f := &Frontend{
		cfg:                     cfg,
		log:                     log,
		limits:                  limits,
		requestsCh:              requestsCh,
		schedulerWorkers:        schedulerWorkers,
		schedulerWorkersWatcher: services.NewFailureWatcher(),
		requests:                newRequestsInProgress(),
	}
	// Randomize to avoid getting responses from queries sent before restart, which could lead to mixing results
	// between different queries. Note that frontend verifies the user, so it cannot leak results between tenants.
	// This isn't perfect, but better than nothing.
	f.lastQueryID.Store(rand.Uint64())

	promauto.With(reg).NewGaugeFunc(prometheus.GaugeOpts{
		Name: "pyroscope_query_frontend_queries_in_progress",
		Help: "Number of queries in progress handled by this frontend.",
	}, func() float64 {
		return float64(f.requests.count())
	})

	promauto.With(reg).NewGaugeFunc(prometheus.GaugeOpts{
		Name: "pyroscope_query_frontend_connected_schedulers",
		Help: "Number of schedulers this frontend is connected to.",
	}, func() float64 {
		return float64(f.schedulerWorkers.getWorkersCount())
	})

	f.Service = services.NewBasicService(f.starting, f.running, f.stopping)
	return f, nil
}

func (f *Frontend) starting(ctx context.Context) error {
	f.schedulerWorkersWatcher.WatchService(f.schedulerWorkers)

	return errors.Wrap(services.StartAndAwaitRunning(ctx, f.schedulerWorkers), "failed to start frontend scheduler workers")
}

func (f *Frontend) running(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return nil
	case err := <-f.schedulerWorkersWatcher.Chan():
		return errors.Wrap(err, "query-frontend subservice failed")
	}
}

func (f *Frontend) stopping(_ error) error {
	return errors.Wrap(services.StopAndAwaitTerminated(context.Background(), f.schedulerWorkers), "failed to stop frontend scheduler workers")
}

// RoundTripGRPC round trips a proto (instead of an HTTP request).
func (f *Frontend) RoundTripGRPC(ctx context.Context, req *httpgrpc.HTTPRequest) (*httpgrpc.HTTPResponse, error) {
	if s := f.State(); s != services.Running {
		return nil, fmt.Errorf("frontend not running: %v", s)
	}

	tenantIDs, err := tenant.TenantIDs(ctx)
	if err != nil {
		return nil, err
	}
	userID := tenant.JoinTenantIDs(tenantIDs)

	// Propagate trace context in gRPC too - this will be ignored if using HTTP.
	tracer, span := opentracing.GlobalTracer(), opentracing.SpanFromContext(ctx)
	if tracer != nil && span != nil {
		carrier := (*httpgrpcutil.HttpgrpcHeadersCarrier)(req)
		if err := tracer.Inject(span.Context(), opentracing.HTTPHeaders, carrier); err != nil {
			return nil, err
		}
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	freq := &frontendRequest{
		queryID:      f.lastQueryID.Inc(),
		request:      req,
		userID:       userID,
		statsEnabled: stats.IsEnabled(ctx),

		cancel: cancel,

		// Buffer of 1 to ensure response or error can be written to the channel
		// even if this goroutine goes away due to client context cancellation.
		enqueue:  make(chan enqueueResult, 1),
		response: make(chan *frontendpb.QueryResultRequest, 1),
	}

	f.requests.put(freq)
	defer f.requests.delete(freq.queryID)

	retries := f.cfg.WorkerConcurrency + 1 // To make sure we hit at least two different schedulers.

enqueueAgain:
	var cancelCh chan<- uint64
	select {
	case <-ctx.Done():
		return nil, ctx.Err()

	case f.requestsCh <- freq:
		// Enqueued, let's wait for response.
		enqRes := <-freq.enqueue
		if enqRes.status == waitForResponse {
			cancelCh = enqRes.cancelCh
			break // go wait for response.
		} else if enqRes.status == failed {
			retries--
			if retries > 0 {
				goto enqueueAgain
			}
		}

		return nil, httpgrpc.Errorf(http.StatusInternalServerError, "failed to enqueue request")
	}

	select {
	case <-ctx.Done():
		if cancelCh != nil {
			select {
			case cancelCh <- freq.queryID:
				// cancellation sent.
			default:
				// failed to cancel, ignore.
				level.Warn(f.log).Log("msg", "failed to send cancellation request to scheduler, queue full")
			}
		}
		return nil, ctx.Err()

	case resp := <-freq.response:
		if stats.ShouldTrackHTTPGRPCResponse(resp.HttpResponse) {
			stats.FromContext(ctx).Merge(resp.Stats) // Safe if stats is nil.
		}

		return resp.HttpResponse, nil
	}
}

func (f *Frontend) QueryResult(ctx context.Context, r *connect.Request[frontendpb.QueryResultRequest]) (*connect.Response[frontendpb.QueryResultResponse], error) {
	qrReq := r.Msg
	tenantIDs, err := tenant.TenantIDs(ctx)
	if err != nil {
		return nil, err
	}
	userID := tenant.JoinTenantIDs(tenantIDs)

	req := f.requests.get(qrReq.QueryID)
	// It is possible that some old response belonging to different user was received, if frontend has restarted.
	// To avoid leaking query results between users, we verify the user here.
	// To avoid mixing results from different queries, we randomize queryID counter on start.
	if req != nil && req.userID == userID {
		select {
		case req.response <- qrReq:
			// Should always be possible, unless QueryResult is called multiple times with the same queryID.
		default:
			level.Warn(f.log).Log("msg", "failed to write query result to the response channel", "queryID", qrReq.QueryID, "user", userID)
		}
	}

	return connect.NewResponse(&frontendpb.QueryResultResponse{}), nil
}

// CheckReady determines if the query frontend is ready.  Function parameters/return
// chosen to match the same method in the ingester
func (f *Frontend) CheckReady(_ context.Context) error {
	workers := f.schedulerWorkers.getWorkersCount()

	// If frontend is connected to at least one scheduler, we are ready.
	if workers > 0 {
		return nil
	}

	msg := fmt.Sprintf("not ready: number of schedulers this worker is connected to is %d", workers)
	level.Info(f.log).Log("msg", msg)
	return errors.New(msg)
}

type requestsInProgress struct {
	mu       sync.Mutex
	requests map[uint64]*frontendRequest
}

func newRequestsInProgress() *requestsInProgress {
	return &requestsInProgress{
		requests: map[uint64]*frontendRequest{},
	}
}

func (r *requestsInProgress) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()

	return len(r.requests)
}

func (r *requestsInProgress) put(req *frontendRequest) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.requests[req.queryID] = req
}

func (r *requestsInProgress) delete(queryID uint64) {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.requests, queryID)
}

func (r *requestsInProgress) get(queryID uint64) *frontendRequest {
	r.mu.Lock()
	defer r.mu.Unlock()

	return r.requests[queryID]
}
