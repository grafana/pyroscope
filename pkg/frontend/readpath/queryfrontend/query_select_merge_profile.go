package queryfrontend

import (
	"context"

	"connectrpc.com/connect"
	"github.com/go-kit/log/level"
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

	// TODO: Remove this path and answer all queries using the tree
	// PGO queries always use the pprof path, as query tree doesn't support the truncation
	useQueryTree := false
	if c.Msg.StackTraceSelector.GetGoPgo() == nil {
		for _, tenantID := range tenantIDs {
			if q.limits.QueryTreeEnabled(tenantID) {
				useQueryTree = true
				break
			}
		}
	}

	if !useQueryTree {
		level.Info(q.logger).Log("msg", "use pprof query-backend based query")
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

	// From now on answer using the more experimental tree based approach
	level.Info(q.logger).Log("msg", "use tree query-backend based query")
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
	p.Mapping = report.Tree.Symbols.Mappings[1:]
	p.Location = report.Tree.Symbols.Locations[1:]
	p.Function = report.Tree.Symbols.Functions[1:]

	otherLocationID := uint64(0)
	otherLocation := func() uint64 {
		if otherLocationID != 0 {
			return otherLocationID
		}
		stringRef := int64(len(p.StringTable))
		p.StringTable = append(p.StringTable, "other")
		funcRef := uint64(len(p.Function) + 1)
		p.Function = append(p.Function, &profilev1.Function{
			Id:         funcRef,
			Name:       stringRef,
			SystemName: stringRef,
		})
		otherLocationID = uint64(len(p.Location) + 1)
		p.Location = append(p.Location, &profilev1.Location{
			Id: otherLocationID,
			Line: []*profilev1.Line{{
				FunctionId: funcRef,
			}},
		})
		return otherLocationID
	}

	t, err := phlaremodel.UnmarshalTree[phlaremodel.LocationRefName, phlaremodel.LocationRefNameI](report.Tree.Tree)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	// TODO: Back this up with better data
	p.Sample = make([]*profilev1.Sample, 0, len(report.Tree.Tree)/16)

	t.IterateStacks(func(_ phlaremodel.LocationRefName, self int64, stack []phlaremodel.LocationRefName) {
		if self <= 0 {
			return
		}
		locationIDs := make([]uint64, 0, len(stack))
		for idx, locID := range stack {
			// when we hit an other, we need to reset the stack
			if locID == phlaremodel.OtherLocationRef {
				locationIDs = append(locationIDs, otherLocation())
				continue
			}
			// handle the sentinel location, when it is the root
			if locID == phlaremodel.LocationRefName(0) {
				if idx == len(stack)-1 {
					break
				}
				panic("unexpected sentinel location")
			}
			locationIDs = append(locationIDs, uint64(locID))
		}

		p.Sample = append(p.Sample, &profilev1.Sample{
			LocationId: locationIDs,
			Value:      []int64{self},
		})
	})

	p.SampleType = []*profilev1.ValueType{{}}
	p.PeriodType = &profilev1.ValueType{}

	p.SampleType[0].Type = int64(len(p.StringTable))
	p.StringTable = append(p.StringTable, profileType.SampleType)
	p.SampleType[0].Unit = int64(len(p.StringTable))
	p.StringTable = append(p.StringTable, profileType.SampleUnit)
	p.PeriodType.Type = int64(len(p.StringTable))
	p.StringTable = append(p.StringTable, profileType.PeriodType)
	p.PeriodType.Unit = int64(len(p.StringTable))
	p.StringTable = append(p.StringTable, profileType.PeriodUnit)
	p.TimeNanos = c.Msg.End * 1e6

	// TODO: Set more fields on profile

	return connect.NewResponse(&p), nil
}
