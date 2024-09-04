package metastoreclient

import (
	"context"
	"fmt"
	compactorv1 "github.com/grafana/pyroscope/api/gen/proto/go/compactor/v1"
	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/hashicorp/raft"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"math/rand"
	"time"
)

func invoke[R any](ctx context.Context, cl *Client,
	f func(ctx context.Context, instance instance) (*R, error),
	raftStatus func(r *R) *metastorev1.RaftStatus,
) (*R, error) {
	const (
		n        = 50
		backoff  = 11 * time.Millisecond
		deadline = 500 * time.Millisecond
	)

	ctx, cancel := context.WithDeadline(ctx, time.Now().Add(deadline))
	defer cancel()

	//var triedIDs map[raft.ServerID]struct{} = nil

	for i := 0; i < n; i++ {
		err := ctx.Err()
		if err != nil {
			return nil, fmt.Errorf("metastore client timeout %w", err)
		}
		it := cl.selectInstance()
		if it == nil {
			cl.logger.Log("msg", "no instances available, backoff and retry")
			time.Sleep(backoff)
			continue
		}
		res, err := f(ctx, it)
		if err == nil {
			rs := raftStatus(res)
			if rs == nil {
				return nil, fmt.Errorf("metastore client error: no raft status")
			}
			if rs.Code == metastorev1.RaftStatusCode_OK {
				return res, nil
			}
			if rs.Code == metastorev1.RaftStatusCode_NOT_LEADER {
				if rs.Leader != "" {
					cl.mu.Lock()
					cl.leader = raft.ServerID(rs.Leader)
					cl.mu.Unlock()
					continue
				}
			}
			return nil, fmt.Errorf("metastore client error: %v", rs.Code)
		}
		cl.logger.Log(
			"msg", "metastore client error",
			"err", err,
			"server_id", it.srv.Raft.ID,
			"server_address", it.srv.Raft.Address,
			"server_resolved_laddress", it.srv.ResolvedAddress,
		)
		s, ok := status.FromError(err)
		if ok && s.Code() == codes.Unavailable {
			cl.mu.Lock()
			if cl.leader == it.srv.Raft.ID {
				cl.leader = ""
			}
			cl.mu.Unlock()
			cl.discovery.ServerError(it.srv)
		}
		time.Sleep(backoff)
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

func (c *Client) AddBlock(ctx context.Context, in *metastorev1.AddBlockRequest, opts ...grpc.CallOption) (*metastorev1.AddBlockResponse, error) {
	return invoke(ctx, c, func(ctx context.Context, instance instance) (*metastorev1.AddBlockResponse, error) {
		return instance.AddBlock(ctx, in, opts...)
	}, func(r *metastorev1.AddBlockResponse) *metastorev1.RaftStatus {
		return r.RaftStatus
	})
}

func (c *Client) QueryMetadata(ctx context.Context, in *metastorev1.QueryMetadataRequest, opts ...grpc.CallOption) (*metastorev1.QueryMetadataResponse, error) {
	return invoke(ctx, c, func(ctx context.Context, instance instance) (*metastorev1.QueryMetadataResponse, error) {
		return instance.QueryMetadata(ctx, in, opts...)
	}, func(r *metastorev1.QueryMetadataResponse) *metastorev1.RaftStatus {
		return r.RaftStatus
	})
}

func (c *Client) ReadIndex(ctx context.Context, in *metastorev1.ReadIndexRequest, opts ...grpc.CallOption) (*metastorev1.ReadIndexResponse, error) {
	return invoke(ctx, c, func(ctx context.Context, instance instance) (*metastorev1.ReadIndexResponse, error) {
		return instance.ReadIndex(ctx, in, opts...)
	}, func(r *metastorev1.ReadIndexResponse) *metastorev1.RaftStatus {
		return r.RaftStatus
	})
}

func (c *Client) PollCompactionJobs(ctx context.Context, in *compactorv1.PollCompactionJobsRequest, opts ...grpc.CallOption) (*compactorv1.PollCompactionJobsResponse, error) {
	return invoke(ctx, c, func(ctx context.Context, instance instance) (*compactorv1.PollCompactionJobsResponse, error) {
		return instance.PollCompactionJobs(ctx, in, opts...)
	}, func(r *compactorv1.PollCompactionJobsResponse) *metastorev1.RaftStatus {
		return nil // todo
	})
}

func (c *Client) GetCompactionJobs(ctx context.Context, in *compactorv1.GetCompactionRequest, opts ...grpc.CallOption) (*compactorv1.GetCompactionResponse, error) {
	return invoke(ctx, c, func(ctx context.Context, instance instance) (*compactorv1.GetCompactionResponse, error) {
		return instance.GetCompactionJobs(ctx, in, opts...)
	}, func(r *compactorv1.GetCompactionResponse) *metastorev1.RaftStatus {
		return nil // todo
	})
}
