package queryfrontend

import (
	"context"

	"connectrpc.com/connect"
	"github.com/grafana/dskit/tenant"

	profilev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	phlaremodel "github.com/grafana/pyroscope/pkg/model"
	"github.com/grafana/pyroscope/pkg/pprof"
	"github.com/grafana/pyroscope/pkg/validation"
)

func (q *QueryFrontend) SelectMergeProfile(
	ctx context.Context,
	c *connect.Request[querierv1.SelectMergeProfileRequest],
) (*connect.Response[profilev1.Profile], error) {
	tenantIDs, err := tenant.TenantIDs(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	empty, err := validation.SanitizeTimeRange(q.limits, tenantIDs, &c.Msg.Start, &c.Msg.End)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	if empty {
		return connect.NewResponse(&profilev1.Profile{}), nil
	}

	_, err = phlaremodel.ParseProfileTypeSelector(c.Msg.ProfileTypeID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	// NOTE: Max nodes limit is off by default. SelectMergeProfile is primarily
	// used for pprof export where truncation would result in incomplete profiles.
	// This can be overridden per-tenant to enable enforcement if needed.
	maxNodesEnabled := false
	for _, tenantID := range tenantIDs {
		if q.limits.MaxFlameGraphNodesOnSelectMergeProfile(tenantID) {
			maxNodesEnabled = true
		}
	}

	maxNodes := c.Msg.GetMaxNodes()
	if maxNodesEnabled {
		maxNodes, err = validation.ValidateMaxNodes(q.limits, tenantIDs, c.Msg.GetMaxNodes())
		if err != nil {
			return nil, connect.NewError(connect.CodeInvalidArgument, err)
		}
	}

	labelSelector, err := buildLabelSelectorWithProfileType(c.Msg.LabelSelector, c.Msg.ProfileTypeID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	report, err := q.querySingle(ctx, &queryv1.QueryRequest{
		StartTime:     c.Msg.Start,
		EndTime:       c.Msg.End,
		LabelSelector: labelSelector,
		Query: []*queryv1.Query{{
			QueryType: queryv1.QueryType_QUERY_PPROF,
			Pprof: &queryv1.PprofQuery{
				MaxNodes:           maxNodes,
				StackTraceSelector: c.Msg.StackTraceSelector,
				ProfileIdSelector:  c.Msg.ProfileIdSelector,
			},
		}},
	})
	if err != nil {
		return nil, err
	}
	if report == nil {
		return connect.NewResponse(&profilev1.Profile{}), nil
	}
	var p profilev1.Profile
	if err = pprof.Unmarshal(report.Pprof.Pprof, &p); err != nil {
		return nil, err
	}

	return connect.NewResponse(&p), nil
}
