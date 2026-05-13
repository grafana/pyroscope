package queryfrontend

import (
	"context"
	"io"
	"time"

	"connectrpc.com/connect"
	"github.com/grafana/dskit/tenant"
	"github.com/pkg/errors"

	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	phlaremodel "github.com/grafana/pyroscope/v2/pkg/model"
	"github.com/grafana/pyroscope/v2/pkg/querybackend/queryplan"
	"github.com/grafana/pyroscope/v2/pkg/validation"
)

// SelectSeriesStream implements QuerierServiceHandler. The Router calls
// StreamSelectSeries directly via type assertion (inProcessStreamer), but this
// method allows QueryFrontend to be registered as a handler directly.
func (q *QueryFrontend) SelectSeriesStream(ctx context.Context, req *connect.Request[querierv1.SelectSeriesRequest], stream *connect.ServerStream[querierv1.SelectSeriesPartial]) error {
	return q.StreamSelectSeries(ctx, req.Msg, stream)
}

// StreamSelectSeries implements inProcessStreamer. It is called by the Router
// when it detects an in-process frontend.
func (q *QueryFrontend) StreamSelectSeries(
	ctx context.Context,
	req *querierv1.SelectSeriesRequest,
	stream *connect.ServerStream[querierv1.SelectSeriesPartial],
) error {
	tenantIDs, err := tenant.TenantIDs(ctx)
	if err != nil {
		return connect.NewError(connect.CodeInvalidArgument, err)
	}
	empty, err := validation.SanitizeTimeRange(q.limits, tenantIDs, &req.Start, &req.End)
	if err != nil {
		return connect.NewError(connect.CodeInvalidArgument, err)
	}
	if empty {
		return stream.Send(&querierv1.SelectSeriesPartial{
			Kind: &querierv1.SelectSeriesPartial_Result{
				Result: &querierv1.QueryResult{},
			},
		})
	}

	_, err = phlaremodel.ParseProfileTypeSelector(req.ProfileTypeID)
	if err != nil {
		return connect.NewError(connect.CodeInvalidArgument, err)
	}

	// Sub-millisecond step values truncate to 0 in the backend's millisecond
	// arithmetic and would cause an unbounded loop in RangeSeries; reject
	// anything below 1ms.
	if req.Step < 0.001 {
		return connect.NewError(connect.CodeInvalidArgument, errors.New("step must be >= 1ms"))
	}

	stepMs := time.Duration(req.Step * float64(time.Second)).Milliseconds()
	start := req.Start - stepMs

	labelSelector, err := buildLabelSelectorWithProfileType(req.LabelSelector, req.ProfileTypeID)
	if err != nil {
		return connect.NewError(connect.CodeInvalidArgument, err)
	}

	blocks, err := q.QueryMetadata(ctx, &queryv1.QueryRequest{
		StartTime:     start,
		EndTime:       req.End,
		LabelSelector: labelSelector,
		Query: []*queryv1.Query{{
			QueryType: queryv1.QueryType_QUERY_TIME_SERIES,
			TimeSeries: &queryv1.TimeSeriesQuery{
				Step:         req.GetStep(),
				GroupBy:      req.GetGroupBy(),
				Limit:        req.GetLimit(),
				ExemplarType: req.GetExemplarType(),
			},
		}},
	})
	if err != nil {
		return err
	}
	if len(blocks) == 0 {
		return stream.Send(&querierv1.SelectSeriesPartial{
			Kind: &querierv1.SelectSeriesPartial_Result{
				Result: &querierv1.QueryResult{},
			},
		})
	}

	xrandMutex.Lock()
	xrand.Shuffle(len(blocks), func(i, j int) {
		blocks[i], blocks[j] = blocks[j], blocks[i]
	})
	xrandMutex.Unlock()
	p := queryplan.Build(blocks, 4, 20)

	tenants, err := tenant.TenantIDs(ctx)
	if err != nil {
		return connect.NewError(connect.CodeInvalidArgument, err)
	}

	backendStream, err := q.querybackend.InvokeStream(ctx, &queryv1.InvokeRequest{
		Tenant:        tenants,
		StartTime:     start,
		EndTime:       req.End,
		LabelSelector: labelSelector,
		Options: &queryv1.InvokeOptions{
			SanitizeOnMerge: q.limits.QuerySanitizeOnMerge(tenants[0]),
		},
		QueryPlan: p,
		Query: []*queryv1.Query{{
			QueryType: queryv1.QueryType_QUERY_TIME_SERIES,
			TimeSeries: &queryv1.TimeSeriesQuery{
				Step:         req.GetStep(),
				GroupBy:      req.GetGroupBy(),
				Limit:        req.GetLimit(),
				ExemplarType: req.GetExemplarType(),
			},
		}},
	})
	if err != nil {
		return connect.NewError(connect.CodeInternal, err)
	}

	blocksTotal := uint32(len(blocks))
	var datasetsTotal uint32

	for {
		event, recvErr := backendStream.Recv()
		if recvErr == io.EOF {
			break
		}
		if recvErr != nil {
			return connect.NewError(connect.CodeInternal, recvErr)
		}
		switch ev := event.Event.(type) {
		case *queryv1.InvokeStreamEvent_IndexLookup:
			datasetsTotal += ev.IndexLookup.DatasetsFound
			if sendErr := stream.Send(&querierv1.SelectSeriesPartial{
				Kind: &querierv1.SelectSeriesPartial_PlanUpdate{
					PlanUpdate: &querierv1.QueryPlanUpdate{
						BlocksTotal:        blocksTotal,
						DatasetsTotal:      datasetsTotal,
						BytesTotalEstimate: ev.IndexLookup.BytesEstimate,
					},
				},
			}); sendErr != nil {
				return sendErr
			}
		case *queryv1.InvokeStreamEvent_Snapshot:
			tsReport := findTimeSeriesReport(ev.Snapshot.Reports)
			if tsReport == nil {
				continue
			}
			if sendErr := stream.Send(&querierv1.SelectSeriesPartial{
				Kind: &querierv1.SelectSeriesPartial_Chunk{
					Chunk: &querierv1.SeriesChunk{
						BlocksDone:   ev.Snapshot.BlocksDone,
						DatasetsDone: ev.Snapshot.DatasetsDone,
						BytesDone:    ev.Snapshot.BytesDone,
						Series:       tsReport.TimeSeries,
					},
				},
			}); sendErr != nil {
				return sendErr
			}
		case *queryv1.InvokeStreamEvent_Terminal:
			tsReport := findTimeSeriesReport(ev.Terminal.Reports)
			var series []*typesv1.Series
			if tsReport != nil {
				series = tsReport.TimeSeries
			}
			return stream.Send(&querierv1.SelectSeriesPartial{
				Kind: &querierv1.SelectSeriesPartial_Result{
					Result: &querierv1.QueryResult{Series: series},
				},
			})
		}
	}
	return nil
}

func findTimeSeriesReport(reports []*queryv1.Report) *queryv1.TimeSeriesReport {
	for _, r := range reports {
		if r.GetTimeSeries() != nil {
			return r.GetTimeSeries()
		}
	}
	return nil
}
