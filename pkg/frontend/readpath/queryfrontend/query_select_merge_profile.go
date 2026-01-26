package queryfrontend

import (
	"context"

	"connectrpc.com/connect"
	"github.com/grafana/dskit/tenant"

	profilev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	"github.com/grafana/pyroscope/pkg/model"
	phlaremodel "github.com/grafana/pyroscope/pkg/model"
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

	profileType, err := phlaremodel.ParseProfileTypeSelector(c.Msg.ProfileTypeID)
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
			QueryType: queryv1.QueryType_QUERY_TREE,
			Tree: &queryv1.TreeQuery{
				MaxNodes:           maxNodes,
				StackTraceSelector: c.Msg.StackTraceSelector,
				ProfileIdSelector:  c.Msg.ProfileIdSelector,
				FullSymbols:        true,
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

	p.StringTable = report.Tree.Symbols.Strings
	p.Mapping = report.Tree.Symbols.Mappings
	p.Location = report.Tree.Symbols.Locations
	p.Function = report.Tree.Symbols.Functions

	t, err := model.UnmarshalTree[model.LocationRefName, model.LocationRefNameI](report.Tree.Tree)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	// TODO: Back this up with better data
	p.Sample = make([]*profilev1.Sample, 0, len(report.Tree.Tree)/16)

	t.IterateStacks(func(_ model.LocationRefName, self int64, stack []model.LocationRefName) {
		locationID := make([]uint64, len(stack))
		for idx := range locationID {
			locationID[idx] = uint64(stack[idx])
		}
		p.Sample = append(p.Sample, &profilev1.Sample{
			LocationId: locationID,
			Value:      []int64{self},
		})
	})

	sampleTypeRef := len(p.StringTable)
	p.StringTable = append(p.StringTable, profileType.SampleType)
	sampleUnitRef := len(p.StringTable)
	p.StringTable = append(p.StringTable, profileType.SampleUnit)

	p.SampleType = []*profilev1.ValueType{{
		Type: int64(sampleTypeRef),
		Unit: int64(sampleUnitRef),
	}}

	// TODO: Set more fields on profile

	return connect.NewResponse(&p), nil
}
