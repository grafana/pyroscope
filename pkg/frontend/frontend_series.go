package frontend

import (
	"context"

	"connectrpc.com/connect"
	"github.com/go-kit/log/level"
	"github.com/grafana/dskit/tenant"
	"github.com/opentracing/opentracing-go"
	"github.com/prometheus/common/model"

	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
	"github.com/grafana/pyroscope/api/gen/proto/go/querier/v1/querierv1connect"
	querybackendv1 "github.com/grafana/pyroscope/api/gen/proto/go/querybackend/v1"
	phlaremodel "github.com/grafana/pyroscope/pkg/model"
	"github.com/grafana/pyroscope/pkg/util/connectgrpc"
	"github.com/grafana/pyroscope/pkg/validation"
)

func (f *Frontend) Series(ctx context.Context, c *connect.Request[querierv1.SeriesRequest]) (*connect.Response[querierv1.SeriesResponse], error) {
	opentracing.SpanFromContext(ctx).
		SetTag("start", model.Time(c.Msg.Start).Time().String()).
		SetTag("end", model.Time(c.Msg.End).Time().String()).
		SetTag("matchers", c.Msg.Matchers).
		SetTag("label_names", c.Msg.LabelNames)

	ctx = connectgrpc.WithProcedure(ctx, querierv1connect.QuerierServiceSeriesProcedure)
	tenantIDs, err := tenant.TenantIDs(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	interval, ok := phlaremodel.GetTimeRange(c.Msg)
	if ok {
		validated, err := validation.ValidateRangeRequest(f.limits, tenantIDs, interval, model.Now())
		if err != nil {
			return nil, connect.NewError(connect.CodeInvalidArgument, err)
		}
		if validated.IsEmpty {
			return connect.NewResponse(&querierv1.SeriesResponse{}), nil
		}
		c.Msg.Start = int64(validated.Start)
		c.Msg.End = int64(validated.End)
	}

	query, err := buildQueryFromMatchers(c.Msg.Matchers)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	blocks, err := f.listMetadata(ctx, tenantIDs, c.Msg.Start, c.Msg.End, query)
	if err != nil {
		return nil, err
	}

	_ = level.Info(f.log).Log("msg", "calling backend")
	resp, err := f.querybackendclient.Invoke(ctx, &querybackendv1.InvokeRequest{
		Tenant:        tenantIDs,
		StartTime:     c.Msg.Start,
		EndTime:       c.Msg.End,
		LabelSelector: query,
		Options:       &querybackendv1.InvokeOptions{},
		QueryPlan: &querybackendv1.QueryPlan{
			Blocks: blocks,
		},
		Query: []*querybackendv1.Query{{
			QueryType: &querybackendv1.Query_SeriesLabels{
				SeriesLabels: &querybackendv1.SeriesLabelsQuery{
					LabelNames: c.Msg.LabelNames,
				},
			},
		}},
	})
	if err != nil {
		return nil, err
	}

	_ = level.Info(f.log).Log("msg", "backend responded", "reports", len(resp.Reports))
	var report querybackendv1.Report_SeriesLabels
	if !findReport(&report, resp.Reports) {
		return connect.NewResponse(&querierv1.SeriesResponse{}), nil
	}

	return connect.NewResponse(&querierv1.SeriesResponse{
		LabelsSet: report.SeriesLabels.SeriesLabels,
	}), nil
}
