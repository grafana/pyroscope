package segmentwriterclient

import (
	"context"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/grafana/dskit/grpcclient"
	"github.com/grafana/dskit/ring"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	segmentwriterv1 "github.com/grafana/pyroscope/api/gen/proto/go/segmentwriter/v1"
	"github.com/grafana/pyroscope/pkg/experiment/ingester/client/distributor"
)

type SegmentWriterClient struct {
	logger      log.Logger
	distributor *distributor.Distributor
	ring        ring.ReadRing
	pool        ConnPool
}

func NewSegmentWriterClient(
	ring ring.ReadRing,
	distributor *distributor.Distributor,
	logger log.Logger,
	grpcClientConfig grpcclient.Config,
) (*SegmentWriterClient, error) {
	pool, err := NewConnPool(ring, logger, grpcClientConfig)
	if err != nil {
		return nil, err
	}
	c := &SegmentWriterClient{
		logger:      logger,
		distributor: distributor,
		ring:        ring,
		pool:        pool,
	}
	return c, nil
}

func (c *SegmentWriterClient) Push(
	ctx context.Context,
	req *segmentwriterv1.PushRequest,
) (*segmentwriterv1.PushResponse, error) {
	k := distributor.NewTenantServiceDatasetKey(req.TenantId, req.Labels)
	placement, err := c.distributor.Distribute(k, c.ring)
	if err != nil {
		return nil, err
	}
	return c.push(ctx, req, placement)
}

func (c *SegmentWriterClient) push(
	ctx context.Context,
	req *segmentwriterv1.PushRequest,
	placement *distributor.Placement,
) (*segmentwriterv1.PushResponse, error) {
	for {
		instance, ok := placement.Next()
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

func (c *SegmentWriterClient) pushToInstance(
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
