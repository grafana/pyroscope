package main

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
)

func TestOutputSeriesAnalysis(t *testing.T) {
	response := &querierv1.QueryAnomaliesResponse{TimeSeriesAnomalies: []*querierv1.TimeSeriesAnomaly{{
		Type:           querierv1.TimeSeriesAnomalyType_TIME_SERIES_ANOMALY_TYPE_SPIKE,
		TimestampStart: 1000, TimestampEnd: 2000, TimestampPeak: 1000,
		Baseline: 10, PeakValue: 20, RelativeChange: 1, Score: 5,
	}}}
	t.Run("json", func(t *testing.T) {
		var buf bytes.Buffer
		require.NoError(t, outputSeriesAnalysis(withOutput(context.Background(), &buf), response, "json"))
		var decoded querierv1.QueryAnomaliesResponse
		require.NoError(t, protojson.Unmarshal(buf.Bytes(), &decoded))
		require.True(t, proto.Equal(response, &decoded))
	})
	t.Run("table", func(t *testing.T) {
		var buf bytes.Buffer
		require.NoError(t, outputSeriesAnalysis(withOutput(context.Background(), &buf), response, "table"))
		require.Contains(t, buf.String(), "SPIKE")
		require.Contains(t, buf.String(), "100.00%")
		require.Contains(t, buf.String(), "1970-01-01T00:00:01Z")
	})
	t.Run("empty", func(t *testing.T) {
		var buf bytes.Buffer
		require.NoError(t, outputSeriesAnalysis(withOutput(context.Background(), &buf), &querierv1.QueryAnomaliesResponse{}, "json"))
		require.JSONEq(t, `{"timeSeriesAnomalies": []}`, buf.String())
	})
}
