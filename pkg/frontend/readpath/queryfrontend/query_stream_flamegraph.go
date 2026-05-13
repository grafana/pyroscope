package queryfrontend

import (
	"context"
	"io"
	"time"

	"connectrpc.com/connect"
	"github.com/grafana/dskit/tenant"

	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	phlaremodel "github.com/grafana/pyroscope/v2/pkg/model"
	"github.com/grafana/pyroscope/v2/pkg/querybackend/queryplan"
	"github.com/grafana/pyroscope/v2/pkg/validation"
)

// SelectMergeStacktracesStream implements QuerierStreamServiceHandler.
func (q *streamFrontend) SelectMergeStacktracesStream(
	ctx context.Context,
	req *connect.Request[querierv1.SelectMergeStacktracesRequest],
	stream *connect.ServerStream[querierv1.FlamegraphStreamEvent],
) error {
	tenantIDs, err := tenant.TenantIDs(ctx)
	if err != nil {
		return connect.NewError(connect.CodeInvalidArgument, err)
	}
	empty, err := validation.SanitizeTimeRange(q.limits, tenantIDs, &req.Msg.Start, &req.Msg.End)
	if err != nil {
		return connect.NewError(connect.CodeInvalidArgument, err)
	}
	if empty {
		return stream.Send(&querierv1.FlamegraphStreamEvent{
			Kind: &querierv1.FlamegraphStreamEvent_Flamegraph{Flamegraph: new(querierv1.FlameGraph)},
		})
	}

	maxNodes, err := validation.ValidateMaxNodes(q.limits, tenantIDs, req.Msg.GetMaxNodes())
	if err != nil {
		return connect.NewError(connect.CodeInvalidArgument, err)
	}

	_, err = phlaremodel.ParseProfileTypeSelector(req.Msg.ProfileTypeID)
	if err != nil {
		return connect.NewError(connect.CodeInvalidArgument, err)
	}

	labelSelector, err := buildLabelSelectorWithProfileType(req.Msg.LabelSelector, req.Msg.ProfileTypeID)
	if err != nil {
		return connect.NewError(connect.CodeInvalidArgument, err)
	}

	blocks, err := q.QueryMetadata(ctx, &queryv1.QueryRequest{
		StartTime:     req.Msg.Start,
		EndTime:       req.Msg.End,
		LabelSelector: labelSelector,
		Query: []*queryv1.Query{{
			QueryType: queryv1.QueryType_QUERY_TREE,
			Tree: &queryv1.TreeQuery{
				MaxNodes:           maxNodes,
				StackTraceSelector: req.Msg.StackTraceSelector,
				ProfileIdSelector:  req.Msg.ProfileIdSelector,
			},
		}},
	})
	if err != nil {
		return err
	}
	if len(blocks) == 0 {
		return stream.Send(&querierv1.FlamegraphStreamEvent{
			Kind: &querierv1.FlamegraphStreamEvent_Flamegraph{Flamegraph: new(querierv1.FlameGraph)},
		})
	}

	xrandMutex.Lock()
	xrand.Shuffle(len(blocks), func(i, j int) { blocks[i], blocks[j] = blocks[j], blocks[i] })
	xrandMutex.Unlock()
	p := queryplan.Build(blocks, 4, 20)

	backendStream, err := q.querybackend.InvokeStream(ctx, &queryv1.InvokeRequest{
		Tenant:        tenantIDs,
		StartTime:     req.Msg.Start,
		EndTime:       req.Msg.End,
		LabelSelector: labelSelector,
		Options: &queryv1.InvokeOptions{
			SanitizeOnMerge: q.limits.QuerySanitizeOnMerge(tenantIDs[0]),
		},
		QueryPlan: p,
		Query: []*queryv1.Query{{
			QueryType: queryv1.QueryType_QUERY_TREE,
			Tree: &queryv1.TreeQuery{
				MaxNodes:           maxNodes,
				StackTraceSelector: req.Msg.StackTraceSelector,
				ProfileIdSelector:  req.Msg.ProfileIdSelector,
			},
		}},
	})
	if err != nil {
		return connect.NewError(connect.CodeInternal, err)
	}

	startTime := time.Now()
	var (
		bytesTotalEstimate uint64
		bytesDone          uint64
		latestFlamegraph   *querierv1.FlameGraph
		hasProgress        bool
	)

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	evCh := make(chan streamRecv, 16)
	go recvLoop(backendStream.Recv, evCh, ctx.Done())

	sendProgress := func() error {
		if !hasProgress {
			return nil
		}
		return stream.Send(&querierv1.FlamegraphStreamEvent{
			Kind: &querierv1.FlamegraphStreamEvent_Progress{
				Progress: &querierv1.FlamegraphProgress{
					BytesTotalEstimate: bytesTotalEstimate,
					BytesDone:         bytesDone,
					EtaUnixMs:         computeETA(startTime, bytesDone, bytesTotalEstimate),
					Flamegraph:        latestFlamegraph,
				},
			},
		})
	}

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case r := <-evCh:
			if r.err == io.EOF {
				return nil
			}
			if r.err != nil {
				return connect.NewError(connect.CodeInternal, r.err)
			}
			switch ev := r.ev.Event.(type) {
			case *queryv1.InvokeStreamEvent_IndexLookup:
				bytesTotalEstimate += ev.IndexLookup.BytesEstimate
				hasProgress = true
			case *queryv1.InvokeStreamEvent_Snapshot:
				bytesDone = ev.Snapshot.BytesDone
				if tr := findTreeReport(ev.Snapshot.Reports); tr != nil {
					if t, e := phlaremodel.UnmarshalTree[phlaremodel.FunctionName, phlaremodel.FunctionNameI](tr.Tree); e == nil {
						latestFlamegraph = phlaremodel.NewFlameGraph(t, req.Msg.GetMaxNodes())
					}
				}
				hasProgress = true
			case *queryv1.InvokeStreamEvent_Terminal:
				ticker.Stop()
				var fg *querierv1.FlameGraph
				if tr := findTreeReport(ev.Terminal.Reports); tr != nil {
					if t, e := phlaremodel.UnmarshalTree[phlaremodel.FunctionName, phlaremodel.FunctionNameI](tr.Tree); e == nil {
						fg = phlaremodel.NewFlameGraph(t, req.Msg.GetMaxNodes())
					}
				}
				return stream.Send(&querierv1.FlamegraphStreamEvent{
					Kind: &querierv1.FlamegraphStreamEvent_Flamegraph{Flamegraph: fg},
				})
			}
		case <-ticker.C:
			if err := sendProgress(); err != nil {
				return err
			}
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}
