package metastoreclient

import (
	"context"
	"math/rand"
	"strings"
	"time"

	"github.com/go-kit/log/level"
	"github.com/grafana/dskit/backoff"
	"github.com/hashicorp/raft"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/pkg/experiment/metastore/raftnode"
	"github.com/grafana/pyroscope/pkg/experiment/metastore/raftnode/raftnodepb"
)

func invoke[R any](ctx context.Context, cl *Client,
	f func(ctx context.Context, instance instance) (*R, error),
) (*R, error) {
	backoffConfig := backoff.Config{
		MinBackoff: 10 * time.Millisecond,
		MaxBackoff: 100 * time.Millisecond,
		MaxRetries: 50,
	}
	const (
		deadline = 20 * time.Second
	)

	ctx, cancel := context.WithDeadline(ctx, time.Now().Add(deadline))
	defer cancel()

	var res *R
	var err error

	attempt := func() (done bool) {
		it := cl.selectInstance(false)
		if it == nil {
			cl.logger.Log("msg", "no instances available, backoff and retry")
			return false
		}
		responseFromAttempt, errFromAttempt := f(ctx, it)
		if errFromAttempt == nil {
			res = responseFromAttempt
			return true
		}
		cl.logger.Log(
			"msg", "metastore client error",
			"err", errFromAttempt,
			"server_id", it.srv.Raft.ID,
			"server_address", it.srv.Raft.Address,
			"server_resolved_address", it.srv.ResolvedAddress,
		)
		node, ok := raftnode.RaftLeaderFromStatusDetails(errFromAttempt)
		if ok {
			cl.mu.Lock()
			if strings.Contains(string(it.srv.Raft.ID), string(cl.leader)) {
				cl.logger.Log("msg", "changing metastore client leader", "current", cl.leader, "new", node.Id)
				cl.leader = stripPort(node.Id)
			}
			cl.mu.Unlock()
		} else {
			// Some errors will not contain the Raft leader. This is a valid scenario, e.g., when a node gets removed
			// for maintenance. We try to move to a different client instance.
			cl.selectInstance(true)
		}
		// A workaround to prevent retries for specific error codes. This needs a larger refactoring later on.
		switch status.Code(errFromAttempt) {
		case codes.InvalidArgument:
			cl.logger.Log("msg", "skip metastore retries", "err", err, "leader", cl.leader)
			err = errFromAttempt
			return true
		}
		return false
	}

	b := backoff.New(ctx, backoffConfig)

	for b.Ongoing() {
		if !attempt() {
			b.Wait()
			cl.discovery.Rediscover()
		} else {
			return res, err
		}
	}

	return nil, b.Err()
}

func (c *Client) selectInstance(override bool) *client {
	c.mu.Lock()
	defer c.mu.Unlock()

	it := c.servers[c.leader]
	if (it == nil || override) && len(c.servers) > 0 {
		idx := rand.Intn(len(c.servers))
		j := 0
		for k, v := range c.servers {
			if j == idx {
				it = v
				c.leader = k
				level.Debug(c.logger).Log("msg", "selected a random metastore server", "new_leader", c.leader)
				break
			}
			j++
		}
	}
	return it
}

func stripPort(server string) raft.ServerID {
	serverWithoutPort := server
	if idx := strings.LastIndex(serverWithoutPort, ":"); idx != -1 {
		serverWithoutPort = serverWithoutPort[:idx]
	}
	return raft.ServerID(serverWithoutPort)
}

// TODO(kolesnikovae): Interceptor.

func (c *Client) AddBlock(ctx context.Context, in *metastorev1.AddBlockRequest, opts ...grpc.CallOption) (*metastorev1.AddBlockResponse, error) {
	return invoke(ctx, c, func(ctx context.Context, instance instance) (*metastorev1.AddBlockResponse, error) {
		return instance.AddBlock(ctx, in, opts...)
	})
}

func (c *Client) GetBlockMetadata(ctx context.Context, in *metastorev1.GetBlockMetadataRequest, opts ...grpc.CallOption) (*metastorev1.GetBlockMetadataResponse, error) {
	return invoke(ctx, c, func(ctx context.Context, instance instance) (*metastorev1.GetBlockMetadataResponse, error) {
		return instance.GetBlockMetadata(ctx, in, opts...)
	})
}

func (c *Client) QueryMetadata(ctx context.Context, in *metastorev1.QueryMetadataRequest, opts ...grpc.CallOption) (*metastorev1.QueryMetadataResponse, error) {
	return invoke(ctx, c, func(ctx context.Context, instance instance) (*metastorev1.QueryMetadataResponse, error) {
		return instance.QueryMetadata(ctx, in, opts...)
	})
}

func (c *Client) QueryMetadataLabels(ctx context.Context, in *metastorev1.QueryMetadataLabelsRequest, opts ...grpc.CallOption) (*metastorev1.QueryMetadataLabelsResponse, error) {
	return invoke(ctx, c, func(ctx context.Context, instance instance) (*metastorev1.QueryMetadataLabelsResponse, error) {
		return instance.QueryMetadataLabels(ctx, in, opts...)
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

func (c *Client) RemoveNode(ctx context.Context, in *raftnodepb.RemoveNodeRequest, opts ...grpc.CallOption) (*raftnodepb.RemoveNodeResponse, error) {
	return invoke(ctx, c, func(ctx context.Context, instance instance) (*raftnodepb.RemoveNodeResponse, error) {
		return instance.RemoveNode(ctx, in, opts...)
	})
}

func (c *Client) AddNode(ctx context.Context, in *raftnodepb.AddNodeRequest, opts ...grpc.CallOption) (*raftnodepb.AddNodeResponse, error) {
	return invoke(ctx, c, func(ctx context.Context, instance instance) (*raftnodepb.AddNodeResponse, error) {
		return instance.AddNode(ctx, in, opts...)
	})
}

func (c *Client) DemoteLeader(ctx context.Context, in *raftnodepb.DemoteLeaderRequest, opts ...grpc.CallOption) (*raftnodepb.DemoteLeaderResponse, error) {
	return invoke(ctx, c, func(ctx context.Context, instance instance) (*raftnodepb.DemoteLeaderResponse, error) {
		return instance.DemoteLeader(ctx, in, opts...)
	})
}

func (c *Client) PromoteToLeader(ctx context.Context, in *raftnodepb.PromoteToLeaderRequest, opts ...grpc.CallOption) (*raftnodepb.PromoteToLeaderResponse, error) {
	return invoke(ctx, c, func(ctx context.Context, instance instance) (*raftnodepb.PromoteToLeaderResponse, error) {
		return instance.PromoteToLeader(ctx, in, opts...)
	})
}
