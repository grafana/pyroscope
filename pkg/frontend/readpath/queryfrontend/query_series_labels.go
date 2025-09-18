package queryfrontend

import (
	"context"

	"connectrpc.com/connect"
	"github.com/go-kit/log/level"
	"github.com/grafana/dskit/tenant"
	"github.com/grafana/pyroscope/pkg/clientcapability"

	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	"github.com/grafana/pyroscope/pkg/validation"
)

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
	report, err := q.querySingle(ctx, &queryv1.QueryRequest{
		StartTime:     c.Msg.Start,
		EndTime:       c.Msg.End,
		LabelSelector: labelSelector,
		Query: []*queryv1.Query{{
			QueryType: queryv1.QueryType_QUERY_SERIES_LABELS,
			SeriesLabels: &queryv1.SeriesLabelsQuery{
				LabelNames: c.Msg.LabelNames,
			},
		}},
	})
	if err != nil {
		return nil, err
	}
	if report == nil {
		return connect.NewResponse(&querierv1.SeriesResponse{}), nil
	}

	seriesLabels := report.SeriesLabels.SeriesLabels
	if enabled := clientcapability.Utf8LabelNamesEnabled(ctx); !enabled {
		// Use legacy label name sanitization if utf8 label names not enabled
		for _, seriesLabel := range seriesLabels {
			labelNames := make([]string, len(seriesLabel.Labels))
			for i, label := range seriesLabel.Labels {
				labelNames[i] = label.Name
			}

			// Sanitize the label names
			sanitizedNames, err := clientcapability.SanitizeLabelNames(labelNames)
			if err != nil {
				return nil, connect.NewError(connect.CodeInvalidArgument, err)
			}

			// Update the original labels with sanitized names
			for i, label := range seriesLabel.Labels {
				if i < len(sanitizedNames) {
					label.Name = sanitizedNames[i]
				}
			}
		}
	}

	return connect.NewResponse(&querierv1.SeriesResponse{LabelsSet: seriesLabels}), nil
}
