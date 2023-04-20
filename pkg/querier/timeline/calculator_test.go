package timeline_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/grafana/phlare/pkg/querier/timeline"
)

func Test_CalcPointInterval(t *testing.T) {
	TestDate := time.Date(2023, time.April, 18, 1, 2, 3, 4, time.UTC)

	testCases := []struct {
		name  string
		start time.Time
		end   time.Time
		want  int64
	}{
		{name: "1 second", start: TestDate, end: TestDate.Add(1 * time.Second), want: 10},
		{name: "1 hour", start: TestDate, end: TestDate.Add(1 * time.Hour), want: 10},
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
			got := timeline.CalcPointInterval(tc.start.UnixMilli(), tc.end.UnixMilli())

			assert.Equal(t, float64(tc.want), got)
		})
	}

}
