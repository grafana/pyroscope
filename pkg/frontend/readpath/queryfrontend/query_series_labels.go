package queryfrontend

import (
	"context"

	"connectrpc.com/connect"
	"github.com/go-kit/log/level"
	"github.com/grafana/dskit/tenant"

	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"

	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	"github.com/grafana/pyroscope/pkg/featureflags"
	"github.com/grafana/pyroscope/pkg/validation"
)

func (q *QueryFrontend) filterLabelNames(
	ctx context.Context,
	c *connect.Request[querierv1.SeriesRequest],
	labelNames []string,
	matchers []string,
) ([]string, error) {
	if capabilities, ok := featureflags.GetClientCapabilities(ctx); ok && capabilities.AllowUtf8LabelNames {
		return labelNames, nil
	}

	toFilter := make([]string, len(labelNames))
	copy(toFilter, labelNames)

	// Filter out label names not passing legacy validation if utf8 label names not enabled
	if len(labelNames) == 0 {
		// Querying for all label names; must retrieve all label names to then filter out
		response, err := q.LabelNames(ctx, connect.NewRequest(&typesv1.LabelNamesRequest{
			Start:    c.Msg.Start,
			End:      c.Msg.End,
			Matchers: matchers,
		}))

		if err != nil {
			return nil, err
		}
		if response != nil {
			toFilter = response.Msg.Names
		}
	}

	filtered := make([]string, 0, len(labelNames))
	for _, name := range toFilter {
		if _, _, ok := validation.SanitizeLegacyLabelName(name); !ok {
			level.Debug(q.logger).Log("msg", "filtering out label", "label_name", name)
			continue
		}
		filtered = append(filtered, name)
	}
	return filtered, nil
}

func (q *QueryFrontend) Series(
	ctx context.Context,
	c *connect.Request[querierv1.SeriesRequest],
) (*connect.Response[querierv1.SeriesResponse], error) {
	tenantIDs, err := tenant.TenantIDs(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	empty, err := validation.SanitizeTimeRange(q.limits, tenantIDs, &c.Msg.Start, &c.Msg.End)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	if empty {
		return connect.NewResponse(&querierv1.SeriesResponse{}), nil
	}

	if q.isProfileTypeQuery(c.Msg.LabelNames, c.Msg.Matchers) {
		level.Debug(q.logger).Log("msg", "listing profile types from metadata as series labels")
		return q.queryProfileTypeMetadataLabels(ctx, tenantIDs, c.Msg.Start, c.Msg.End, c.Msg.LabelNames)
	}

	labelSelector, err := buildLabelSelectorFromMatchers(c.Msg.Matchers)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	labelNames, err := q.filterLabelNames(ctx, c, c.Msg.LabelNames, c.Msg.Matchers)
	if err != nil {
		return nil, err
	}

	report, err := q.querySingle(ctx, &queryv1.QueryRequest{
		StartTime:     c.Msg.Start,
		EndTime:       c.Msg.End,
		LabelSelector: labelSelector,
		Query: []*queryv1.Query{{
			QueryType: queryv1.QueryType_QUERY_SERIES_LABELS,
			SeriesLabels: &queryv1.SeriesLabelsQuery{
				LabelNames: labelNames,
			},
		}},
	})
	if err != nil {
		return nil, err
	}
	if report == nil {
		return connect.NewResponse(&querierv1.SeriesResponse{}), nil
	}

	return connect.NewResponse(&querierv1.SeriesResponse{LabelsSet: report.SeriesLabels.SeriesLabels}), nil
}
