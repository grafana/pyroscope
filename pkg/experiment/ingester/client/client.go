package segmentwriterclient

import (
	"context"
	"fmt"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/grafana/dskit/grpcclient"
	"github.com/grafana/dskit/ring"
	"github.com/grafana/dskit/services"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	segmentwriterv1 "github.com/grafana/pyroscope/api/gen/proto/go/segmentwriter/v1"
	"github.com/grafana/pyroscope/pkg/experiment/ingester/client/distributor"
	"github.com/grafana/pyroscope/pkg/experiment/ingester/client/distributor/placement"
)

type ConnPool interface {
	GetConnFor(addr string) (grpc.ClientConnInterface, error)
	services.Service
}

type Client struct {
	distributor *distributor.Distributor
	logger      log.Logger
	ring        ring.ReadRing
	pool        ConnPool

	service     services.Service
	subservices *services.Manager
	watcher     *services.FailureWatcher
}

func NewSegmentWriterClient(
	grpcClientConfig grpcclient.Config,
	logger log.Logger,
	ring ring.ReadRing,
) (*Client, error) {
	pool, err := NewConnPool(ring, logger, grpcClientConfig)
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
	p, err := c.distributor.Distribute(k, c.ring)
	if err != nil {
		return nil, err
	}
	req.Shard = p.Shard
	return c.push(ctx, req, p)
}

func (c *Client) push(
	ctx context.Context,
	req *segmentwriterv1.PushRequest,
	p *placement.Placement,
) (*segmentwriterv1.PushResponse, error) {
	for {
		instance, ok := p.Next()
		if !ok {
			return nil, status.Error(codes.Unavailable, "service is unavailable")
		}
		resp, err := c.pushToInstance(ctx, req, instance.Addr)
		if err == nil {
			return resp, nil
		}
		_ = level.Warn(c.logger).Log(
			"msg", "failed to push data to segment writer", "err",
			err, "instance", instance.Addr,
		)
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
