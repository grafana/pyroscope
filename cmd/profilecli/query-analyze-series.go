package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"connectrpc.com/connect"

	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
	"github.com/grafana/pyroscope/v2/pkg/model"
)

type queryAnalyzeSeriesParams struct {
	*queryParams
	ProfileType string
	GroupBy     []string
	Step        time.Duration
	Limit       int64
	Output      string
}

func addQueryAnalyzeSeriesParams(cmd commander) *queryAnalyzeSeriesParams {
	p := &queryAnalyzeSeriesParams{queryParams: addQueryParams(cmd)}
	cmd.Flag("profile-type", "Profile type to query.").Default("process_cpu:cpu:nanoseconds:cpu:nanoseconds").StringVar(&p.ProfileType)
	cmd.Flag("group-by", "Label to group by. Can be specified multiple times.").StringsVar(&p.GroupBy)
	cmd.Flag("step", "Time-series resolution (at least 1ms).").Default("1m").DurationVar(&p.Step)
	cmd.Flag("limit", "Return anomalies from the top N series by strongest score (nonpositive means unlimited).").Int64Var(&p.Limit)
	cmd.Flag("output", "Output format: table or json.").Default("table").EnumVar(&p.Output, "table", "json")
	return p
}

func queryAnalyzeSeries(ctx context.Context, p *queryAnalyzeSeriesParams) error {
	from, to, err := p.parseFromTo()
	if err != nil {
		return err
	}
	if p.Step < time.Millisecond {
		return fmt.Errorf("--step must be at least 1ms")
	}
	req := &querierv1.QueryAnomaliesRequest{
		AnomalyTypes:  []querierv1.AnomalyType{querierv1.AnomalyType_ANOMALY_TYPE_TIME_SERIES},
		ProfileTypeID: p.ProfileType, LabelSelector: p.Query,
		Start: from.UnixMilli(), End: to.UnixMilli(),
		TimeSeries: &querierv1.AnomalyTimeSeriesRequest{GroupBy: p.GroupBy, Step: p.Step.Seconds()},
	}
	if p.Limit > 0 {
		req.TimeSeries.Limit = &p.Limit
	}
	resp, err := p.queryClient().QueryAnomalies(ctx, connect.NewRequest(req))
	if err != nil {
		return fmt.Errorf("failed to analyze series: %w", err)
	}
	logDiagnostics(p.phlareClient, resp.Header())
	return outputSeriesAnalysis(ctx, resp.Msg, p.Output)
}

func outputSeriesAnalysis(ctx context.Context, response *querierv1.QueryAnomaliesResponse, format string) error {
	if format == outputJSON {
		return outputAnomalies(ctx, response, []querierv1.AnomalyType{querierv1.AnomalyType_ANOMALY_TYPE_TIME_SERIES}, outputJSON)
	}
	table := newTableWriter(output(ctx))
	table.SetHeader([]string{"Labels", "Type", "Start", "End", "Peak time", "Baseline", "Peak value", "Change", "Score"})
	for _, event := range response.TimeSeriesAnomalies {
		table.Append([]string{
			model.Labels(event.Labels).ToPrometheusLabels().String(),
			strings.TrimPrefix(event.Type.String(), "TIME_SERIES_ANOMALY_TYPE_"),
			time.UnixMilli(event.TimestampStart).UTC().Format(time.RFC3339Nano),
			time.UnixMilli(event.TimestampEnd).UTC().Format(time.RFC3339Nano),
			time.UnixMilli(event.TimestampPeak).UTC().Format(time.RFC3339Nano),
			fmt.Sprintf("%.6g", event.Baseline), fmt.Sprintf("%.6g", event.PeakValue),
			fmt.Sprintf("%.2f%%", event.RelativeChange*100), fmt.Sprintf("%.6g", event.Score),
		})
	}
	table.Render()
	return nil
}
