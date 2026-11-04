package main

import (
	"context"
	"fmt"
	"time"

	"connectrpc.com/connect"
	"github.com/go-kit/log/level"

	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
)

type queryAnomaliesParams struct {
	*queryParams
	ProfileType string
	Output      string
	GroupBy     []string
	Step        time.Duration
	Limit       int64
}

func addQueryAnomaliesParams(queryCmd commander) *queryAnomaliesParams {
	params := new(queryAnomaliesParams)
	params.queryParams = addQueryParams(queryCmd)
	queryCmd.Flag("profile-type", "Profile type to query.").Default("process_cpu:cpu:nanoseconds:cpu:nanoseconds").StringVar(&params.ProfileType)
	queryCmd.Flag("group-by", "Label to group by. Repeatable.").StringsVar(&params.GroupBy)
	queryCmd.Flag("step", "Resolution (at least 1ms).").Default("1m").DurationVar(&params.Step)
	queryCmd.Flag("limit", "Top N series by highest event priority score (nonpositive means unlimited).").Int64Var(&params.Limit)
	queryCmd.Flag("output", "Output format, one of: table, json.").Default("table").StringVar(&params.Output)
	return params
}

func queryAnomalies(ctx context.Context, params *queryAnomaliesParams) error {
	from, to, err := params.parseFromTo()
	if err != nil {
		return err
	}
	if params.Step < time.Millisecond {
		return fmt.Errorf("--step must be at least 1ms")
	}

	level.Info(logger).Log(
		"msg", "querying anomalies",
		"url", params.URL,
		"from", from,
		"to", to,
		"query", params.Query,
		"type", params.ProfileType,
	)

	req := &querierv1.QueryAnomaliesRequest{
		ProfileTypeID: params.ProfileType,
		LabelSelector: params.Query,
		Start:         from.UnixMilli(),
		End:           to.UnixMilli(),
		AnomalyTypes:  []querierv1.AnomalyType{querierv1.AnomalyType_ANOMALY_TYPE_TIME_SERIES},
		TimeSeries:    &querierv1.AnomalyTimeSeriesRequest{GroupBy: params.GroupBy, Step: params.Step.Seconds()},
	}
	if params.Limit > 0 {
		req.TimeSeries.Limit = &params.Limit
	}
	resp, err := params.queryClient().QueryAnomalies(ctx, connect.NewRequest(req))
	if err != nil {
		return fmt.Errorf("failed to query anomalies: %w", err)
	}
	logDiagnostics(params.phlareClient, resp.Header())

	return outputSeriesAnalysis(ctx, resp.Msg, params.Output)
}
