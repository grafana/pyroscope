package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"connectrpc.com/connect"
	"github.com/go-kit/log/level"
	"google.golang.org/protobuf/encoding/protojson"

	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	phlaremodel "github.com/grafana/pyroscope/v2/pkg/model"
)

type queryAnomaliesParams struct {
	*queryParams
	ProfileType  string
	AnomalyTypes []string
	Output       string
	GroupBy      []string
	Step         time.Duration
	Limit        int64
}

func addQueryAnomaliesParams(queryCmd commander) *queryAnomaliesParams {
	params := new(queryAnomaliesParams)
	params.queryParams = addQueryParams(queryCmd)
	queryCmd.Flag("profile-type", "Profile type to query.").Default("process_cpu:cpu:nanoseconds:cpu:nanoseconds").StringVar(&params.ProfileType)
	queryCmd.Flag("anomaly-type", "Allowed values: stacktrace, time_series, ANOMALY_TYPE_STACKTRACE, ANOMALY_TYPE_TIME_SERIES (case-insensitive). Repeatable for multiple types.").Default("stacktrace").StringsVar(&params.AnomalyTypes)
	queryCmd.Flag("time-series.group-by", "Time-series only: label to group by. Repeatable; ignored unless time_series is requested.").StringsVar(&params.GroupBy)
	queryCmd.Flag("time-series.step", "Time-series only: resolution (at least 1ms). Ignored unless time_series is requested.").Default("1m").DurationVar(&params.Step)
	queryCmd.Flag("time-series.limit", "Time-series only: top N series by strongest anomaly score (nonpositive means unlimited). Ignored unless time_series is requested.").Int64Var(&params.Limit)
	queryCmd.Flag("output", "Output format, one of: table, json.").Default("table").StringVar(&params.Output)
	return params
}

func parseAnomalyTypes(names []string) ([]querierv1.AnomalyType, error) {
	types := make([]querierv1.AnomalyType, len(names))
	for i, name := range names {
		normalized := strings.TrimPrefix(strings.ToUpper(name), "ANOMALY_TYPE_")
		v, ok := querierv1.AnomalyType_value["ANOMALY_TYPE_"+normalized]
		if !ok {
			return nil, fmt.Errorf("unknown anomaly type %q; allowed values: stacktrace, time_series, ANOMALY_TYPE_STACKTRACE, ANOMALY_TYPE_TIME_SERIES (case-insensitive)", name)
		}
		types[i] = querierv1.AnomalyType(v)
	}
	return types, nil
}

func queryAnomalies(ctx context.Context, params *queryAnomaliesParams) error {
	from, to, err := params.parseFromTo()
	if err != nil {
		return err
	}

	anomalyTypes, err := parseAnomalyTypes(params.AnomalyTypes)
	if err != nil {
		return err
	}

	level.Info(logger).Log(
		"msg", "querying anomalies",
		"url", params.URL,
		"from", from,
		"to", to,
		"query", params.Query,
		"type", params.ProfileType,
		"anomaly_types", strings.Join(params.AnomalyTypes, ","),
	)

	req := &querierv1.QueryAnomaliesRequest{
		ProfileTypeID: params.ProfileType,
		LabelSelector: params.Query,
		Start:         from.UnixMilli(),
		End:           to.UnixMilli(),
		AnomalyTypes:  anomalyTypes,
	}
	for _, anomalyType := range anomalyTypes {
		if anomalyType == querierv1.AnomalyType_ANOMALY_TYPE_TIME_SERIES {
			if params.Step < time.Millisecond {
				return fmt.Errorf("--time-series.step must be at least 1ms")
			}
			req.TimeSeries = &querierv1.AnomalyTimeSeriesRequest{GroupBy: params.GroupBy, Step: params.Step.Seconds()}
			if params.Limit > 0 {
				req.TimeSeries.Limit = &params.Limit
			}
			break
		}
	}
	resp, err := params.queryClient().QueryAnomalies(ctx, connect.NewRequest(req))
	if err != nil {
		return fmt.Errorf("failed to query anomalies: %w", err)
	}
	logDiagnostics(params.phlareClient, resp.Header())

	return outputAnomalies(ctx, resp.Msg, anomalyTypes, params.Output)
}

func outputAnomalies(ctx context.Context, response *querierv1.QueryAnomaliesResponse, anomalyTypes []querierv1.AnomalyType, format string) error {
	var hasStacktrace, hasTimeSeries bool
	for _, anomalyType := range anomalyTypes {
		switch anomalyType {
		case querierv1.AnomalyType_ANOMALY_TYPE_STACKTRACE:
			hasStacktrace = true
		case querierv1.AnomalyType_ANOMALY_TYPE_TIME_SERIES:
			hasTimeSeries = true
		}
	}
	if hasTimeSeries && format == outputJSON {
		data, err := (protojson.MarshalOptions{EmitUnpopulated: true}).Marshal(response)
		if err != nil {
			return err
		}
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(data, &fields); err != nil {
			return err
		}
		if !hasStacktrace {
			delete(fields, "stacktraceAnomalies")
		}
		enc := json.NewEncoder(output(ctx))
		enc.SetIndent("", "  ")
		return enc.Encode(fields)
	}
	if !hasStacktrace {
		if hasTimeSeries {
			return outputSeriesAnalysis(ctx, response, format)
		}
		return nil
	}
	profiles := response.StacktraceAnomalies

	switch format {
	case outputJSON:
		type jsonAnomaly struct {
			ProfileID string            `json:"profile_id"`
			Timestamp time.Time         `json:"timestamp"`
			Score     float64           `json:"score"`
			Labels    map[string]string `json:"labels,omitempty"`
		}
		out := make([]jsonAnomaly, len(profiles))
		for i, p := range profiles {
			out[i] = jsonAnomaly{
				ProfileID: p.ProfileId,
				Timestamp: time.UnixMilli(p.Timestamp).UTC(),
				Score:     p.Score,
				Labels:    labelsToMap(p.Labels),
			}
		}
		enc := json.NewEncoder(output(ctx))
		enc.SetIndent("", "  ")
		return enc.Encode(out)
	default:
		table := newTableWriter(output(ctx))
		table.SetHeader([]string{"Profile ID", "Timestamp", "Score", "Labels"})
		for _, p := range profiles {
			table.Append([]string{
				p.ProfileId,
				time.UnixMilli(p.Timestamp).UTC().Format(time.RFC3339),
				fmt.Sprintf("%.4f", p.Score),
				formatLabels(p.Labels),
			})
		}
		table.Render()
	}

	if hasTimeSeries {
		return outputSeriesAnalysis(ctx, response, "table")
	}
	return nil
}

func labelsToMap(labels []*typesv1.LabelPair) map[string]string {
	labels = phlaremodel.Labels(labels).WithoutPrivateLabels()
	if len(labels) == 0 {
		return nil
	}
	m := make(map[string]string, len(labels))
	for _, l := range labels {
		m[l.Name] = l.Value
	}
	return m
}

func formatLabels(labels []*typesv1.LabelPair) string {
	return phlaremodel.LabelPairsString(phlaremodel.Labels(labels).WithoutPrivateLabels())
}
