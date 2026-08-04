package async

import (
	"context"
	"errors"
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

// queryResult is a query execution's outcome, sent over resultCh.
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

// Submit persists req as a new in-progress query and dispatches it in the
// background, returning the assigned request ID.
func (c *Coordinator) Submit(ctx context.Context, tenantID string, req *querierv1.SelectMergeStacktracesRequest) (string, error) {
	if err := c.tryAcquire(tenantID); err != nil {
		return "", err
	}

	requestID := uuid.New().String()
	spec := proto.Clone(req).(*querierv1.SelectMergeStacktracesRequest)
	spec.Async = nil

	lease, err := c.store.create(ctx, tenantID, requestID, spec)
	if err != nil {
		c.decrement(tenantID)
		return "", fmt.Errorf("failed to create async query: %w", err)
	}

	c.dispatch(tenantID, requestID, lease, spec)
	level.Info(c.logger).Log("msg", "async query submitted", "tenant", tenantID, "request_id", requestID)
	return requestID, nil
}

// HasCapacity implements Dispatcher: a read-only peek at spare capacity.
func (c *Coordinator) HasCapacity(tenantID string) bool {
	maxConcurrent := c.limits.MaxAsyncQueryConcurrency(tenantID)
	c.mu.Lock()
	defer c.mu.Unlock()
	return maxConcurrent > 0 && c.inFlight[tenantID] < maxConcurrent
}

// Dispatch implements Dispatcher, re-running an adopted query under lease. A
// declined dispatch never rolls back the claim; a later scan retries once
// the lease expires again.
func (c *Coordinator) Dispatch(tenantID, requestID string, lease leaseHandle, spec *querierv1.SelectMergeStacktracesRequest) {
	if err := c.tryAcquire(tenantID); err != nil {
		level.Warn(c.logger).Log("msg", "skipping async query adoption: concurrency limit reached", "tenant", tenantID, "request_id", requestID, "err", err)
		return
	}
	c.dispatch(tenantID, requestID, lease, spec)
}

// dispatch runs spec on a background context so the query survives client
// disconnects; cancel lets awaitResult stop a query it no longer owns
// instead of pinning the concurrency slot forever.
func (c *Coordinator) dispatch(tenantID, requestID string, lease leaseHandle, spec *querierv1.SelectMergeStacktracesRequest) {
	queryCtx, cancel := context.WithCancel(tenant.InjectTenantID(context.Background(), tenantID))
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
	go c.awaitResult(tenantID, requestID, lease, cancel, resultCh)
}

func (c *Coordinator) awaitResult(tenantID, requestID string, lease leaseHandle, cancel context.CancelFunc, resultCh <-chan queryResult) {
	defer c.decrement(tenantID)
	defer cancel()

	ctx := tenant.InjectTenantID(context.Background(), tenantID)

	ticker := time.NewTicker(c.store.heartbeatInterval)
	defer ticker.Stop()
	tickerC := ticker.C

	for {
		select {
		case res := <-resultCh:
			c.report(ctx, tenantID, requestID, &lease, res)
			return
		case <-tickerC:
			if err := c.store.heartbeat(ctx, tenantID, requestID, &lease); errors.Is(err, errLostOwnership) {
				// Cancel the query this store no longer owns and wait for
				// its one buffered outcome. A success is a real result and
				// must be reported regardless of why ownership looks lost
				// (see heartbeat's ambiguous-ack comment); a failure is
				// dropped, since the new owner produces the record's outcome
				// and a fenced failure mark would lose anyway.
				cancel()
				if res := <-resultCh; res.Err == nil {
					c.report(ctx, tenantID, requestID, &lease, res)
				}
				return
			}
		}
	}
}

// report persists a finished query's outcome.
func (c *Coordinator) report(ctx context.Context, tenantID, requestID string, lease *leaseHandle, res queryResult) {
	if res.Err != nil {
		level.Error(c.logger).Log("msg", "async query failed", "tenant", tenantID, "request_id", requestID, "err", res.Err)
		if storeErr := c.store.fail(ctx, tenantID, requestID, lease, res.Err); storeErr != nil {
			level.Error(c.logger).Log("msg", "failed to store async query failure", "tenant", tenantID, "request_id", requestID, "err", storeErr)
		}
		return
	}
	if err := c.store.complete(ctx, tenantID, requestID, lease, res.Response); err != nil {
		level.Error(c.logger).Log("msg", "failed to store async query result", "tenant", tenantID, "request_id", requestID, "err", err)
		return
	}
	level.Info(c.logger).Log("msg", "async query completed", "tenant", tenantID, "request_id", requestID)
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
