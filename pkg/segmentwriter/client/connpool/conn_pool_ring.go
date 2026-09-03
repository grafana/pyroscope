package connpool

import (
	"io"

	"github.com/grafana/dskit/services"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health/grpc_health_v1"

	"github.com/grafana/dskit/ring"
	ring_client "github.com/grafana/dskit/ring/client"

	"github.com/grafana/pyroscope/v2/pkg/util/health"
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
	onClose func(addr string)
}

// NewConnPoolFactory creates a factory of pool clients. The optional onClose
// hook is called with the instance address after the pool closes the client
// connection, which happens when the instance is no longer found in the ring.
func NewConnPoolFactory(
	options func(ring.InstanceDesc) []grpc.DialOption,
	onClose func(addr string),
) ring_client.PoolFactory {
	return &ConnFactory{options: options, onClose: onClose}
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
		addr:                inst.Addr,
		onClose:             f.onClose,
	}, nil
}

type poolConn struct {
	grpc.ClientConnInterface
	grpc_health_v1.HealthClient
	io.Closer
	addr    string
	onClose func(addr string)
}

func (c *poolConn) Close() error {
	err := c.Closer.Close()
	if c.onClose != nil {
		c.onClose(c.addr)
	}
	return err
}
