package querier

import (
	"context"
	"time"

	"github.com/bufbuild/connect-go"
	"github.com/grafana/dskit/ring"
	ring_client "github.com/grafana/dskit/ring/client"

	ingestv1 "github.com/grafana/phlare/api/gen/proto/go/ingester/v1"
	"github.com/grafana/phlare/pkg/ingester/clientpool"
)

type IngesterQueryClient interface {
	LabelValues(context.Context, *connect.Request[ingestv1.LabelValuesRequest]) (*connect.Response[ingestv1.LabelValuesResponse], error)
	LabelNames(context.Context, *connect.Request[ingestv1.LabelNamesRequest]) (*connect.Response[ingestv1.LabelNamesResponse], error)
	ProfileTypes(context.Context, *connect.Request[ingestv1.ProfileTypesRequest]) (*connect.Response[ingestv1.ProfileTypesResponse], error)
	Series(ctx context.Context, req *connect.Request[ingestv1.SeriesRequest]) (*connect.Response[ingestv1.SeriesResponse], error)
	MergeProfilesStacktraces(context.Context) clientpool.BidiClientMergeProfilesStacktraces
	MergeProfilesLabels(ctx context.Context) clientpool.BidiClientMergeProfilesLabels
	MergeProfilesPprof(ctx context.Context) clientpool.BidiClientMergeProfilesPprof
}

type responseFromIngesters[T interface{}] struct {
	addr     string
	response T
}

type IngesterFn[T interface{}] func(context.Context, IngesterQueryClient) (T, error)

// IngesterQuerier helps with querying the ingesters.
type IngesterQuerier struct {
	ring            ring.ReadRing
	pool            *ring_client.Pool
	extraQueryDelay time.Duration
}

func NewIngesterQuerier(pool *ring_client.Pool, ring ring.ReadRing, extraQueryDelay time.Duration) *IngesterQuerier {
	return &IngesterQuerier{
		ring:            ring,
		pool:            pool,
		extraQueryDelay: extraQueryDelay,
	}
}

// forAllIngesters runs f, in parallel, for all ingesters
func forAllIngesters[T any](ctx context.Context, q *IngesterQuerier, f IngesterFn[T]) ([]responseFromIngesters[T], error) {
	replicationSet, err := q.ring.GetReplicationSetForOperation(ring.Read)
	if err != nil {
		return nil, err
	}

	return forGivenIngesters(ctx, q, replicationSet, f)
}

// forGivenIngesters runs f, in parallel, for given ingesters
func forGivenIngesters[T any](ctx context.Context, q *IngesterQuerier, replicationSet ring.ReplicationSet, f IngesterFn[T]) ([]responseFromIngesters[T], error) {
	results, err := replicationSet.Do(ctx, q.extraQueryDelay, func(ctx context.Context, ingester *ring.InstanceDesc) (interface{}, error) {
		client, err := q.pool.GetClientFor(ingester.Addr)
		if err != nil {
			return nil, err
		}

		resp, err := f(ctx, client.(IngesterQueryClient))
		if err != nil {
			return nil, err
		}

		return responseFromIngesters[T]{ingester.Addr, resp}, nil
	})
	if err != nil {
		return nil, err
	}

	responses := make([]responseFromIngesters[T], 0, len(results))
	for _, result := range results {
		responses = append(responses, result.(responseFromIngesters[T]))
	}

	return responses, err
}
