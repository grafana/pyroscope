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
	"github.com/grafana/pyroscope/v2/pkg/model"
	"github.com/grafana/pyroscope/v2/pkg/querybackend/queryplan"
	"github.com/grafana/pyroscope/v2/pkg/validation"
)

// SelectSeriesStream implements QuerierStreamServiceHandler.
func (q *streamFrontend) SelectSeriesStream(
	ctx context.Context,
	req *connect.Request[querierv1.SelectSeriesRequest],
	stream *connect.ServerStream[querierv1.SeriesStreamEvent],
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
		return stream.Send(&querierv1.SeriesStreamEvent{
			Kind: &querierv1.SeriesStreamEvent_Result{Result: new(querierv1.SeriesStreamResult)},
		})
	}

	_, err = model.ParseProfileTypeSelector(req.Msg.ProfileTypeID)
	if err != nil {
		return connect.NewError(connect.CodeInvalidArgument, err)
	}

	if req.Msg.Step < 0.001 {
		return connect.NewError(connect.CodeInvalidArgument, errors.New("step must be >= 1ms"))
	}

	stepMs := time.Duration(req.Msg.Step * float64(time.Second)).Milliseconds()
	start := req.Msg.Start - stepMs

	labelSelector, err := buildLabelSelectorWithProfileType(req.Msg.LabelSelector, req.Msg.ProfileTypeID)
	if err != nil {
		return connect.NewError(connect.CodeInvalidArgument, err)
	}

	blocks, err := q.QueryMetadata(ctx, &queryv1.QueryRequest{
		StartTime:     start,
		EndTime:       req.Msg.End,
		LabelSelector: labelSelector,
		Query: []*queryv1.Query{{
			QueryType: queryv1.QueryType_QUERY_TIME_SERIES,
			TimeSeries: &queryv1.TimeSeriesQuery{
				Step:         req.Msg.GetStep(),
				GroupBy:      req.Msg.GetGroupBy(),
				Limit:        req.Msg.GetLimit(),
				ExemplarType: req.Msg.GetExemplarType(),
			},
		}},
	})
	if err != nil {
		return err
	}
	if len(blocks) == 0 {
		return stream.Send(&querierv1.SeriesStreamEvent{
			Kind: &querierv1.SeriesStreamEvent_Result{Result: new(querierv1.SeriesStreamResult)},
		})
	}

	xrandMutex.Lock()
	xrand.Shuffle(len(blocks), func(i, j int) { blocks[i], blocks[j] = blocks[j], blocks[i] })
	xrandMutex.Unlock()
	p := queryplan.Build(blocks, 4, 20)

	backendStream, err := q.querybackend.InvokeStream(ctx, &queryv1.InvokeRequest{
		Tenant:        tenantIDs,
		StartTime:     start,
		EndTime:       req.Msg.End,
		LabelSelector: labelSelector,
		Options: &queryv1.InvokeOptions{
			SanitizeOnMerge: q.limits.QuerySanitizeOnMerge(tenantIDs[0]),
		},
		QueryPlan: p,
		Query: []*queryv1.Query{{
			QueryType: queryv1.QueryType_QUERY_TIME_SERIES,
			TimeSeries: &queryv1.TimeSeriesQuery{
				Step:         req.Msg.GetStep(),
				GroupBy:      req.Msg.GetGroupBy(),
				Limit:        req.Msg.GetLimit(),
				ExemplarType: req.Msg.GetExemplarType(),
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
		latestSeries       []*typesv1.Series
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
		return stream.Send(&querierv1.SeriesStreamEvent{
			Kind: &querierv1.SeriesStreamEvent_Progress{
				Progress: &querierv1.SeriesProgress{
					BytesTotalEstimate: bytesTotalEstimate,
					BytesDone:         bytesDone,
					EtaUnixMs:         computeETA(startTime, bytesDone, bytesTotalEstimate),
					Series:            latestSeries,
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
				if tr := findTimeSeriesReport(ev.Snapshot.Reports); tr != nil {
					latestSeries = tr.TimeSeries
				}
				hasProgress = true
			case *queryv1.InvokeStreamEvent_Terminal:
				ticker.Stop()
				var series []*typesv1.Series
				if tr := findTimeSeriesReport(ev.Terminal.Reports); tr != nil {
					series = tr.TimeSeries
				}
				return stream.Send(&querierv1.SeriesStreamEvent{
					Kind: &querierv1.SeriesStreamEvent_Result{
						Result: &querierv1.SeriesStreamResult{Series: series},
					},
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
