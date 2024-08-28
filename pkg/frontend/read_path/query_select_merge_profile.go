package read_path

import (
	"context"

	"connectrpc.com/connect"
	"github.com/grafana/dskit/tenant"
	"github.com/opentracing/opentracing-go"
	"github.com/prometheus/common/model"

	profilev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
	querybackendv1 "github.com/grafana/pyroscope/api/gen/proto/go/querybackend/v1"
	"github.com/grafana/pyroscope/pkg/pprof"
	"github.com/grafana/pyroscope/pkg/validation"
)

// TODO(kolesnikovae): Implement span selector.

func (q *QueryBackend) SelectMergeProfile(
	ctx context.Context,
	c *connect.Request[querierv1.SelectMergeProfileRequest],
) (*connect.Response[profilev1.Profile], error) {
	opentracing.SpanFromContext(ctx).
		SetTag("start", model.Time(c.Msg.Start).Time().String()).
		SetTag("end", model.Time(c.Msg.End).Time().String()).
		SetTag("selector", c.Msg.LabelSelector).
		SetTag("max_nodes", c.Msg.GetMaxNodes()).
		SetTag("profile_type", c.Msg.ProfileTypeID)

	tenantIDs, err := tenant.TenantIDs(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	empty, err := validation.ValidateTimeRange(q.limits, tenantIDs, &c.Msg.Start, &c.Msg.End)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	if empty {
		return connect.NewResponse(&profilev1.Profile{}), nil
	}

	// NOTE: Max nodes limit is not set by default:
	//   the method is used for pprof export and
	//   truncation might not be desirable.

	labelSelector, err := buildLabelSelectorWithProfileType(c.Msg.LabelSelector, c.Msg.ProfileTypeID)
	if err != nil {
		return nil, err
	}
	report, err := q.Query(ctx, c.Msg.Start, c.Msg.End, tenantIDs, labelSelector, &querybackendv1.Query{
		QueryType: querybackendv1.QueryType_QUERY_PPROF,
		Pprof:     &querybackendv1.PprofQuery{MaxNodes: c.Msg.GetMaxNodes()},
	})
	if err != nil {
		return nil, err
	}
	if report == nil {
		return nil, nil
	}
	var p profilev1.Profile
	if err = pprof.Unmarshal(report.Pprof.Pprof, &p); err != nil {
		return nil, err
	}
	return connect.NewResponse(&p), nil
}
