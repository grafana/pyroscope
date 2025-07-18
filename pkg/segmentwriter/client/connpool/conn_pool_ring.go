package connpool

import (
	"io"

	"github.com/grafana/dskit/services"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health/grpc_health_v1"

	"github.com/grafana/dskit/ring"
	ring_client "github.com/grafana/dskit/ring/client"

	"github.com/grafana/pyroscope/pkg/util/health"
)

// NOTE(kolesnikovae): This is a tiny wrapper for ring_client.Pool
// that is not tailored for the specific use case of the segment
// writer client, and it should be refactored out.

type ConnPool interface {
	GetConnFor(addr string) (grpc.ClientConnInterface, error)
	services.Service
}

type Pool struct{ *ring_client.Pool }

func (p *Pool) GetConnFor(addr string) (grpc.ClientConnInterface, error) {
	c, err := p.GetClientFor(addr)
	if err != nil {
		return nil, err
	}
	return c.(grpc.ClientConnInterface), nil
}

type ConnFactory struct {
	options func(ring.InstanceDesc) []grpc.DialOption
}

func NewConnPoolFactory(options func(ring.InstanceDesc) []grpc.DialOption) ring_client.PoolFactory {
	return &ConnFactory{options: options}
}

func (f *ConnFactory) FromInstance(inst ring.InstanceDesc) (ring_client.PoolClient, error) {
	conn, err := grpc.NewClient(inst.Addr, f.options(inst)...)
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
