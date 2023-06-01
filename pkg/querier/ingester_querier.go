package querier

import (
	"context"

	"github.com/bufbuild/connect-go"
	"github.com/grafana/dskit/ring"
	ring_client "github.com/grafana/dskit/ring/client"
	"github.com/opentracing/opentracing-go"
	"github.com/prometheus/prometheus/promql/parser"
	"golang.org/x/sync/errgroup"

	ingesterv1 "github.com/grafana/phlare/api/gen/proto/go/ingester/v1"
	ingestv1 "github.com/grafana/phlare/api/gen/proto/go/ingester/v1"
	querierv1 "github.com/grafana/phlare/api/gen/proto/go/querier/v1"
	typesv1 "github.com/grafana/phlare/api/gen/proto/go/types/v1"
	"github.com/grafana/phlare/pkg/clientpool"
	phlaremodel "github.com/grafana/phlare/pkg/model"
	"github.com/grafana/phlare/pkg/util"
)

type IngesterQueryClient interface {
	LabelValues(context.Context, *connect.Request[typesv1.LabelValuesRequest]) (*connect.Response[typesv1.LabelValuesResponse], error)
	LabelNames(context.Context, *connect.Request[typesv1.LabelNamesRequest]) (*connect.Response[typesv1.LabelNamesResponse], error)
	ProfileTypes(context.Context, *connect.Request[ingestv1.ProfileTypesRequest]) (*connect.Response[ingestv1.ProfileTypesResponse], error)
	Series(ctx context.Context, req *connect.Request[ingestv1.SeriesRequest]) (*connect.Response[ingestv1.SeriesResponse], error)
	MergeProfilesStacktraces(context.Context) clientpool.BidiClientMergeProfilesStacktraces
	MergeProfilesLabels(ctx context.Context) clientpool.BidiClientMergeProfilesLabels
	MergeProfilesPprof(ctx context.Context) clientpool.BidiClientMergeProfilesPprof
}

// IngesterQuerier helps with querying the ingesters.
type IngesterQuerier struct {
	ring ring.ReadRing
	pool *ring_client.Pool
}

func NewIngesterQuerier(pool *ring_client.Pool, ring ring.ReadRing) *IngesterQuerier {
	return &IngesterQuerier{
		ring: ring,
		pool: pool,
	}
}

// forAllIngesters runs f, in parallel, for all ingesters
func forAllIngesters[T any](ctx context.Context, ingesterQuerier *IngesterQuerier, f QueryReplicaFn[T, IngesterQueryClient]) ([]ResponseFromReplica[T], error) {
	replicationSet, err := ingesterQuerier.ring.GetReplicationSetForOperation(ring.Read)
	if err != nil {
		return nil, err
	}
	return forGivenReplicationSet(ctx, func(addr string) (IngesterQueryClient, error) {
		client, err := ingesterQuerier.pool.GetClientFor(addr)
		if err != nil {
			return nil, err
		}
		return client.(IngesterQueryClient), nil
	}, replicationSet, f)
}

func (q *Querier) selectTreeFromIngesters(ctx context.Context, req *querierv1.SelectMergeStacktracesRequest) (*phlaremodel.Tree, error) {
	sp, ctx := opentracing.StartSpanFromContext(ctx, "SelectTree Ingesters")
	defer sp.Finish()
	profileType, err := phlaremodel.ParseProfileTypeSelector(req.ProfileTypeID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	_, err = parser.ParseMetricSelector(req.LabelSelector)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	responses, err := forAllIngesters(ctx, q.ingesterQuerier, func(ctx context.Context, ic IngesterQueryClient) (clientpool.BidiClientMergeProfilesStacktraces, error) {
		return ic.MergeProfilesStacktraces(ctx), nil
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	// send the first initial request to all ingesters.
	g, gCtx := errgroup.WithContext(ctx)
	for _, r := range responses {
		r := r
		g.Go(util.RecoverPanic(func() error {
			return r.response.Send(&ingestv1.MergeProfilesStacktracesRequest{
				Request: &ingestv1.SelectProfilesRequest{
					LabelSelector: req.LabelSelector,
					Start:         req.Start,
					End:           req.End,
					Type:          profileType,
				},
				MaxNodes: req.MaxNodes,
				// TODO(kolesnikovae): Max stacks.
			})
		}))
	}
	if err = g.Wait(); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	// merge all profiles
	return selectMergeTree(gCtx, responses)
}

func (q *Querier) selectSeriesFromIngesters(ctx context.Context, req *ingesterv1.MergeProfilesLabelsRequest) ([]ResponseFromReplica[clientpool.BidiClientMergeProfilesLabels], error) {
	sp, ctx := opentracing.StartSpanFromContext(ctx, "SelectSeries Ingesters")
	defer sp.Finish()
	responses, err := forAllIngesters(ctx, q.ingesterQuerier, func(ctx context.Context, ic IngesterQueryClient) (clientpool.BidiClientMergeProfilesLabels, error) {
		return ic.MergeProfilesLabels(ctx), nil
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	// send the first initial request to all ingesters.
	g, _ := errgroup.WithContext(ctx)
	for _, r := range responses {
		r := r
		g.Go(util.RecoverPanic(func() error {
			return r.response.Send(req.CloneVT())
		}))
	}
	if err := g.Wait(); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return responses, nil
}
