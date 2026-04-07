package async

import (
	"context"
	"fmt"
	"sync"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	profilev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	"github.com/grafana/pyroscope/v2/pkg/frontend"
	"github.com/grafana/pyroscope/v2/pkg/tenant"
)

type Coordinator struct {
	logger log.Logger
	store  *Store
	limits frontend.Limits

	mu       sync.Mutex
	inFlight map[string]int // tenantID -> count

	asyncQueriesCurrent *prometheus.GaugeVec
	asyncQueriesMax     *prometheus.GaugeVec
}

func NewCoordinator(logger log.Logger, store *Store, limits frontend.Limits, reg prometheus.Registerer) *Coordinator {
	c := &Coordinator{
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
	return c
}

type QueryFunc func(context.Context) (*profilev1.Profile, error)

// StartQuery starts a new async query if the tenant has capacity.
// It returns the request ID for polling.
func (c *Coordinator) StartQuery(ctx context.Context, tenantID string, queryFn QueryFunc) (string, error) {
	maxConcurrent := c.limits.MaxAsyncQueryConcurrency(tenantID)
	c.asyncQueriesMax.WithLabelValues(tenantID).Set(float64(maxConcurrent))

	if maxConcurrent <= 0 {
		return "", fmt.Errorf("async queries are disabled for tenant %s", tenantID)
	}

	c.mu.Lock()
	current := c.inFlight[tenantID]
	if current >= maxConcurrent {
		c.mu.Unlock()
		return "", fmt.Errorf("tenant %s has reached the maximum number of concurrent async queries (%d)", tenantID, maxConcurrent)
	}
	c.inFlight[tenantID]++
	c.asyncQueriesCurrent.WithLabelValues(tenantID).Set(float64(c.inFlight[tenantID]))
	c.mu.Unlock()

	requestID := uuid.New().String()

	if err := c.store.Create(ctx, tenantID, requestID); err != nil {
		c.decrement(tenantID)
		return "", fmt.Errorf("failed to create async query: %w", err)
	}

	go c.executeQuery(tenantID, requestID, queryFn)

	return requestID, nil
}

func (c *Coordinator) executeQuery(tenantID, requestID string, queryFn QueryFunc) {
	defer c.decrement(tenantID)

	// Use a background context with the tenant injected so downstream
	// handlers can extract it. The caller's context is not used because
	// the query should complete even if the caller disconnects.
	ctx := tenant.InjectTenantID(context.Background(), tenantID)

	profile, err := queryFn(ctx)
	if err != nil {
		level.Error(c.logger).Log("msg", "async query failed", "tenant", tenantID, "request_id", requestID, "err", err)
		if storeErr := c.store.Fail(ctx, tenantID, requestID, err); storeErr != nil {
			level.Error(c.logger).Log("msg", "failed to store async query failure", "tenant", tenantID, "request_id", requestID, "err", storeErr)
		}
		return
	}

	if err := c.store.Complete(ctx, tenantID, requestID, profile); err != nil {
		level.Error(c.logger).Log("msg", "failed to store async query result", "tenant", tenantID, "request_id", requestID, "err", err)
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
