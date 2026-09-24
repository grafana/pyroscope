package queryfrontend

import (
	"context"
	"fmt"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/grafana/dskit/tenant"

	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	"github.com/grafana/pyroscope/v2/pkg/anomalyapi"
	phlaremodel "github.com/grafana/pyroscope/v2/pkg/model"
	"github.com/grafana/pyroscope/v2/pkg/validation"
)

// QueryAnomalies returns, out of the profiles matching the request, the profile IDs also
// flagged as anomalies by the configured external anomaly source.
func (q *QueryFrontend) QueryAnomalies(
	ctx context.Context,
	c *connect.Request[querierv1.QueryAnomaliesRequest],
) (*connect.Response[querierv1.QueryAnomaliesResponse], error) {
	if len(c.Msg.AnomalyTypes) == 0 {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("at least one anomaly_type is required"))
	}

	resp := &querierv1.QueryAnomaliesResponse{}
	for _, t := range c.Msg.AnomalyTypes {
		switch t {
		case querierv1.AnomalyType_ANOMALY_TYPE_STACKTRACE:
			profiles, err := q.queryStacktraceAnomalies(ctx, c.Msg)
			if err != nil {
				return nil, err
			}
			resp.StacktraceAnomalies = profiles
		default:
			return nil, connect.NewError(connect.CodeUnimplemented,
				fmt.Errorf("unsupported anomaly_type %q", t))
		}
	}
	return connect.NewResponse(resp), nil
}

func (q *QueryFrontend) queryStacktraceAnomalies(
	ctx context.Context,
	req *querierv1.QueryAnomaliesRequest,
) ([]*querierv1.StacktraceAnomaly, error) {
	if q.anomalyAPI == nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition,
			fmt.Errorf("anomaly_type ANOMALY_TYPE_STACKTRACE requires query-frontend.anomaly-api.url to be configured"))
	}

	tenantIDs, err := tenant.TenantIDs(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	if len(tenantIDs) != 1 {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("anomaly_type ANOMALY_TYPE_STACKTRACE requires a single tenant, got %d", len(tenantIDs)))
	}

	empty, err := validation.SanitizeTimeRange(q.limits, tenantIDs, &req.Start, &req.End)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	if empty {
		return nil, nil
	}

	serviceNames, err := q.resolveServiceNames(ctx, req)
	if err != nil {
		return nil, err
	}

	anomalies, err := q.anomalyAPI.ListAnomalies(ctx, tenantIDs[0], serviceNames,
		time.UnixMilli(req.Start), time.UnixMilli(req.End))
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("listing anomalies: %w", err))
	}

	confirmed, err := q.confirmAnomalies(ctx, req, anomalies)
	if err != nil {
		return nil, err
	}

	scoreByID := make(map[string]float64, len(anomalies))
	for _, a := range anomalies {
		id, err := uuid.Parse(a.ProfileUUID)
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal,
				fmt.Errorf("anomaly source returned invalid profile_uuid %q: %w", a.ProfileUUID, err))
		}
		scoreByID[id.String()] = a.Score
	}
	profiles := make([]*querierv1.StacktraceAnomaly, len(confirmed))
	for i, p := range confirmed {
		profiles[i] = &querierv1.StacktraceAnomaly{
			ProfileId: p.ProfileId,
			Timestamp: p.Timestamp,
			Labels:    p.Labels,
			Score:     scoreByID[p.ProfileId],
		}
	}
	return profiles, nil
}

// confirmAnomalies filters candidate anomalies down to the ones actually present in ingested
// data, matching the request's full label selector and time range, via a QUERY_PROFILE_PRESENCE
// query-backend call carrying the whole candidate list
func (q *QueryFrontend) confirmAnomalies(
	ctx context.Context,
	req *querierv1.QueryAnomaliesRequest,
	anomalies []anomalyapi.Anomaly,
) ([]*queryv1.ProfilePresenceEntry, error) {
	if len(anomalies) == 0 {
		return nil, nil
	}

	allIDs := make([]string, len(anomalies))
	for i, a := range anomalies {
		allIDs[i] = a.ProfileUUID
	}

	labelSelector, err := buildLabelSelectorWithProfileType(req.LabelSelector, req.ProfileTypeID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	report, err := q.querySingle(ctx, &queryv1.QueryRequest{
		StartTime:     req.Start,
		EndTime:       req.End,
		LabelSelector: labelSelector,
		Query: []*queryv1.Query{{
			QueryType:       queryv1.QueryType_QUERY_PROFILE_PRESENCE,
			ProfilePresence: &queryv1.ProfilePresenceQuery{ProfileIdSelector: allIDs},
		}},
	}, nil)
	if err != nil {
		return nil, err
	}
	if report == nil {
		return nil, nil
	}
	return report.ProfilePresence.GetProfiles(), nil
}

func (q *QueryFrontend) resolveServiceNames(
	ctx context.Context,
	req *querierv1.QueryAnomaliesRequest,
) ([]string, error) {
	resp, err := q.Series(ctx, connect.NewRequest(&querierv1.SeriesRequest{
		Start:      req.Start,
		End:        req.End,
		Matchers:   []string{req.LabelSelector},
		LabelNames: []string{phlaremodel.LabelNameServiceName},
	}))
	if err != nil {
		return nil, err
	}

	seen := make(map[string]struct{})
	var names []string
	for _, lbls := range resp.Msg.LabelsSet {
		for _, l := range lbls.Labels {
			if l.Name == phlaremodel.LabelNameServiceName {
				if _, ok := seen[l.Value]; !ok {
					seen[l.Value] = struct{}{}
					names = append(names, l.Value)
				}
			}
		}
	}

	if len(names) == 0 {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("label selector %q does not match any service_name", req.LabelSelector))
	}
	return names, nil
}
