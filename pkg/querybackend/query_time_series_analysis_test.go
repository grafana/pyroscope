package querybackend

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	"github.com/grafana/pyroscope/v2/pkg/model/timeseriesanalysis"
)

func TestQueryTimeSeriesAnalysis_InvalidStep(t *testing.T) {
	for _, tc := range []struct {
		name string
		step float64
	}{
		{"zero", 0},
		{"sub-millisecond", 0.0009},
		{"NaN", math.NaN()},
		{"positive infinity", math.Inf(1)},
		{"negative infinity", math.Inf(-1)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := queryTimeSeriesAnalysis(nil, &queryv1.Query{
				TimeSeriesAnalysis: &queryv1.TimeSeriesAnalysisQuery{Step: tc.step},
			})
			require.Error(t, err)
			assert.Equal(t, codes.InvalidArgument, status.Code(err))
		})
	}
}

func TestAnalysisEventType(t *testing.T) {
	for _, tc := range []struct {
		name  string
		input timeseriesanalysis.EventType
		want  querierv1.TimeSeriesAnomalyType
	}{
		{"spike", timeseriesanalysis.EventTypeSpike, querierv1.TimeSeriesAnomalyType_TIME_SERIES_ANOMALY_TYPE_SPIKE},
		{"drop", timeseriesanalysis.EventTypeDrop, querierv1.TimeSeriesAnomalyType_TIME_SERIES_ANOMALY_TYPE_DROP},
		{"increase", timeseriesanalysis.EventTypeIncrease, querierv1.TimeSeriesAnomalyType_TIME_SERIES_ANOMALY_TYPE_INCREASE},
		{"decrease", timeseriesanalysis.EventTypeDecrease, querierv1.TimeSeriesAnomalyType_TIME_SERIES_ANOMALY_TYPE_DECREASE},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, analysisEventType(tc.input))
		})
	}
}

func TestTimeSeriesAnalysisAggregator(t *testing.T) {
	query := &queryv1.TimeSeriesAnalysisQuery{
		Step:    1,
		GroupBy: []string{"service_name"},
		Config: &queryv1.TimeSeriesAnalysisConfig{
			BaselineWindow:         3,
			ConfirmationWindow:     3,
			MinimumSustainedPoints: 2,
			MinimumRelativeChange:  0.25,
			ScoreThreshold:         3.5,
			RecoveryThresholdRatio: 0.5,
			FlatnessThreshold:      0.01,
		},
	}
	agg := newTimeSeriesAnalysisAggregator(&queryv1.InvokeRequest{StartTime: 0, EndTime: 8000}).(*timeSeriesAnalysisAggregator)
	report := &queryv1.Report{TimeSeriesAnalysis: &queryv1.TimeSeriesAnalysisReport{
		Query: query,
		TimeSeries: []*typesv1.Series{{
			Labels: []*typesv1.LabelPair{{Name: "service_name", Value: "api"}},
			Points: []*typesv1.Point{
				{Timestamp: 1000, Value: 100},
				{Timestamp: 2000, Value: 100},
				{Timestamp: 3000, Value: 100},
				{Timestamp: 4000, Value: 200},
				{Timestamp: 5000, Value: 105},
				{Timestamp: 6000, Value: 100},
				{Timestamp: 7000, Value: 100},
			},
		}},
	}}
	require.NoError(t, agg.aggregate(report))

	result := agg.build().TimeSeriesAnalysis
	require.Len(t, result.TimeSeries, 1)
	// Even a complete event is not analyzed during intermediate aggregation.
	require.Empty(t, result.Events)
	require.NoError(t, finalizeTimeSeriesAnalysis(result))
	require.Empty(t, result.TimeSeries)
	require.Len(t, result.Events, 1)
	event := result.Events[0]
	assert.Equal(t, querierv1.TimeSeriesAnomalyType_TIME_SERIES_ANOMALY_TYPE_SPIKE, event.Type)
	assert.Equal(t, int64(4000), event.TimestampStart)
	assert.Equal(t, int64(4000), event.TimestampEnd)
	assert.Equal(t, "api", event.Labels[0].Value)
}
