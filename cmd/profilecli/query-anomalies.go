package main

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"connectrpc.com/connect"
	"github.com/go-kit/log/level"

	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	phlaremodel "github.com/grafana/pyroscope/v2/pkg/model"
)

type queryAnomaliesParams struct {
	*queryParams
	ProfileType string
	AnomalyType string
	Output      string
}

func addQueryAnomaliesParams(queryCmd commander) *queryAnomaliesParams {
	params := new(queryAnomaliesParams)
	params.queryParams = addQueryParams(queryCmd)
	queryCmd.Flag("profile-type", "Profile type to query.").Default("process_cpu:cpu:nanoseconds:cpu:nanoseconds").StringVar(&params.ProfileType)
	queryCmd.Flag("anomaly-type", "Which anomaly source to consult. Only \"stacktrace\" is supported today.").Default("stacktrace").StringVar(&params.AnomalyType)
	queryCmd.Flag("output", "Output format, one of: table, json.").Default("table").StringVar(&params.Output)
	return params
}

func queryAnomalies(ctx context.Context, params *queryAnomaliesParams) error {
	from, to, err := params.parseFromTo()
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
		"anomaly_type", params.AnomalyType,
	)

	qc := params.queryClient()
	resp, err := qc.QueryAnomalies(ctx, connect.NewRequest(&querierv1.QueryAnomaliesRequest{
		ProfileTypeID: params.ProfileType,
		LabelSelector: params.Query,
		Start:         from.UnixMilli(),
		End:           to.UnixMilli(),
		AnomalyType:   params.AnomalyType,
	}))
	if err != nil {
		return fmt.Errorf("failed to query anomalies: %w", err)
	}
	logDiagnostics(params.phlareClient, resp.Header())

	profiles := resp.Msg.Profiles

	switch params.Output {
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
