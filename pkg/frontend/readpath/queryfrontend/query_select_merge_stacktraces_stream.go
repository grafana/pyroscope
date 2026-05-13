package queryfrontend

import (
	"context"
	"io"

	"connectrpc.com/connect"
	"github.com/grafana/dskit/tenant"

	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	phlaremodel "github.com/grafana/pyroscope/v2/pkg/model"
	"github.com/grafana/pyroscope/v2/pkg/querybackend/queryplan"
	"github.com/grafana/pyroscope/v2/pkg/validation"
)

// SelectMergeStacktracesStream implements QuerierServiceHandler. The Router
// calls StreamSelectMergeStacktraces directly via type assertion (inProcessStreamer),
// but this method allows QueryFrontend to be registered as a handler directly.
func (q *QueryFrontend) SelectMergeStacktracesStream(ctx context.Context, req *connect.Request[querierv1.SelectMergeStacktracesRequest], stream *connect.ServerStream[querierv1.SelectMergeStacktracesPartial]) error {
	return q.StreamSelectMergeStacktraces(ctx, req.Msg, stream)
}

// StreamSelectMergeStacktraces implements inProcessStreamer. It is called by
// the Router when it detects an in-process frontend.
func (q *QueryFrontend) StreamSelectMergeStacktraces(
	ctx context.Context,
	req *querierv1.SelectMergeStacktracesRequest,
	stream *connect.ServerStream[querierv1.SelectMergeStacktracesPartial],
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
		return stream.Send(&querierv1.SelectMergeStacktracesPartial{
			Kind: &querierv1.SelectMergeStacktracesPartial_Result{
				Result: &querierv1.QueryResult{},
			},
		})
	}

	maxNodes, err := validation.ValidateMaxNodes(q.limits, tenantIDs, req.GetMaxNodes())
	if err != nil {
		return connect.NewError(connect.CodeInvalidArgument, err)
	}

	_, err = phlaremodel.ParseProfileTypeSelector(req.ProfileTypeID)
	if err != nil {
		return connect.NewError(connect.CodeInvalidArgument, err)
	}

	labelSelector, err := buildLabelSelectorWithProfileType(req.LabelSelector, req.ProfileTypeID)
	if err != nil {
		return connect.NewError(connect.CodeInvalidArgument, err)
	}

	blocks, err := q.QueryMetadata(ctx, &queryv1.QueryRequest{
		StartTime:     req.Start,
		EndTime:       req.End,
		LabelSelector: labelSelector,
		Query: []*queryv1.Query{{
			QueryType: queryv1.QueryType_QUERY_TREE,
			Tree: &queryv1.TreeQuery{
				MaxNodes:           maxNodes,
				StackTraceSelector: req.StackTraceSelector,
				ProfileIdSelector:  req.ProfileIdSelector,
			},
		}},
	})
	if err != nil {
		return err
	}
	if len(blocks) == 0 {
		return stream.Send(&querierv1.SelectMergeStacktracesPartial{
			Kind: &querierv1.SelectMergeStacktracesPartial_Result{
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
		StartTime:     req.Start,
		EndTime:       req.End,
		LabelSelector: labelSelector,
		Options: &queryv1.InvokeOptions{
			SanitizeOnMerge: q.limits.QuerySanitizeOnMerge(tenants[0]),
		},
		QueryPlan: p,
		Query: []*queryv1.Query{{
			QueryType: queryv1.QueryType_QUERY_TREE,
			Tree: &queryv1.TreeQuery{
				MaxNodes:           maxNodes,
				StackTraceSelector: req.StackTraceSelector,
				ProfileIdSelector:  req.ProfileIdSelector,
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
			if sendErr := stream.Send(&querierv1.SelectMergeStacktracesPartial{
				Kind: &querierv1.SelectMergeStacktracesPartial_PlanUpdate{
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
			treeReport := findTreeReport(ev.Snapshot.Reports)
			if treeReport == nil {
				continue
			}
			t, unmarshalErr := phlaremodel.UnmarshalTree[phlaremodel.FunctionName, phlaremodel.FunctionNameI](treeReport.Tree)
			if unmarshalErr != nil {
				// Skip bad snapshot; final result is still sent in TerminalEvent.
				continue
			}
			fg := phlaremodel.NewFlameGraph(t, req.GetMaxNodes())
			if sendErr := stream.Send(&querierv1.SelectMergeStacktracesPartial{
				Kind: &querierv1.SelectMergeStacktracesPartial_Snapshot{
					Snapshot: &querierv1.QuerySnapshot{
						BlocksDone:   ev.Snapshot.BlocksDone,
						DatasetsDone: ev.Snapshot.DatasetsDone,
						BytesDone:    ev.Snapshot.BytesDone,
						Flamegraph:   fg,
					},
				},
			}); sendErr != nil {
				return sendErr
			}
		case *queryv1.InvokeStreamEvent_Terminal:
			treeReport := findTreeReport(ev.Terminal.Reports)
			var fg *querierv1.FlameGraph
			if treeReport != nil {
				t, unmarshalErr := phlaremodel.UnmarshalTree[phlaremodel.FunctionName, phlaremodel.FunctionNameI](treeReport.Tree)
				if unmarshalErr == nil {
					fg = phlaremodel.NewFlameGraph(t, req.GetMaxNodes())
				}
			}
			return stream.Send(&querierv1.SelectMergeStacktracesPartial{
				Kind: &querierv1.SelectMergeStacktracesPartial_Result{
					Result: &querierv1.QueryResult{Flamegraph: fg},
				},
			})
		}
	}
	return nil
}

func findTreeReport(reports []*queryv1.Report) *queryv1.TreeReport {
	for _, r := range reports {
		if r.GetTree() != nil {
			return r.GetTree()
		}
	}
	return nil
}
