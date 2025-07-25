package segmentwriterclient

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/grafana/dskit/grpcclient"
	"github.com/grafana/dskit/ring"
	ring_client "github.com/grafana/dskit/ring/client"
	"github.com/grafana/dskit/services"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/sony/gobreaker/v2"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	segmentwriterv1 "github.com/grafana/pyroscope/api/gen/proto/go/segmentwriter/v1"
	"github.com/grafana/pyroscope/pkg/segmentwriter/client/connpool"
	"github.com/grafana/pyroscope/pkg/segmentwriter/client/distributor"
	"github.com/grafana/pyroscope/pkg/segmentwriter/client/distributor/placement"
	"github.com/grafana/pyroscope/pkg/util/circuitbreaker"
)

var errServiceUnavailableMsg = "service is unavailable"

// TODO(kolesnikovae):
//  * Replace the ring service discovery and client pool implementations.
//  * Make CB options configurable.

const (
	// Circuit breaker defaults.
	cbMinSuccess     = 5
	cbMaxFailures    = 3
	cbClosedInterval = 0
	cbOpenTimeout    = time.Second

	poolCleanupPeriod = 15 * time.Second
)

// Only these errors are considered as a signal to retry the request
// and send it to another instance. Client-side, internal, and unknown
// errors should not be retried, as they are likely to be permanent.
// Note that the client errors are not excluded from the list.
func isRetryable(err error) bool {
	switch status.Code(err) {
	case codes.Unknown,
		codes.Internal,
		codes.FailedPrecondition:
		return false
	default:
		// All sorts of network errors.
		return true
	}
}

// Client errors are returned as is without retries.
// Any other error is substituted with a stub message
// and UNAVAILABLE status.
func isClientError(err error) bool {
	switch status.Code(err) {
	case codes.InvalidArgument,
		codes.Canceled,
		codes.PermissionDenied,
		codes.Unauthenticated:
		return true
	default:
		return errors.Is(err, context.Canceled)
	}
}

// https://en.wikipedia.org/wiki/Circuit_breaker_design_pattern
// The circuit breaker is used to prevent the client from sending
// requests to unhealthy instances. The logic is as follows:
//
// Once we observe 3 consecutive failures, the circuit breaker will trip
// and open the circuit – any attempt to send a request will fail
// immediately with a "circuit breaker is open" error (UNAVAILABLE).
//
// After the expiration of the Timeout (5 seconds), the circuit breaker will
// transition to the half-open state. In this state, if a failure occurs,
// the breaker will revert to the open state. After MaxRequests (5)
// consecutive successful requests, the circuit breaker will return to the
// closed state.
var circuitBreakerConfig = gobreaker.Settings{
	MaxRequests:  cbMinSuccess,
	Interval:     cbClosedInterval,
	Timeout:      cbOpenTimeout,
	IsSuccessful: shouldBeHandledByCaller,
	ReadyToTrip: func(counts gobreaker.Counts) bool {
		return counts.ConsecutiveFailures >= cbMaxFailures
	},
}

// If the function returns false, the error is counted towards tripping
// the open state, when no requests flow through the circuit. Otherwise,
// the error handling is returned back the caller.
//
// In fact, the configuration should only prevent sending requests
// to instances that are a-priory unable to process them at the moment,
// and we want to avoid time waste. For example, when a service instance
// went unavailable for a long period of time, or is not reposing in
// timely fashion.
//
// From the caller perspective, we're converting those to UNAVAILABLE,
// thereby allowing the caller to retry the request against another service
// instance.
//
// Note that client-side, internal, and unknown errors are not included:
// in case if a request is failing permanently regardless of the service
// instance, there is a good chance that all the circuits will be opened
// by retries, making the whole service unavailable.
//
// Next, ResourceExhausted also excluded from the list: as the error is
// tenant-request-specific, and the circuit breaker operates connection-wise.
func shouldBeHandledByCaller(err error) bool {
	if errors.Is(err, os.ErrDeadlineExceeded) {
		return false
	}
	if status.Code(err) == codes.Unavailable {
		return false
	}
	// The error handling is returned back the caller: the circuit
	// remains closed.
	return true
}

// The default gRPC service config is explicitly set to balance between
// instances.
const grpcServiceConfig = `{
    "healthCheckConfig": {
         "serviceName": "pyroscope.segment-writer"
    }
}`

type Client struct {
	logger  log.Logger
	metrics *metrics

	ring        ring.ReadRing
	pool        *connpool.Pool
	distributor *distributor.Distributor

	service     services.Service
	subservices *services.Manager
	watcher     *services.FailureWatcher
}

func NewSegmentWriterClient(
	grpcClientConfig grpcclient.Config,
	logger log.Logger,
	registry prometheus.Registerer,
	ring ring.ReadRing,
	placement placement.Placement,
	dialOpts ...grpc.DialOption,
) (*Client, error) {
	pool, err := newConnPool(ring, logger, grpcClientConfig, dialOpts...)
	if err != nil {
		return nil, err
	}
	c := &Client{
		logger:      logger,
		metrics:     newMetrics(registry),
		distributor: distributor.NewDistributor(placement, ring),
		pool:        pool,
		ring:        ring,
	}
	c.subservices, err = services.NewManager(c.pool)
	if err != nil {
		return nil, fmt.Errorf("services manager: %w", err)
	}
	c.watcher = services.NewFailureWatcher()
	c.watcher.WatchManager(c.subservices)
	c.service = services.NewBasicService(c.starting, c.running, c.stopping)
	return c, nil
}

func (c *Client) Service() services.Service { return c.service }

func (c *Client) starting(ctx context.Context) error {
	// Warm up connections. The pool does not do this.
	instances, err := c.ring.GetAllHealthy(ring.Reporting)
	if err != nil {
		// The ring might be empty initially if the segment-writer service
		// is not yet ready. In such cases, we avoid failing the client to
		// allow for eventual readiness.
		level.Debug(c.logger).Log("msg", "unable to create connections", "err", err)
	} else {
		var wg sync.WaitGroup
		for _, x := range instances.Instances {
			wg.Add(1)
			go func(x ring.InstanceDesc) {
				defer wg.Done()
				_, _ = c.pool.GetClientFor(x.Addr)
			}(x)
		}
		wg.Wait()
	}
	return services.StartManagerAndAwaitHealthy(ctx, c.subservices)
}

func (c *Client) running(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return nil
	case err := <-c.watcher.Chan():
		return fmt.Errorf("segement writer client subservice failed: %w", err)
	}
}

func (c *Client) stopping(_ error) error {
	return services.StopManagerAndAwaitStopped(context.Background(), c.subservices)
}

func (c *Client) Push(
	ctx context.Context,
	req *segmentwriterv1.PushRequest,
) (resp *segmentwriterv1.PushResponse, err error) {
	k := distributor.NewTenantServiceDatasetKey(req.TenantId, req.Labels...)
	p, dErr := c.distributor.Distribute(k)
	if dErr != nil {
		level.Error(c.logger).Log(
			"msg", "unable to distribute request",
			"tenant", req.TenantId,
			"err", dErr,
		)
		return nil, status.Error(codes.Unavailable, errServiceUnavailableMsg)
	}

	// In case of a failure, the request is sent to another instance.
	// At most 5 attempts to push the data to the segment writer.
	instances := placement.ActiveInstances(p.Instances)
	req.Shard = p.Shard
	for attempts := 5; attempts >= 0 && instances.Next(); attempts-- {
		instance := instances.At()
		logger := log.With(c.logger,
			"tenant", req.TenantId,
			"shard", req.Shard,
			"instance_addr", instance.Addr,
			"instance_id", instance.Id,
			"attempts_left", attempts,
		)
		level.Debug(logger).Log("msg", "sending request")
		resp, err = c.pushToInstance(ctx, req, instance.Addr)
		if err == nil {
			return resp, nil
		}
		if isClientError(err) {
			return nil, err
		}
		if !isRetryable(err) {
			level.Error(logger).Log("msg", "failed to push data to segment writer", "err", err)
			return nil, status.Error(codes.Unavailable, errServiceUnavailableMsg)
		}
		level.Warn(logger).Log("msg", "failed attempt to push data to segment writer", "err", err)
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
	}

	level.Error(c.logger).Log(
		"msg", "no segment writer instances available for the request",
		"tenant", req.TenantId,
		"shard", req.Shard,
		"last_err", err,
	)

	return nil, status.Error(codes.Unavailable, errServiceUnavailableMsg)
}

func (c *Client) pushToInstance(
	ctx context.Context,
	req *segmentwriterv1.PushRequest,
	addr string,
) (*segmentwriterv1.PushResponse, error) {
	conn, err := c.pool.GetConnFor(addr)
	if err != nil {
		return nil, err
	}
	// We explicitly force the client to not wait for the connection:
	// if the connection is not ready, the client will go to the next
	// instance.
	client := segmentwriterv1.NewSegmentWriterServiceClient(conn)
	resp, err := client.Push(ctx, req, grpc.WaitForReady(false))
	if err == nil {
		c.metrics.sentBytes.
			WithLabelValues(strconv.Itoa(int(req.Shard)), req.TenantId, addr).
			Observe(float64(len(req.Profile)))
	}
	return resp, err
}

func newConnPool(
	rring ring.ReadRing,
	logger log.Logger,
	grpcClientConfig grpcclient.Config,
	dialOpts ...grpc.DialOption,
) (*connpool.Pool, error) {
	options, err := grpcClientConfig.DialOption(nil, nil)
	if err != nil {
		return nil, err
	}

	// The options (including interceptors) are shared by all client connections.
	options = append(options, dialOpts...)
	options = append(options,
		grpc.WithDefaultServiceConfig(grpcServiceConfig),
		// Just in case: we explicitly disable the built-in
		// retry mechanism of the gRPC client.
		grpc.WithDisableRetry(),
	)

	// Note that circuit breaker must be created per client conn.
	factory := connpool.NewConnPoolFactory(func(ring.InstanceDesc) []grpc.DialOption {
		cb := circuitbreaker.UnaryClientInterceptor(gobreaker.NewCircuitBreaker[any](circuitBreakerConfig))
		return append(options, grpc.WithUnaryInterceptor(cb))
	})

	p := ring_client.NewPool(
		"segment-writer",
		ring_client.PoolConfig{
			CheckInterval: poolCleanupPeriod,
			// Note that health checks are not used: gGRPC health-checking
			// is done at the gRPC connection level.
			HealthCheckEnabled:        false,
			HealthCheckTimeout:        0,
			MaxConcurrentHealthChecks: 0,
		},
		// Discovery is used to remove clients that can't be found
		// in the ring, including unhealthy instances. CheckInterval
		// specifies how frequently the stale clients are removed.
		// Discovery builds a list of healthy instances.
		// An instance is healthy, if it's heartbeat timestamp
		// is not older than a configured threshold (intrinsic
		// to the ring itself).
		ring_client.NewRingServiceDiscovery(rring),
		factory,
		nil, // Client count gauge is not used.
		logger,
	)

	return &connpool.Pool{Pool: p}, nil
}
