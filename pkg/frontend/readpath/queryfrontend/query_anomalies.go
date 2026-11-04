package queryfrontend

import (
	"context"
	"fmt"

	"connectrpc.com/connect"

	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
)

// QueryAnomalies queries the requested anomaly sources for matching profiles.
func (q *QueryFrontend) QueryAnomalies(
	ctx context.Context,
	c *connect.Request[querierv1.QueryAnomaliesRequest],
) (*connect.Response[querierv1.QueryAnomaliesResponse], error) {
	if len(c.Msg.AnomalyTypes) == 0 {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("at least one anomaly_type is required"))
	}

	seen := make(map[querierv1.AnomalyType]struct{}, len(c.Msg.AnomalyTypes))
	uniqueTypes := make([]querierv1.AnomalyType, 0, len(c.Msg.AnomalyTypes))
	for _, t := range c.Msg.AnomalyTypes {
		switch t {
		case querierv1.AnomalyType_ANOMALY_TYPE_TIME_SERIES:
		default:
			return nil, connect.NewError(connect.CodeUnimplemented,
				fmt.Errorf("unsupported anomaly_type %q", t))
		}
		if _, ok := seen[t]; ok {
			continue
		}
		seen[t] = struct{}{}
		uniqueTypes = append(uniqueTypes, t)
	}

	resp := &querierv1.QueryAnomaliesResponse{}
	for _, t := range uniqueTypes {
		switch t {
		case querierv1.AnomalyType_ANOMALY_TYPE_TIME_SERIES:
			anomalies, err := q.queryTimeSeriesAnomalies(ctx, c.Msg)
			if err != nil {
				return nil, err
			}
			resp.TimeSeriesAnomalies = anomalies
		}
	}
	return connect.NewResponse(resp), nil
}
