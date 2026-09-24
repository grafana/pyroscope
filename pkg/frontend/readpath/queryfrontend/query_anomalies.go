package queryfrontend

import (
	"context"
	"fmt"
	"time"

	"connectrpc.com/connect"
	"github.com/grafana/dskit/tenant"
	"github.com/prometheus/prometheus/model/labels"

	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	"github.com/grafana/pyroscope/v2/pkg/anomalyapi"
	phlaremodel "github.com/grafana/pyroscope/v2/pkg/model"
)

// QueryAnomalies returns, out of the profiles matching the request, the profile IDs also
// flagged as anomalies by the configured external anomaly source.
func (q *QueryFrontend) QueryAnomalies(
	ctx context.Context,
	c *connect.Request[querierv1.QueryAnomaliesRequest],
) (*connect.Response[querierv1.QueryAnomaliesResponse], error) {
	switch c.Msg.AnomalyType {
	case "stacktrace":
		return q.queryStacktraceAnomalies(ctx, c.Msg)
	default:
		return nil, connect.NewError(connect.CodeUnimplemented,
			fmt.Errorf("unsupported anomaly_type %q", c.Msg.AnomalyType))
	}
}

func (q *QueryFrontend) queryStacktraceAnomalies(
	ctx context.Context,
	req *querierv1.QueryAnomaliesRequest,
) (*connect.Response[querierv1.QueryAnomaliesResponse], error) {
	if q.anomalyAPI == nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition,
			fmt.Errorf(`anomaly_type "stacktrace" requires anomaly-api.url to be configured`))
	}

	tenantIDs, err := tenant.TenantIDs(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	if len(tenantIDs) != 1 {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("anomaly_type %q requires a single tenant, got %d", req.AnomalyType, len(tenantIDs)))
	}

	serviceName, err := serviceNameFromLabelSelector(req.LabelSelector)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	anomalies, err := q.anomalyAPI.ListAnomalies(ctx, tenantIDs[0], serviceName,
		time.UnixMilli(req.Start), time.UnixMilli(req.End))
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("listing anomalies: %w", err))
	}

	confirmed, err := q.confirmAnomalies(ctx, req, anomalies)
	if err != nil {
		return nil, err
	}

	observedAt := make(map[string]int64, len(anomalies))
	for _, a := range anomalies {
		observedAt[a.ProfileUUID] = a.ObservedAt.UnixMilli()
	}
	profiles := make([]*querierv1.AnomalyProfile, len(confirmed))
	for i, p := range confirmed {
		profiles[i] = &querierv1.AnomalyProfile{
			ProfileId:  p.ProfileId,
			ObservedAt: observedAt[p.ProfileId],
			Labels:     p.Labels,
		}
	}
	return connect.NewResponse(&querierv1.QueryAnomaliesResponse{Profiles: profiles}), nil
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

// serviceNameFromLabelSelector extracts the service_name matcher's value. The anomaly source
// is indexed by tenant+service only; the rest of the label selector is enforced later, in
// confirmAnomalies.
func serviceNameFromLabelSelector(labelSelector string) (string, error) {
	matchers, err := phlaremodel.ParseMetricSelector(labelSelector)
	if err != nil {
		return "", err
	}
	for _, m := range matchers {
		if m.Name == phlaremodel.LabelNameServiceName && m.Type == labels.MatchEqual {
			return m.Value, nil
		}
	}
	return "", fmt.Errorf("label selector %q does not contain a service_name matcher", labelSelector)
}
