package querier

import (
	"context"
	"fmt"

	"connectrpc.com/connect"
	"github.com/grafana/dskit/ring"
	ring_client "github.com/grafana/dskit/ring/client"
	"github.com/opentracing/opentracing-go"
	otlog "github.com/opentracing/opentracing-go/log"
	"github.com/prometheus/prometheus/promql/parser"
	"golang.org/x/sync/errgroup"

	googlev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	ingesterv1 "github.com/grafana/pyroscope/api/gen/proto/go/ingester/v1"
	ingestv1 "github.com/grafana/pyroscope/api/gen/proto/go/ingester/v1"
	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	"github.com/grafana/pyroscope/pkg/clientpool"
	phlaremodel "github.com/grafana/pyroscope/pkg/model"
	"github.com/grafana/pyroscope/pkg/util"
)

type IngesterQueryClient interface {
	LabelValues(context.Context, *connect.Request[typesv1.LabelValuesRequest]) (*connect.Response[typesv1.LabelValuesResponse], error)
	LabelNames(context.Context, *connect.Request[typesv1.LabelNamesRequest]) (*connect.Response[typesv1.LabelNamesResponse], error)
	ProfileTypes(context.Context, *connect.Request[ingestv1.ProfileTypesRequest]) (*connect.Response[ingestv1.ProfileTypesResponse], error)
	Series(ctx context.Context, req *connect.Request[ingestv1.SeriesRequest]) (*connect.Response[ingestv1.SeriesResponse], error)
	MergeProfilesStacktraces(context.Context) clientpool.BidiClientMergeProfilesStacktraces
	MergeProfilesLabels(ctx context.Context) clientpool.BidiClientMergeProfilesLabels
	MergeProfilesPprof(ctx context.Context) clientpool.BidiClientMergeProfilesPprof
	MergeSpanProfile(ctx context.Context) clientpool.BidiClientMergeSpanProfile
	BlockMetadata(ctx context.Context, req *connect.Request[ingestv1.BlockMetadataRequest]) (*connect.Response[ingestv1.BlockMetadataResponse], error)
	GetProfileStats(ctx context.Context, req *connect.Request[typesv1.GetProfileStatsRequest]) (*connect.Response[typesv1.GetProfileStatsResponse], error)
	GetBlockStats(ctx context.Context, req *connect.Request[ingestv1.GetBlockStatsRequest]) (*connect.Response[ingestv1.GetBlockStatsResponse], error)
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

// readNoExtend is a ring.Operation that only selects instances marked as ring.ACTIVE.
// This should mirror the operation used when choosing ingesters to write series to (ring.WriteNoExtend).
var readNoExtend = ring.NewOp([]ring.InstanceState{ring.ACTIVE}, nil)

// forAllIngesters runs f, in parallel, for all ingesters
func forAllIngesters[T any](ctx context.Context, ingesterQuerier *IngesterQuerier, f QueryReplicaFn[T, IngesterQueryClient]) ([]ResponseFromReplica[T], error) {
	replicationSet, err := ingesterQuerier.ring.GetReplicationSetForOperation(readNoExtend)
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

// forAllPlannedIngesters runs f, in parallel, for all ingesters part of the plan
func forAllPlannedIngesters[T any](ctx context.Context, ingesterQuerier *IngesterQuerier, plan blockPlan, f QueryReplicaWithHintsFn[T, IngesterQueryClient]) ([]ResponseFromReplica[T], error) {
	replicationSet, err := ingesterQuerier.ring.GetReplicationSetForOperation(readNoExtend)
	if err != nil {
		return nil, err
	}

	return forGivenPlan(ctx, plan, func(addr string) (IngesterQueryClient, error) {
		client, err := ingesterQuerier.pool.GetClientFor(addr)
		if err != nil {
			return nil, err
		}
		return client.(IngesterQueryClient), nil
	}, replicationSet, f)
}

func (q *Querier) selectTreeFromIngesters(ctx context.Context, req *querierv1.SelectMergeStacktracesRequest, plan blockPlan) (*phlaremodel.Tree, error) {
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

	var responses []ResponseFromReplica[clientpool.BidiClientMergeProfilesStacktraces]
	if plan != nil {
		responses, err = forAllPlannedIngesters(ctx, q.ingesterQuerier, plan, func(ctx context.Context, ic IngesterQueryClient, hints *ingestv1.Hints) (clientpool.BidiClientMergeProfilesStacktraces, error) {
			return ic.MergeProfilesStacktraces(ctx), nil
		})
	} else {
		responses, err = forAllIngesters(ctx, q.ingesterQuerier, func(ctx context.Context, ic IngesterQueryClient) (clientpool.BidiClientMergeProfilesStacktraces, error) {
			return ic.MergeProfilesStacktraces(ctx), nil
		})
	}
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	// send the first initial request to all ingesters.
	g, gCtx := errgroup.WithContext(ctx)
	for idx := range responses {
		r := responses[idx]
		hints, ok := plan[r.addr]
		if !ok && plan != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("no hints found for replica %s", r.addr))
		}

		g.Go(util.RecoverPanic(func() error {
			return r.response.Send(&ingestv1.MergeProfilesStacktracesRequest{
				Request: &ingestv1.SelectProfilesRequest{
					LabelSelector: req.LabelSelector,
					Start:         req.Start,
					End:           req.End,
					Type:          profileType,
					Hints:         &ingestv1.Hints{Block: hints},
				},
				MaxNodes: req.MaxNodes,
			})
		}))
	}
	if err = g.Wait(); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	// merge all profiles
	return selectMergeTree(gCtx, responses)
}

func (q *Querier) selectProfileFromIngesters(ctx context.Context, req *querierv1.SelectMergeProfileRequest, plan blockPlan) (*googlev1.Profile, error) {
	span, ctx := opentracing.StartSpanFromContext(ctx, "SelectProfile Ingesters")
	defer span.Finish()
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

	var responses []ResponseFromReplica[clientpool.BidiClientMergeProfilesPprof]
	if plan != nil {
		responses, err = forAllPlannedIngesters(ctx, q.ingesterQuerier, plan, func(ctx context.Context, ic IngesterQueryClient, hints *ingestv1.Hints) (clientpool.BidiClientMergeProfilesPprof, error) {
			return ic.MergeProfilesPprof(ctx), nil
		})
	} else {
		responses, err = forAllIngesters(ctx, q.ingesterQuerier, func(ctx context.Context, ic IngesterQueryClient) (clientpool.BidiClientMergeProfilesPprof, error) {
			return ic.MergeProfilesPprof(ctx), nil
		})
	}
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	// send the first initial request to all ingesters.
	g, gCtx := errgroup.WithContext(ctx)
	for idx := range responses {
		r := responses[idx]
		hints, ok := plan[r.addr]
		if !ok && plan != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("no hints found for replica %s", r.addr))
		}

		g.Go(util.RecoverPanic(func() error {
			return r.response.Send(&ingestv1.MergeProfilesPprofRequest{
				Request: &ingestv1.SelectProfilesRequest{
					LabelSelector: req.LabelSelector,
					Start:         req.Start,
					End:           req.End,
					Type:          profileType,
					Hints:         &ingestv1.Hints{Block: hints},
				},
				MaxNodes:           req.MaxNodes,
				StackTraceSelector: req.StackTraceSelector,
			})
		}))
	}
	if err = g.Wait(); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	// merge all profiles
	span.LogFields(otlog.String("msg", "selectMergePprofProfile"))
	return selectMergePprofProfile(gCtx, profileType, responses)
}

func (q *Querier) selectSeriesFromIngesters(ctx context.Context, req *ingesterv1.MergeProfilesLabelsRequest, plan map[string]*ingestv1.BlockHints) ([]ResponseFromReplica[clientpool.BidiClientMergeProfilesLabels], error) {
	sp, ctx := opentracing.StartSpanFromContext(ctx, "SelectSeries Ingesters")
	defer sp.Finish()
	var responses []ResponseFromReplica[clientpool.BidiClientMergeProfilesLabels]
	var err error

	if plan != nil {
		responses, err = forAllPlannedIngesters(ctx, q.ingesterQuerier, plan, func(ctx context.Context, q IngesterQueryClient, hint *ingestv1.Hints) (clientpool.BidiClientMergeProfilesLabels, error) {
			return q.MergeProfilesLabels(ctx), nil
		})
	} else {
		responses, err = forAllIngesters(ctx, q.ingesterQuerier, func(ctx context.Context, ic IngesterQueryClient) (clientpool.BidiClientMergeProfilesLabels, error) {
			return ic.MergeProfilesLabels(ctx), nil
		})
	}
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	// send the first initial request to all ingesters.
	g, _ := errgroup.WithContext(ctx)
	for _, r := range responses {
		r := r
		hints, ok := plan[r.addr]
		if !ok && plan != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("no hints found for replica %s", r.addr))
		}
		g.Go(util.RecoverPanic(func() error {
			req := req.CloneVT()
			req.Request.Hints = &ingestv1.Hints{Block: hints}
			return r.response.Send(req)
		}))
	}
	if err := g.Wait(); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return responses, nil
}

func (q *Querier) labelValuesFromIngesters(ctx context.Context, req *typesv1.LabelValuesRequest) ([]ResponseFromReplica[[]string], error) {
	sp, ctx := opentracing.StartSpanFromContext(ctx, "LabelValues Ingesters")
	defer sp.Finish()

	responses, err := forAllIngesters(ctx, q.ingesterQuerier, func(childCtx context.Context, ic IngesterQueryClient) ([]string, error) {
		res, err := ic.LabelValues(childCtx, connect.NewRequest(req))
		if err != nil {
			return nil, err
		}
		return res.Msg.Names, nil
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return responses, nil
}

func (q *Querier) labelNamesFromIngesters(ctx context.Context, req *typesv1.LabelNamesRequest) ([]ResponseFromReplica[[]string], error) {
	sp, ctx := opentracing.StartSpanFromContext(ctx, "LabelNames Ingesters")
	defer sp.Finish()

	responses, err := forAllIngesters(ctx, q.ingesterQuerier, func(childCtx context.Context, ic IngesterQueryClient) ([]string, error) {
		res, err := ic.LabelNames(childCtx, connect.NewRequest(req))
		if err != nil {
			return nil, err
		}
		return res.Msg.Names, nil
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return responses, nil
}

func (q *Querier) seriesFromIngesters(ctx context.Context, req *ingesterv1.SeriesRequest) ([]ResponseFromReplica[[]*typesv1.Labels], error) {
	sp, ctx := opentracing.StartSpanFromContext(ctx, "Series Ingesters")
	defer sp.Finish()

	responses, err := forAllIngesters(ctx, q.ingesterQuerier, func(childCtx context.Context, ic IngesterQueryClient) ([]*typesv1.Labels, error) {
		res, err := ic.Series(childCtx, connect.NewRequest(&ingestv1.SeriesRequest{
			Matchers:   req.Matchers,
			LabelNames: req.LabelNames,
			Start:      req.Start,
			End:        req.End,
		}))
		if err != nil {
			return nil, err
		}
		return res.Msg.LabelsSet, nil
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return responses, nil
}

func (q *Querier) selectSpanProfileFromIngesters(ctx context.Context, req *querierv1.SelectMergeSpanProfileRequest, plan map[string]*ingestv1.BlockHints) (*phlaremodel.Tree, error) {
	sp, ctx := opentracing.StartSpanFromContext(ctx, "SelectMergeSpanProfile Ingesters")
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

	var responses []ResponseFromReplica[clientpool.BidiClientMergeSpanProfile]
	if plan != nil {
		responses, err = forAllPlannedIngesters(ctx, q.ingesterQuerier, plan, func(ctx context.Context, ic IngesterQueryClient, hints *ingestv1.Hints) (clientpool.BidiClientMergeSpanProfile, error) {
			return ic.MergeSpanProfile(ctx), nil
		})
	} else {
		responses, err = forAllIngesters(ctx, q.ingesterQuerier, func(ctx context.Context, ic IngesterQueryClient) (clientpool.BidiClientMergeSpanProfile, error) {
			return ic.MergeSpanProfile(ctx), nil
		})
	}
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	// send the first initial request to all ingesters.
	g, gCtx := errgroup.WithContext(ctx)
	for idx := range responses {
		r := responses[idx]
		hints, ok := plan[r.addr]
		if !ok && plan != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("no hints found for replica %s", r.addr))
		}

		g.Go(util.RecoverPanic(func() error {
			return r.response.Send(&ingestv1.MergeSpanProfileRequest{
				Request: &ingestv1.SelectSpanProfileRequest{
					LabelSelector: req.LabelSelector,
					Start:         req.Start,
					End:           req.End,
					Type:          profileType,
					SpanSelector:  req.SpanSelector,
					Hints:         &ingestv1.Hints{Block: hints},
				},
				MaxNodes: req.MaxNodes,
			})
		}))
	}
	if err = g.Wait(); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	// merge all profiles
	return selectMergeSpanProfile(gCtx, responses)
}

func (q *Querier) blockSelectFromIngesters(ctx context.Context, req *ingestv1.BlockMetadataRequest) ([]ResponseFromReplica[[]*typesv1.BlockInfo], error) {
	sp, ctx := opentracing.StartSpanFromContext(ctx, "blockSelectFromIngesters")
	defer sp.Finish()

	responses, err := forAllIngesters(ctx, q.ingesterQuerier, func(childCtx context.Context, ic IngesterQueryClient) ([]*typesv1.BlockInfo, error) {
		res, err := ic.BlockMetadata(childCtx, connect.NewRequest(&ingestv1.BlockMetadataRequest{
			Start: req.Start,
			End:   req.End,
		}))
		if err != nil {
			return nil, err
		}
		return res.Msg.Blocks, nil
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return responses, nil
}
