package main

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
)

func TestParseAnomalyTypes(t *testing.T) {
	for _, tc := range []struct {
		name string
		want querierv1.AnomalyType
	}{
		{"stacktrace", querierv1.AnomalyType_ANOMALY_TYPE_STACKTRACE},
		{"ANOMALY_TYPE_STACKTRACE", querierv1.AnomalyType_ANOMALY_TYPE_STACKTRACE},
		{"time_series", querierv1.AnomalyType_ANOMALY_TYPE_TIME_SERIES},
		{"ANOMALY_TYPE_TIME_SERIES", querierv1.AnomalyType_ANOMALY_TYPE_TIME_SERIES},
		{"anomaly_type_time_series", querierv1.AnomalyType_ANOMALY_TYPE_TIME_SERIES},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseAnomalyTypes([]string{tc.name})
			require.NoError(t, err)
			require.Equal(t, []querierv1.AnomalyType{tc.want}, got)
		})
	}
	_, err := parseAnomalyTypes([]string{"unknown"})
	require.Error(t, err)
}

func TestOutputAnomalies_OnlyRequestedTypes(t *testing.T) {
	response := &querierv1.QueryAnomaliesResponse{
		StacktraceAnomalies: []*querierv1.StacktraceAnomaly{{ProfileId: "unrequested-profile"}},
		TimeSeriesAnomalies: []*querierv1.TimeSeriesAnomaly{{Type: querierv1.TimeSeriesAnomalyType_TIME_SERIES_ANOMALY_TYPE_SPIKE}},
	}
	for _, format := range []string{"table", "json"} {
		t.Run(format, func(t *testing.T) {
			var buf bytes.Buffer
			err := outputAnomalies(withOutput(context.Background(), &buf), response,
				[]querierv1.AnomalyType{querierv1.AnomalyType_ANOMALY_TYPE_TIME_SERIES}, format)
			require.NoError(t, err)
			require.NotContains(t, buf.String(), "Profile ID")
			require.NotContains(t, buf.String(), "stacktraceAnomalies")
			require.NotContains(t, buf.String(), "unrequested-profile")
			require.Contains(t, buf.String(), "SPIKE")
		})
	}
	t.Run("stacktrace only", func(t *testing.T) {
		var buf bytes.Buffer
		require.NoError(t, outputAnomalies(withOutput(context.Background(), &buf), response,
			[]querierv1.AnomalyType{querierv1.AnomalyType_ANOMALY_TYPE_STACKTRACE}, "table"))
		require.Contains(t, buf.String(), "Profile ID")
		require.NotContains(t, buf.String(), "SPIKE")
	})
	t.Run("both", func(t *testing.T) {
		var buf bytes.Buffer
		require.NoError(t, outputAnomalies(withOutput(context.Background(), &buf), response,
			[]querierv1.AnomalyType{querierv1.AnomalyType_ANOMALY_TYPE_STACKTRACE, querierv1.AnomalyType_ANOMALY_TYPE_TIME_SERIES}, "table"))
		require.Contains(t, buf.String(), "Profile ID")
		require.Contains(t, buf.String(), "SPIKE")
	})
}
