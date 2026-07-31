package async

import (
	"context"
	"fmt"
	"sync"
	"time"

	"connectrpc.com/connect"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"google.golang.org/protobuf/proto"

	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
	"github.com/grafana/pyroscope/api/gen/proto/go/querier/v1/querierv1connect"
	"github.com/grafana/pyroscope/v2/pkg/tenant"
)

type Limits interface {
	MaxAsyncQueryConcurrency(tenantID string) int
}

// queryResult is the result of a query execution sent over a channel.
// On success, Response carries the raw SelectMergeStacktracesResponse
// from the wrapped handler (its Async field is unset); the
// coordinator/store own request_id and status.
type queryResult struct {
	Response *querierv1.SelectMergeStacktracesResponse
	Err      error
}

type Coordinator struct {
	logger log.Logger
	store  *Store
	limits Limits
	next   querierv1connect.QuerierServiceHandler

	mu       sync.Mutex
	inFlight map[string]int // tenantID -> count

	asyncQueriesCurrent *prometheus.GaugeVec
	asyncQueriesMax     *prometheus.GaugeVec
}

func NewCoordinator(logger log.Logger, store *Store, limits Limits, next querierv1connect.QuerierServiceHandler, reg prometheus.Registerer) *Coordinator {
	return &Coordinator{
		logger:   logger,
		store:    store,
		limits:   limits,
		next:     next,
		inFlight: make(map[string]int),
		asyncQueriesCurrent: promauto.With(reg).NewGaugeVec(prometheus.GaugeOpts{
			Name: "pyroscope_async_queries_in_progress",
			Help: "Number of async queries currently in progress.",
		}, []string{"tenant"}),
		asyncQueriesMax: promauto.With(reg).NewGaugeVec(prometheus.GaugeOpts{
			Name: "pyroscope_async_queries_max",
			Help: "Maximum number of concurrent async queries allowed per tenant.",
		}, []string{"tenant"}),
	}
}

func (c *Coordinator) tryAcquire(tenantID string) error {
	maxConcurrent := c.limits.MaxAsyncQueryConcurrency(tenantID)
	c.asyncQueriesMax.WithLabelValues(tenantID).Set(float64(maxConcurrent))

	if maxConcurrent <= 0 {
		return fmt.Errorf("async queries are disabled for tenant %s", tenantID)
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.inFlight[tenantID] >= maxConcurrent {
		return fmt.Errorf("tenant %s has reached the maximum number of concurrent async queries (%d)", tenantID, maxConcurrent)
	}
	c.inFlight[tenantID]++
	c.asyncQueriesCurrent.WithLabelValues(tenantID).Set(float64(c.inFlight[tenantID]))
	return nil
}

// Submit reserves the tenant's concurrency slot, strips the Async marker
// from req, persists the resulting spec as a new in-progress query, and
// dispatches it in the background. Returns the assigned request ID.
func (c *Coordinator) Submit(ctx context.Context, tenantID string, req *querierv1.SelectMergeStacktracesRequest) (string, error) {
	if err := c.tryAcquire(tenantID); err != nil {
		return "", err
	}

	requestID := uuid.New().String()
	spec := proto.Clone(req).(*querierv1.SelectMergeStacktracesRequest)
	spec.Async = nil

	if err := c.store.create(ctx, tenantID, requestID, spec); err != nil {
		c.decrement(tenantID)
		return "", fmt.Errorf("failed to create async query: %w", err)
	}

	c.dispatch(tenantID, requestID, spec)
	level.Info(c.logger).Log("msg", "async query submitted", "tenant", tenantID, "request_id", requestID)
	return requestID, nil
}

// Dispatch implements Store's Dispatcher interface: it (re-)runs a query
// whose record and spec are already persisted. A declined dispatch (tenant
// at its concurrency limit) never rolls back the store's claim on the
// record; it is simply retried by a later adoption scan once the lease
// expires again.
func (c *Coordinator) Dispatch(tenantID, requestID string, spec *querierv1.SelectMergeStacktracesRequest) {
	if err := c.tryAcquire(tenantID); err != nil {
		level.Warn(c.logger).Log("msg", "skipping async query adoption: concurrency limit reached", "tenant", tenantID, "request_id", requestID, "err", err)
		return
	}
	c.dispatch(tenantID, requestID, spec)
}

// dispatch runs spec against next on a background context, detached from the
// caller's context, so the query survives client disconnects and outlives
// the scan tick that triggered an adoption. Submit and Dispatch both share
// this path once their concurrency slot is reserved.
func (c *Coordinator) dispatch(tenantID, requestID string, spec *querierv1.SelectMergeStacktracesRequest) {
	queryCtx := tenant.InjectTenantID(context.Background(), tenantID)
	resultCh := make(chan queryResult, 1)

	// query goroutine: runs the spec end-to-end.
	go func() {
		resp, err := c.next.SelectMergeStacktraces(queryCtx, connect.NewRequest(spec))
		if err != nil {
			resultCh <- queryResult{Err: err}
			return
		}
		resultCh <- queryResult{Response: resp.Msg}
	}()

	// supervisor goroutine: renews the lease and records the outcome.
	go c.awaitResult(tenantID, requestID, resultCh)
}

func (c *Coordinator) awaitResult(tenantID, requestID string, resultCh <-chan queryResult) {
	defer c.decrement(tenantID)

	ctx := tenant.InjectTenantID(context.Background(), tenantID)

	ticker := time.NewTicker(c.store.heartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case res := <-resultCh:
			if res.Err != nil {
				level.Error(c.logger).Log("msg", "async query failed", "tenant", tenantID, "request_id", requestID, "err", res.Err)
				if storeErr := c.store.fail(ctx, tenantID, requestID, res.Err); storeErr != nil {
					level.Error(c.logger).Log("msg", "failed to store async query failure", "tenant", tenantID, "request_id", requestID, "err", storeErr)
				}
				return
			}
			if err := c.store.complete(ctx, tenantID, requestID, res.Response); err != nil {
				level.Error(c.logger).Log("msg", "failed to store async query result", "tenant", tenantID, "request_id", requestID, "err", err)
			} else {
				level.Info(c.logger).Log("msg", "async query completed", "tenant", tenantID, "request_id", requestID)
			}
			return
		case <-ticker.C:
			if err := c.store.heartbeat(ctx, tenantID, requestID); err != nil {
				level.Warn(c.logger).Log("msg", "failed to update heartbeat", "tenant", tenantID, "request_id", requestID, "err", err)
			}
		}
	}
}

func (c *Coordinator) decrement(tenantID string) {
	c.mu.Lock()
	c.inFlight[tenantID]--
	if c.inFlight[tenantID] <= 0 {
		delete(c.inFlight, tenantID)
	}
	c.asyncQueriesCurrent.WithLabelValues(tenantID).Set(float64(c.inFlight[tenantID]))
	c.mu.Unlock()
}

// PollQuery checks the status of an async query for the given tenant.
// Returns nil if the request is not found (caller should return NotFound).
func (c *Coordinator) PollQuery(ctx context.Context, tenantID, requestID string) (*Result, error) {
	return c.store.get(ctx, tenantID, requestID)
}
