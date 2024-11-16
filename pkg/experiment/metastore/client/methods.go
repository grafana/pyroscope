package metastoreclient

import (
	"context"
	"fmt"
	"math/rand"
	"time"

	"github.com/hashicorp/raft"
	"google.golang.org/grpc"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/pkg/experiment/metastore/raftnode"
	"github.com/grafana/pyroscope/pkg/experiment/metastore/raftnode/raftnodepb"
)

func invoke[R any](ctx context.Context, cl *Client,
	f func(ctx context.Context, instance instance) (*R, error),
) (*R, error) {
	const (
		n        = 50
		backoff  = 51 * time.Millisecond
		deadline = 500000000 * time.Millisecond
	)

	ctx, cancel := context.WithDeadline(ctx, time.Now().Add(deadline))
	defer cancel()

	for i := 0; i < n; i++ {
		err := ctx.Err()
		if err != nil {
			return nil, fmt.Errorf("metastore client timeout %w", err)
		}
		it := cl.selectInstance()
		if it == nil {
			cl.logger.Log("msg", "no instances available, backoff and retry")
			time.Sleep(backoff)
			cl.discovery.Rediscover()
			continue
		}
		res, err := f(ctx, it)
		if err == nil {
			return res, nil
		}
		cl.logger.Log(
			"msg", "metastore client error",
			"err", err,
			"server_id", it.srv.Raft.ID,
			"server_address", it.srv.Raft.Address,
			"server_resolved_laddress", it.srv.ResolvedAddress,
		)
		node, ok := raftnode.RaftLeaderFromStatusDetails(err)
		if ok {
			cl.mu.Lock()
			if cl.leader == it.srv.Raft.ID {
				cl.leader = raft.ServerID(node.Id)
			}
			cl.mu.Unlock()
		}
		time.Sleep(backoff)
		cl.discovery.Rediscover()
	}
	return nil, fmt.Errorf("metastore client retries failed")
}

func (c *Client) selectInstance() *client {
	c.mu.Lock()
	it := c.servers[c.leader]
	if it == nil && len(c.servers) > 0 {
		idx := rand.Intn(len(c.servers))
		j := 0
		for k, v := range c.servers {
			if j == idx {
				it = v
				c.leader = k
				break
			}
			j++
		}
	}
	c.mu.Unlock()
	return it
}

// TODO(kolesnikovae): Interceptor.

func (c *Client) AddBlock(ctx context.Context, in *metastorev1.AddBlockRequest, opts ...grpc.CallOption) (*metastorev1.AddBlockResponse, error) {
	return invoke(ctx, c, func(ctx context.Context, instance instance) (*metastorev1.AddBlockResponse, error) {
		return instance.AddBlock(ctx, in, opts...)
	})
}

func (c *Client) QueryMetadata(ctx context.Context, in *metastorev1.QueryMetadataRequest, opts ...grpc.CallOption) (*metastorev1.QueryMetadataResponse, error) {
	return invoke(ctx, c, func(ctx context.Context, instance instance) (*metastorev1.QueryMetadataResponse, error) {
		return instance.QueryMetadata(ctx, in, opts...)
	})
}

func (c *Client) PollCompactionJobs(ctx context.Context, in *metastorev1.PollCompactionJobsRequest, opts ...grpc.CallOption) (*metastorev1.PollCompactionJobsResponse, error) {
	return invoke(ctx, c, func(ctx context.Context, instance instance) (*metastorev1.PollCompactionJobsResponse, error) {
		return instance.PollCompactionJobs(ctx, in, opts...)
	})
}

func (c *Client) GetTenant(ctx context.Context, in *metastorev1.GetTenantRequest, opts ...grpc.CallOption) (*metastorev1.GetTenantResponse, error) {
	return invoke(ctx, c, func(ctx context.Context, instance instance) (*metastorev1.GetTenantResponse, error) {
		return instance.GetTenant(ctx, in, opts...)
	})
}

func (c *Client) DeleteTenant(ctx context.Context, in *metastorev1.DeleteTenantRequest, opts ...grpc.CallOption) (*metastorev1.DeleteTenantResponse, error) {
	return invoke(ctx, c, func(ctx context.Context, instance instance) (*metastorev1.DeleteTenantResponse, error) {
		return instance.DeleteTenant(ctx, in, opts...)
	})
}

func (c *Client) ReadIndex(ctx context.Context, in *raftnodepb.ReadIndexRequest, opts ...grpc.CallOption) (*raftnodepb.ReadIndexResponse, error) {
	return invoke(ctx, c, func(ctx context.Context, instance instance) (*raftnodepb.ReadIndexResponse, error) {
		return instance.ReadIndex(ctx, in, opts...)
	})
}

func (c *Client) NodeInfo(ctx context.Context, in *raftnodepb.NodeInfoRequest, opts ...grpc.CallOption) (*raftnodepb.NodeInfoResponse, error) {
	return invoke(ctx, c, func(ctx context.Context, instance instance) (*raftnodepb.NodeInfoResponse, error) {
		return instance.NodeInfo(ctx, in, opts...)
	})
}
