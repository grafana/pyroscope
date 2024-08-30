package metastoreclient

import (
	"context"
	"fmt"
	compactorv1 "github.com/grafana/pyroscope/api/gen/proto/go/compactor/v1"
	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"google.golang.org/grpc"
)

type instance interface {
	metastorev1.MetastoreServiceClient
	compactorv1.CompactionPlannerClient
}

func invoke[R any](cl *Client, f func(instance instance) (*R, error)) (*R, error) {
	cl.mu.Lock()
	servers := cl.servers
	i := servers[cl.leader]
	cl.mu.Unlock()

	if i != nil {
		r, err := f(servers[cl.leader])
		if err == nil {
			return r, nil
		}
		return nil, fmt.Errorf("TODO implement")
	}
	return nil, fmt.Errorf("TODO implement")

}

func (c *Client) AddBlock(ctx context.Context, in *metastorev1.AddBlockRequest, opts ...grpc.CallOption) (*metastorev1.AddBlockResponse, error) {
	return invoke(c, func(instance instance) (*metastorev1.AddBlockResponse, error) {
		return instance.AddBlock(ctx, in, opts...)
	})
}

func (c *Client) QueryMetadata(ctx context.Context, in *metastorev1.QueryMetadataRequest, opts ...grpc.CallOption) (*metastorev1.QueryMetadataResponse, error) {
	return invoke(c, func(instance instance) (*metastorev1.QueryMetadataResponse, error) {
		return instance.QueryMetadata(ctx, in, opts...)
	})
}

func (c *Client) ReadIndex(ctx context.Context, in *metastorev1.ReadIndexRequest, opts ...grpc.CallOption) (*metastorev1.ReadIndexResponse, error) {
	return invoke(c, func(instance instance) (*metastorev1.ReadIndexResponse, error) {
		return instance.ReadIndex(ctx, in, opts...)
	})
}

func (c *Client) PollCompactionJobs(ctx context.Context, in *compactorv1.PollCompactionJobsRequest, opts ...grpc.CallOption) (*compactorv1.PollCompactionJobsResponse, error) {
	return invoke(c, func(instance instance) (*compactorv1.PollCompactionJobsResponse, error) {
		return instance.PollCompactionJobs(ctx, in, opts...)
	})
}

func (c *Client) GetCompactionJobs(ctx context.Context, in *compactorv1.GetCompactionRequest, opts ...grpc.CallOption) (*compactorv1.GetCompactionResponse, error) {
	return invoke(c, func(instance instance) (*compactorv1.GetCompactionResponse, error) {
		return instance.GetCompactionJobs(ctx, in, opts...)
	})
}
