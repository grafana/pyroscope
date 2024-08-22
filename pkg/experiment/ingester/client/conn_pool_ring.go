package segmentwriterclient

import (
	"io"

	"google.golang.org/grpc"
	"google.golang.org/grpc/health/grpc_health_v1"

	"github.com/grafana/dskit/ring"
	ring_client "github.com/grafana/dskit/ring/client"

	"github.com/grafana/pyroscope/pkg/util/health"
)

type RingConnPool struct{ *ring_client.Pool }

func (p *RingConnPool) GetConnFor(addr string) (grpc.ClientConnInterface, error) {
	c, err := p.Pool.GetClientFor(addr)
	if err != nil {
		return nil, err
	}
	return c.(grpc.ClientConnInterface), nil
}

type connFactory struct {
	options func(ring.InstanceDesc) []grpc.DialOption
}

func newConnPoolFactory(options func(ring.InstanceDesc) []grpc.DialOption) ring_client.PoolFactory {
	return &connFactory{
		options: options,
	}
}

func (f *connFactory) FromInstance(inst ring.InstanceDesc) (ring_client.PoolClient, error) {
	conn, err := grpc.Dial(inst.Addr, f.options(inst)...)
	if err != nil {
		return nil, err
	}
	return &poolConn{
		ClientConnInterface: conn,
		HealthClient:        health.NoOpClient,
		Closer:              conn,
	}, nil
}

type poolConn struct {
	grpc.ClientConnInterface
	grpc_health_v1.HealthClient
	io.Closer
}
