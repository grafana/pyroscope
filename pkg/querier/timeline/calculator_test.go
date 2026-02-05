package timeline_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/grafana/pyroscope/pkg/querier/timeline"
)

func Test_CalcPointInterval(t *testing.T) {
	TestDate := time.Date(2023, time.April, 18, 1, 2, 3, 4, time.UTC)
	defaultMinStepDuration := 15 * time.Second

	testCases := []struct {
		name  string
		start time.Time
		end   time.Time
		want  int64
	}{
		{name: "1 second", start: TestDate, end: TestDate.Add(1 * time.Second), want: 15},
		{name: "1 hour", start: TestDate, end: TestDate.Add(1 * time.Hour), want: 15},
		{name: "7 days", start: TestDate, end: TestDate.Add(7 * 24 * time.Hour), want: 300},
		{name: "30 days", start: TestDate, end: TestDate.Add(30 * 24 * time.Hour), want: 1800},
		{name: "90 days", start: TestDate, end: TestDate.Add(30 * 24 * time.Hour), want: 1800},
		{name: "~6 months", start: TestDate, end: TestDate.Add(6 * 30 * 24 * time.Hour), want: 10800},
		{name: "~1 year", start: TestDate, end: TestDate.Add(12 * 30 * 24 * time.Hour), want: 21600},
		{name: "~2 years", start: TestDate, end: TestDate.Add(2 * 12 * 30 * 24 * time.Hour), want: 43200},
		{name: "~5 years", start: TestDate, end: TestDate.Add(5 * 12 * 30 * 24 * time.Hour), want: 86400},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := timeline.CalcPointInterval(tc.start.UnixMilli(), tc.end.UnixMilli(), defaultMinStepDuration)

			assert.Equal(t, float64(tc.want), got)
		})
	}

}

func Test_CalcPointInterval_WithCustomMinStepDuration(t *testing.T) {
	TestDate := time.Date(2023, time.April, 18, 1, 2, 3, 4, time.UTC)

	testCases := []struct {
		name            string
		start           time.Time
		end             time.Time
		minStepDuration time.Duration
		want            float64
	}{
		{
			name:            "1 second with 5s min step duration",
			start:           TestDate,
			end:             TestDate.Add(1 * time.Second),
			minStepDuration: 5 * time.Second,
			want:            5.0,
		},
		{
			name:            "1 second with 30s min step duration",
			start:           TestDate,
			end:             TestDate.Add(1 * time.Second),
			minStepDuration: 30 * time.Second,
			want:            30.0,
		},
		{
			name:            "1 hour with 5s min step duration",
			start:           TestDate,
			end:             TestDate.Add(1 * time.Hour),
			minStepDuration: 5 * time.Second,
			want:            5.0,
		},
		{
			name:            "1 hour with 1m min step duration",
			start:           TestDate,
			end:             TestDate.Add(1 * time.Hour),
			minStepDuration: 1 * time.Minute,
			want:            60.0,
		},
		{
			name:            "7 days with 1m min step duration",
			start:           TestDate,
			end:             TestDate.Add(7 * 24 * time.Hour),
			minStepDuration: 1 * time.Minute,
			want:            300.0, // calculated interval is 5m, which is > 1m min
		},
		{
			name:            "7 days with 10m min step duration",
			start:           TestDate,
			end:             TestDate.Add(7 * 24 * time.Hour),
			minStepDuration: 10 * time.Minute,
			want:            600.0, // min step duration enforced (10m)
		},
		{
			name:            "30 days with default min step duration",
			start:           TestDate,
			end:             TestDate.Add(30 * 24 * time.Hour),
			minStepDuration: 15 * time.Second,
			want:            1800.0, // calculated interval is 30m
		},
		{
			name:            "30 days with 1h min step duration",
			start:           TestDate,
			end:             TestDate.Add(30 * 24 * time.Hour),
			minStepDuration: 1 * time.Hour,
			want:            3600.0, // min step duration enforced (1h)
		},
		{
			name:            "1 year with 5m min step duration",
			start:           TestDate,
			end:             TestDate.Add(365 * 24 * time.Hour),
			minStepDuration: 5 * time.Minute,
			want:            21600.0, // calculated interval is 6h
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := timeline.CalcPointInterval(tc.start.UnixMilli(), tc.end.UnixMilli(), tc.minStepDuration)

			assert.Equal(t, tc.want, got, "expected %v seconds, got %v seconds", tc.want, got)
		})
	}
}
