package segmentwriterclient

import (
	"context"
	"fmt"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/grafana/dskit/grpcclient"
	"github.com/grafana/dskit/ring"
	ring_client "github.com/grafana/dskit/ring/client"
	"github.com/grafana/dskit/services"
	otgrpc "github.com/opentracing-contrib/go-grpc"
	"github.com/opentracing/opentracing-go"
	"github.com/sony/gobreaker/v2"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	segmentwriterv1 "github.com/grafana/pyroscope/api/gen/proto/go/segmentwriter/v1"
	"github.com/grafana/pyroscope/pkg/experiment/ingester/client/connpool"
	"github.com/grafana/pyroscope/pkg/experiment/ingester/client/distributor"
	"github.com/grafana/pyroscope/pkg/experiment/ingester/client/distributor/placement"
	"github.com/grafana/pyroscope/pkg/util/circuitbreaker"
)

var ErrServiceUnavailable = "service is unavailable"

// Circuit breaker defaults.
// TODO(kolesnikovae): Make these configurable?
const (
	cbMinSuccess     = 5
	cbMaxFailures    = 3
	cbClosedInterval = 0
	cbOpenTimeout    = 5 * time.Second

	poolCleanupPeriod = 15 * time.Second
)

const grpcServiceConfig = `{
    "methodConfig": [{
        "name": [{"service": ""}],
        "retryPolicy": {}
    }]
}`

type Client struct {
	distributor *distributor.Distributor
	logger      log.Logger
	ring        ring.ReadRing
	pool        *connpool.RingConnPool

	service     services.Service
	subservices *services.Manager
	watcher     *services.FailureWatcher
}

func NewSegmentWriterClient(
	grpcClientConfig grpcclient.Config,
	logger log.Logger,
	ring ring.ReadRing,
	dialOpts ...grpc.DialOption,
) (*Client, error) {
	pool, err := newConnPool(ring, logger, grpcClientConfig, dialOpts...)
	if err != nil {
		return nil, err
	}
	c := &Client{
		distributor: distributor.NewDistributor(placement.DefaultPlacement),
		logger:      logger,
		ring:        ring,
		pool:        pool,
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
) (*segmentwriterv1.PushResponse, error) {
	k := distributor.NewTenantServiceDatasetKey(req.TenantId, req.Labels)
	p, dErr := c.distributor.Distribute(k, c.ring)
	if dErr != nil {
		_ = level.Error(c.logger).Log(
			"msg", "unable to distribute request",
			"tenant", req.TenantId,
			"err", dErr,
		)
		return nil, status.Error(codes.Unavailable, ErrServiceUnavailable)
	}
	req.Shard = p.Shard
	for {
		instance, ok := p.Next()
		if !ok {
			_ = level.Error(c.logger).Log(
				"msg", "no segment writer instances available for the request",
				"tenant", req.TenantId,
			)
			return nil, status.Error(codes.Unavailable, ErrServiceUnavailable)
		}
		resp, err := c.pushToInstance(ctx, req, instance.Addr)
		if err == nil {
			return resp, nil
		}
		_ = level.Warn(c.logger).Log(
			"msg", "failed to push data to segment writer",
			"tenant", req.TenantId,
			"instance", instance.Addr,
			"err", err,
		)
		// These are "retryable" errors.
		switch status.Code(err) {
		case codes.ResourceExhausted,
			codes.Unavailable:
			continue
		default:
			// Any other error returned as is.
			return nil, err
		}
	}
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
	return segmentwriterv1.NewSegmentWriterServiceClient(conn).Push(ctx, req)
}

func newConnPool(
	rring ring.ReadRing,
	logger log.Logger,
	grpcClientConfig grpcclient.Config,
	dialOpts ...grpc.DialOption,
) (*connpool.RingConnPool, error) {
	options, err := grpcClientConfig.DialOption(nil, nil)
	if err != nil {
		return nil, err
	}

	// The options (including interceptors) are shared by all client connections.
	options = append(options, dialOpts...)
	options = append(options,
		grpc.WithUnaryInterceptor(otgrpc.OpenTracingClientInterceptor(opentracing.GlobalTracer())),
		grpc.WithDefaultServiceConfig(grpcServiceConfig),
	)

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
	cbconfig := gobreaker.Settings{
		MaxRequests: cbMinSuccess,
		Interval:    cbClosedInterval,
		Timeout:     cbOpenTimeout,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			return counts.ConsecutiveFailures >= cbMaxFailures
		},
		IsSuccessful: func(err error) bool {
			// Only these codes are counted towards tripping the open state,
			// when no requests flow through the circuit.
			//
			// From the caller perspective, we're converting those to
			// UNAVAILABLE, thereby allowing the caller to retry the request
			// against another service instance.
			//
			// Note that client-side, internal, and unknown errors are not
			// included: in case if a request was failing permanently, there
			// would be a chance to "open" all the circuits (if the caller
			// retries), making the whole service unavailable.
			switch status.Code(err) {
			case codes.Unavailable,
				codes.DeadlineExceeded:
				return false
			}
			return true
		},
	}

	// Note that circuit breaker must be created per client conn.
	factory := connpool.NewConnPoolFactory(func(desc ring.InstanceDesc) []grpc.DialOption {
		cb := gobreaker.NewCircuitBreaker[any](cbconfig)
		return append(options, grpc.WithUnaryInterceptor(circuitbreaker.UnaryClientInterceptor(cb)))
	})

	p := ring_client.NewPool(
		"segment-writer",
		// Discovery is used to remove clients that can't be found
		// in the ring, including unhealthy instances. CheckInterval
		// specifies how frequently the stale clients are removed.
		ring_client.PoolConfig{
			CheckInterval: poolCleanupPeriod,
			// Note that no health checks are performed: it's caller
			// responsibility to pick the healthy clients.
			HealthCheckEnabled:        false,
			HealthCheckTimeout:        0,
			MaxConcurrentHealthChecks: 0,
		},
		// Discovery builds a list of healthy instances.
		// An instance is healthy, if it's heartbeat timestamp
		// is not older than a configured threshold (intrinsic
		// to the ring itself).
		ring_client.NewRingServiceDiscovery(rring),
		factory,
		nil, // Client count gauge is not used.
		logger,
	)

	return &connpool.RingConnPool{Pool: p}, nil
}
