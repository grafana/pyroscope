package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
	"github.com/grafana/pyroscope/v2/pkg/model"
)

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
