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
	"google.golang.org/grpc"
	"google.golang.org/grpc/health/grpc_health_v1"

	segmentwriterv1 "github.com/grafana/pyroscope/api/gen/proto/go/segmentwriter/v1"
	"github.com/grafana/pyroscope/pkg/util/health"
)

const cleanupPeriod = 15 * time.Second

const grpcServiceConfig = `{
    "methodConfig": [{
        "name": [{"service": ""}],
        "retryPolicy": {}
    }]
}`

type ClientPool struct{ pool *ring_client.Pool }

func NewClient(ring ring.ReadRing, logger log.Logger, grpcClientConfig grpcclient.Config) (*ClientPool, error) {
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

func newSegmentWriterRingClientPool(ring ring.ReadRing, logger log.Logger, grpcClientConfig grpcclient.Config) (*ring_client.Pool, error) {
	options, err := grpcClientConfig.DialOption(nil, nil)
	if err != nil {
		return nil, err
	}
	factory := newSegmentWriterPoolFactory(append(options,
		grpc.WithUnaryInterceptor(otgrpc.OpenTracingClientInterceptor(opentracing.GlobalTracer())),
		grpc.WithDefaultServiceConfig(grpcServiceConfig),
	)...)

	p := ring_client.NewPool(
		"segment-writer",
		// Discovery is used to remove clients that can't be found
		// in the ring, including unhealthy instances. CheckInterval
		// specifies how frequently the stale clients are removed.
		ring_client.PoolConfig{
			CheckInterval: cleanupPeriod,
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
		ring_client.NewRingServiceDiscovery(ring),
		factory,
		nil, // Client count gauge is not used.
		logger,
	)
	return p, nil
}

type segmentWriterPoolFactory struct {
	options []grpc.DialOption
}

func newSegmentWriterPoolFactory(options ...grpc.DialOption) ring_client.PoolFactory {
	return &segmentWriterPoolFactory{
		options: options,
	}
}

func (f *segmentWriterPoolFactory) FromInstance(inst ring.InstanceDesc) (ring_client.PoolClient, error) {
	conn, err := grpc.Dial(inst.Addr, f.options...)
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
