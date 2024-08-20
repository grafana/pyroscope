package sewgmentwriterclient

import (
	"io"

	"github.com/go-kit/log"
	"github.com/grafana/dskit/ring"
	ring_client "github.com/grafana/dskit/ring/client"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health/grpc_health_v1"

	segmentwriterv1 "github.com/grafana/pyroscope/api/gen/proto/go/segmentwriter/v1"
)

func NewSegmentWriterClientPool(ring ring.ReadRing, logger log.Logger) *ring_client.Pool {
	return ring_client.NewPool(
		"segment-writer",
		ring_client.PoolConfig{}, // No health checks.
		ring_client.NewRingServiceDiscovery(ring),
		newSegmentWriterPoolFactory(),
		nil, // Client count gauge is not used.
		logger,
	)
}

type segmentWriterPoolFactory struct{}

func newSegmentWriterPoolFactory() ring_client.PoolFactory { return &segmentWriterPoolFactory{} }

func (f *segmentWriterPoolFactory) FromInstance(inst ring.InstanceDesc) (ring_client.PoolClient, error) {
	conn, err := grpc.Dial(inst.Addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, err
	}
	// TODO(kolesnikovae): Interceptors.
	return &segmentWriterPoolClient{
		SegmentWriterServiceClient: segmentwriterv1.NewSegmentWriterServiceClient(conn),
		HealthClient:               grpc_health_v1.NewHealthClient(conn),
		Closer:                     conn,
	}, nil
}

type segmentWriterPoolClient struct {
	segmentwriterv1.SegmentWriterServiceClient
	grpc_health_v1.HealthClient
	io.Closer
}
