package sewgmentwriterclient

import (
	"io"
	"time"

	"github.com/go-kit/log"
	"github.com/grafana/dskit/grpcclient"
	"github.com/grafana/dskit/ring"
	ring_client "github.com/grafana/dskit/ring/client"
	"github.com/opentracing-contrib/go-grpc"
	"github.com/opentracing/opentracing-go"
	"github.com/sony/gobreaker/v2"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/status"

	segmentwriterv1 "github.com/grafana/pyroscope/api/gen/proto/go/segmentwriter/v1"
	"github.com/grafana/pyroscope/pkg/util/circuitbreaker"
	"github.com/grafana/pyroscope/pkg/util/health"
)

const poolCleanupPeriod = 15 * time.Second

// Circuit breaker defaults.
// TODO(kolesnikovae): Configurable?
const (
	cbMinSuccess     = 5
	cbMaxFailures    = 3
	cbClosedInterval = 0
	cbOpenTimeout    = 5 * time.Second
)

const grpcServiceConfig = `{
    "methodConfig": [{
        "name": [{"service": ""}],
        "retryPolicy": {}
    }]
}`

type ClientPool struct{ pool *ring_client.Pool }

func NewClientPool(ring ring.ReadRing, logger log.Logger, grpcClientConfig grpcclient.Config) (*ClientPool, error) {
	p, err := newSegmentWriterRingClientPool(ring, logger, grpcClientConfig)
	if err != nil {
		return nil, err
	}
	return &ClientPool{pool: p}, nil
}

func (p *ClientPool) GetClientFor(addr string) (segmentwriterv1.SegmentWriterServiceClient, error) {
	c, err := p.pool.GetClientFor(addr)
	if err != nil {
		return nil, err
	}
	return c.(segmentwriterv1.SegmentWriterServiceClient), nil
}

func newSegmentWriterRingClientPool(rring ring.ReadRing, logger log.Logger, grpcClientConfig grpcclient.Config) (*ring_client.Pool, error) {
	options, err := grpcClientConfig.DialOption(nil, nil)
	if err != nil {
		return nil, err
	}

	// https://en.wikipedia.org/wiki/Circuit_breaker_design_pattern
	// The circuit breaker is used to prevent the client from sending
	// requests to unhealthy instances. The logic is as follows:
	//
	// Once we observe 3 consecutive failures, the circuit breaker will trip
	// and open the circuit – any attempt to send a request will fail
	// immediately with a "circuit breaker is open" error.
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
			// Only these codes are counted towards tripping
			// the open state (no requests flow through).
			switch status.Code(err) {
			case codes.Unavailable,
				codes.DeadlineExceeded,
				codes.ResourceExhausted:
				return false
			}
			return true
		},
	}

	// Note that interceptors are created per client.
	factory := newSegmentWriterPoolFactory(func(desc ring.InstanceDesc) []grpc.DialOption {
		return append(options,
			grpc.WithUnaryInterceptor(circuitbreaker.UnaryClientInterceptor(gobreaker.NewCircuitBreaker[any](cbconfig))),
			grpc.WithUnaryInterceptor(otgrpc.OpenTracingClientInterceptor(opentracing.GlobalTracer())),
			grpc.WithDefaultServiceConfig(grpcServiceConfig),
		)
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

	return p, nil
}

type segmentWriterPoolFactory struct {
	options func(ring.InstanceDesc) []grpc.DialOption
}

func newSegmentWriterPoolFactory(options func(ring.InstanceDesc) []grpc.DialOption) ring_client.PoolFactory {
	return &segmentWriterPoolFactory{
		options: options,
	}
}

func (f *segmentWriterPoolFactory) FromInstance(inst ring.InstanceDesc) (ring_client.PoolClient, error) {
	conn, err := grpc.Dial(inst.Addr, f.options(inst)...)
	if err != nil {
		return nil, err
	}
	return &segmentWriterPoolClient{
		SegmentWriterServiceClient: segmentwriterv1.NewSegmentWriterServiceClient(conn),
		HealthClient:               health.NoOpClient,
		Closer:                     conn,
	}, nil
}

type segmentWriterPoolClient struct {
	segmentwriterv1.SegmentWriterServiceClient
	grpc_health_v1.HealthClient
	io.Closer
}
