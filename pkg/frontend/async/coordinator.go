package async

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	"github.com/grafana/pyroscope/v2/pkg/tenant"
)

type Limits interface {
	MaxAsyncQueryConcurrency(tenantID string) int
}

// QueryResult is the result of a query execution sent over a channel.
type QueryResult struct {
	Response *queryv1.QueryResponse
	Err      error
}

type Coordinator struct {
	logger log.Logger
	store  *Store
	limits Limits

	mu       sync.Mutex
	inFlight map[string]int // tenantID -> count

	asyncQueriesCurrent *prometheus.GaugeVec
	asyncQueriesMax     *prometheus.GaugeVec
}

func NewCoordinator(logger log.Logger, store *Store, limits Limits, reg prometheus.Registerer) *Coordinator {
	return &Coordinator{
		logger:   logger,
		store:    store,
		limits:   limits,
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

// PromoteToAsync promotes an already-running query to async execution.
// It takes ownership of the result channel: the coordinator will read from it,
// store the result, and manage the heartbeat loop. Returns the request ID.
func (c *Coordinator) PromoteToAsync(ctx context.Context, tenantID string, resultCh <-chan QueryResult) (string, error) {
	if err := c.tryAcquire(tenantID); err != nil {
		return "", err
	}

	requestID := uuid.New().String()

	if err := c.store.Create(ctx, tenantID, requestID); err != nil {
		c.decrement(tenantID)
		return "", fmt.Errorf("failed to create async query: %w", err)
	}

	go c.awaitResult(tenantID, requestID, resultCh)

	return requestID, nil
}

func (c *Coordinator) awaitResult(tenantID, requestID string, resultCh <-chan QueryResult) {
	defer c.decrement(tenantID)

	ctx := tenant.InjectTenantID(context.Background(), tenantID)

	ticker := time.NewTicker(c.store.HeartbeatInterval())
	defer ticker.Stop()

	for {
		select {
		case res := <-resultCh:
			if res.Err != nil {
				level.Error(c.logger).Log("msg", "async query failed", "tenant", tenantID, "request_id", requestID, "err", res.Err)
				if storeErr := c.store.Fail(ctx, tenantID, requestID, res.Err); storeErr != nil {
					level.Error(c.logger).Log("msg", "failed to store async query failure", "tenant", tenantID, "request_id", requestID, "err", storeErr)
				}
				return
			}
			if err := c.store.Complete(ctx, tenantID, requestID, res.Response); err != nil {
				level.Error(c.logger).Log("msg", "failed to store async query result", "tenant", tenantID, "request_id", requestID, "err", err)
			}
			return
		case <-ticker.C:
			if err := c.store.Heartbeat(ctx, tenantID, requestID); err != nil {
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
	return c.store.Get(ctx, tenantID, requestID)
}
