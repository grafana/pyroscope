package timeseriesanalysis

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
)

func TestAnalyze(t *testing.T) {
	config := Config{
		BaselineWindow:         3,
		ConfirmationWindow:     3,
		MinimumSustainedPoints: 2,
		MinimumRelativeChange:  0.25,
		ScoreThreshold:         3.5,
		RecoveryThresholdRatio: 0.5,
		FlatnessThreshold:      0.01,
	}

	tests := []struct {
		name   string
		values []float64
		want   []Event
	}{
		{
			name:   "flat series produces no events",
			values: []float64{100, 100, 100, 100, 100, 100, 100},
		},
		{
			name:   "short positive deviation is a spike",
			values: []float64{100, 100, 100, 200, 105, 100, 100},
			want: []Event{{
				Type: EventTypeSpike, Duration: DurationShortTerm, StartTime: 4000, EndTime: 4000, PeakTime: 4000,
				Baseline: 100, PeakValue: 200, RelativeChange: 1, Score: 100,
			}},
		},
		{
			name:   "short negative deviation is a drop",
			values: []float64{100, 100, 100, 30, 100, 100, 100},
			want: []Event{{
				Type: EventTypeDrop, Duration: DurationShortTerm, StartTime: 4000, EndTime: 4000, PeakTime: 4000,
				Baseline: 100, PeakValue: 30, RelativeChange: 0.7, Score: 70,
			}},
		},
		{
			name:   "sustained negative deviation is a decrease",
			values: []float64{100, 100, 100, 30, 35, 100, 100},
			want: []Event{{
				Type: EventTypeDecrease, Duration: DurationSustained, StartTime: 4000, EndTime: 5000, PeakTime: 4000,
				Baseline: 100, PeakValue: 30, RelativeChange: 0.7, Score: 70,
			}},
		},
		{
			name:   "sustained positive deviation is an increase",
			values: []float64{100, 100, 100, 140, 150, 160, 100},
			want: []Event{{
				Type: EventTypeIncrease, Duration: DurationSustained, StartTime: 4000, EndTime: 6000, PeakTime: 6000,
				Baseline: 100, PeakValue: 160, RelativeChange: 0.6, Score: 60,
			}},
		},
		{
			name:   "noisy baseline adjusts spike score",
			values: []float64{90, 100, 110, 200, 100, 100, 100},
			want: []Event{{
				Type: EventTypeSpike, Duration: DurationShortTerm, StartTime: 4000, EndTime: 4000, PeakTime: 4000,
				Baseline: 100, PeakValue: 200, RelativeChange: 1, Score: 100 / 14.826,
			}},
		},
		{
			name:   "noisy baseline adjusts later positive peak score",
			values: []float64{90, 100, 110, 160, 180, 200, 100},
			want: []Event{{
				Type: EventTypeIncrease, Duration: DurationSustained, StartTime: 4000, EndTime: 6000, PeakTime: 6000,
				Baseline: 100, PeakValue: 200, RelativeChange: 1, Score: 100 / 14.826,
			}},
		},
		{
			name:   "noisy baseline adjusts later negative peak score",
			values: []float64{90, 100, 110, 40, 30, 20, 100},
			want: []Event{{
				Type: EventTypeDecrease, Duration: DurationSustained, StartTime: 4000, EndTime: 6000, PeakTime: 6000,
				Baseline: 100, PeakValue: 20, RelativeChange: 0.8, Score: 80 / 14.826,
			}},
		},
		{
			name:   "zero baseline uses a finite score floor",
			values: []float64{0, 0, 0, 10, 0, 0, 0},
			want: []Event{{
				Type: EventTypeSpike, Duration: DurationShortTerm, StartTime: 4000, EndTime: 4000, PeakTime: 4000,
				Baseline: 0, PeakValue: 10, RelativeChange: 10, Score: 10,
			}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			events, err := Analyze([]*typesv1.Series{series("api", tt.values...)}, time.Second, config)
			require.NoError(t, err)
			require.Len(t, events, len(tt.want))
			for i := range tt.want {
				assert.Equal(t, tt.want[i].Type, events[i].Type)
				assert.Equal(t, tt.want[i].Duration, events[i].Duration)
				assert.Equal(t, tt.want[i].StartTime, events[i].StartTime)
				assert.Equal(t, tt.want[i].EndTime, events[i].EndTime)
				assert.Equal(t, tt.want[i].PeakTime, events[i].PeakTime)
				assert.InDelta(t, tt.want[i].Baseline, events[i].Baseline, 0.0001)
				assert.InDelta(t, tt.want[i].PeakValue, events[i].PeakValue, 0.0001)
				assert.InDelta(t, tt.want[i].RelativeChange, events[i].RelativeChange, 0.0001)
				assert.InDelta(t, tt.want[i].Score, events[i].Score, 0.0001)
			}
		})
	}
}

func TestAnalyze_GapResetsBaseline(t *testing.T) {
	series := &typesv1.Series{Points: []*typesv1.Point{
		{Timestamp: 1000, Value: 100},
		{Timestamp: 2000, Value: 100},
		{Timestamp: 3000, Value: 100},
		{Timestamp: 6000, Value: 200},
		{Timestamp: 7000, Value: 100},
		{Timestamp: 8000, Value: 100},
		{Timestamp: 9000, Value: 100},
	}}
	events, err := Analyze([]*typesv1.Series{series}, time.Second, Config{
		BaselineWindow: 3, ConfirmationWindow: 2, MinimumSustainedPoints: 2,
		MinimumRelativeChange: 0.25, ScoreThreshold: 3.5, RecoveryThresholdRatio: 0.5, FlatnessThreshold: 0.01,
	})
	require.NoError(t, err)
	assert.Empty(t, events)
}

func TestTopEvents(t *testing.T) {
	low := Event{Labels: labels("service", "low"), StartTime: 1000, Score: 5}
	high := Event{Labels: labels("service", "high"), StartTime: 2000, Score: 10}
	moreHigh := Event{Labels: labels("service", "high"), StartTime: 3000, Score: 6}

	events := TopEvents([]Event{low, high, moreHigh}, 1)
	require.Len(t, events, 2)
	assert.Equal(t, "high", events[0].Labels[0].Value)
	assert.Equal(t, "high", events[1].Labels[0].Value)
}

func TestConfigValidate(t *testing.T) {
	config := DefaultConfig()
	require.NoError(t, config.Validate())
	config.ScoreThreshold = 0
	assert.ErrorContains(t, config.Validate(), "score threshold")
}

func series(service string, values ...float64) *typesv1.Series {
	points := make([]*typesv1.Point, len(values))
	for i, value := range values {
		points[i] = &typesv1.Point{Timestamp: int64(i+1) * 1000, Value: value}
	}
	return &typesv1.Series{Labels: labels("service_name", service), Points: points}
}

func labels(name, value string) []*typesv1.LabelPair {
	return []*typesv1.LabelPair{{Name: name, Value: value}}
}
