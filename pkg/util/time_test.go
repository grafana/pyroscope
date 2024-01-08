package util

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func Test_SplitTimeRangeByResolution(t *testing.T) {
	type testCase struct {
		desc        string
		start       time.Time
		end         time.Time
		resolutions []time.Duration
		expected    []TimeRange
	}

	resolutions := []time.Duration{
		time.Hour,
		time.Minute * 5,
	}

	testCases := []testCase{
		{
			start:       mustTime("15:04:59"),
			end:         mustTime("15:05:01"),
			resolutions: resolutions,
			expected: []TimeRange{
				{Start: mustTime("15:04:59"), End: mustTime("15:05:01"), Resolution: -1},
			},
		},
		{
			start:       mustTime("15:04:59"),
			end:         mustTime("15:09:59"),
			resolutions: resolutions,
			expected: []TimeRange{
				{Start: mustTime("15:04:59"), End: mustTime("15:09:59"), Resolution: -1},
			},
		},
		{
			start:       mustTime("15:05:00"),
			end:         mustTime("15:09:59"),
			resolutions: resolutions,
			expected: []TimeRange{
				{Start: mustTime("15:05:00"), End: mustTime("15:09:59"), Resolution: -1},
			},
		},

		{
			start:       mustTime("15:05:00"),
			end:         mustTime("15:10:00").Add(-time.Millisecond),
			resolutions: resolutions,
			expected: []TimeRange{
				{Start: mustTime("15:05:00"), End: mustTime("15:10:00").Add(-time.Millisecond), Resolution: time.Minute * 5},
			},
		},
		{
			start:       mustTime("15:05:00"),
			end:         mustTime("15:15:00").Add(-time.Millisecond),
			resolutions: resolutions,
			expected: []TimeRange{
				{Start: mustTime("15:05:00"), End: mustTime("15:15:00").Add(-time.Millisecond), Resolution: time.Minute * 5},
			},
		},

		{
			start:       mustTime("15:00:00"),
			end:         mustTime("16:00:00").Add(-time.Millisecond),
			resolutions: resolutions,
			expected: []TimeRange{
				{Start: mustTime("15:00:00"), End: mustTime("16:00:00").Add(-time.Millisecond), Resolution: time.Hour},
			},
		},
		{
			start:       mustTime("15:00:00"),
			end:         mustTime("19:00:00").Add(-time.Millisecond),
			resolutions: resolutions,
			expected: []TimeRange{
				{Start: mustTime("15:00:00"), End: mustTime("19:00:00").Add(-time.Millisecond), Resolution: time.Hour},
			},
		},

		{
			start:       mustTime("15:00:00"),
			end:         mustTime("16:05:00").Add(-time.Millisecond),
			resolutions: resolutions,
			expected: []TimeRange{
				{Start: mustTime("15:00:00"), End: mustTime("16:00:00").Add(-time.Millisecond), Resolution: time.Hour},
				{Start: mustTime("16:00:00"), End: mustTime("16:05:00").Add(-time.Millisecond), Resolution: time.Minute * 5},
			},
		},
		{
			start:       mustTime("14:00:00"),
			end:         mustTime("16:05:00").Add(-time.Millisecond),
			resolutions: resolutions,
			expected: []TimeRange{
				{Start: mustTime("14:00:00"), End: mustTime("16:00:00").Add(-time.Millisecond), Resolution: time.Hour},
				{Start: mustTime("16:00:00"), End: mustTime("16:05:00").Add(-time.Millisecond), Resolution: time.Minute * 5},
			},
		},
		{
			start:       mustTime("14:00:00"),
			end:         mustTime("16:10:00").Add(-time.Millisecond),
			resolutions: resolutions,
			expected: []TimeRange{
				{Start: mustTime("14:00:00"), End: mustTime("16:00:00").Add(-time.Millisecond), Resolution: time.Hour},
				{Start: mustTime("16:00:00"), End: mustTime("16:10:00").Add(-time.Millisecond), Resolution: time.Minute * 5},
			},
		},
		{
			start:       mustTime("15:00:00"),
			end:         mustTime("16:10:00").Add(-time.Millisecond),
			resolutions: resolutions,
			expected: []TimeRange{
				{Start: mustTime("15:00:00"), End: mustTime("16:00:00").Add(-time.Millisecond), Resolution: time.Hour},
				{Start: mustTime("16:00:00"), End: mustTime("16:10:00").Add(-time.Millisecond), Resolution: time.Minute * 5},
			},
		},

		{
			start:       mustTime("15:04:01"),
			end:         mustTime("20:09:59"),
			resolutions: resolutions,
			expected: []TimeRange{
				{Start: mustTime("15:04:01"), End: mustTime("15:05:00").Add(-time.Millisecond), Resolution: -1},
				{Start: mustTime("15:05:00"), End: mustTime("16:00:00").Add(-time.Millisecond), Resolution: time.Minute * 5},
				{Start: mustTime("16:00:00"), End: mustTime("20:00:00").Add(-time.Millisecond), Resolution: time.Hour},
				{Start: mustTime("20:00:00"), End: mustTime("20:05:00").Add(-time.Millisecond), Resolution: time.Minute * 5},
				{Start: mustTime("20:05:00"), End: mustTime("20:09:59"), Resolution: -1},
			},
		},
		{
			start:       mustTime("15:00:00"),
			end:         mustTime("20:09:59"),
			resolutions: resolutions,
			expected: []TimeRange{
				{Start: mustTime("15:00:00"), End: mustTime("20:00:00").Add(-time.Millisecond), Resolution: time.Hour},
				{Start: mustTime("20:00:00"), End: mustTime("20:05:00").Add(-time.Millisecond), Resolution: time.Minute * 5},
				{Start: mustTime("20:05:00"), End: mustTime("20:09:59"), Resolution: -1},
			},
		},
		{
			start:       mustTime("15:04:01"),
			end:         mustTime("20:00:00").Add(-time.Millisecond),
			resolutions: resolutions,
			expected: []TimeRange{
				{Start: mustTime("15:04:01"), End: mustTime("15:05:00").Add(-time.Millisecond), Resolution: -1},
				{Start: mustTime("15:05:00"), End: mustTime("16:00:00").Add(-time.Millisecond), Resolution: time.Minute * 5},
				{Start: mustTime("16:00:00"), End: mustTime("20:00:00").Add(-time.Millisecond), Resolution: time.Hour},
			},
		},
		{
			start:       mustDateTime("2006-01-01 15:04:01"),
			end:         mustDateTime("2006-01-03 20:09:59"),
			resolutions: resolutions,
			expected: []TimeRange{
				{Start: mustDateTime("2006-01-01 15:04:01"), End: mustDateTime("2006-01-01 15:05:00").Add(-time.Millisecond), Resolution: -1},
				{Start: mustDateTime("2006-01-01 15:05:00"), End: mustDateTime("2006-01-01 16:00:00").Add(-time.Millisecond), Resolution: time.Minute * 5},
				{Start: mustDateTime("2006-01-01 16:00:00"), End: mustDateTime("2006-01-03 20:00:00").Add(-time.Millisecond), Resolution: time.Hour},
				{Start: mustDateTime("2006-01-03 20:00:00"), End: mustDateTime("2006-01-03 20:05:00").Add(-time.Millisecond), Resolution: time.Minute * 5},
				{Start: mustDateTime("2006-01-03 20:05:00"), End: mustDateTime("2006-01-03 20:09:59"), Resolution: -1},
			},
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.desc, func(t *testing.T) {
			actual := make([]TimeRange, 0, len(tc.expected))
			SplitTimeRangeByResolution(tc.start, tc.end, tc.resolutions, func(r TimeRange) {
				actual = append(actual, r)
				t.Log(r)
			})
			assert.Equal(t, tc.expected, actual)
		})
	}
}

func mustTime(s string) time.Time {
	t, err := time.Parse(time.TimeOnly, s)
	if err != nil {
		panic(err)
	}
	return t.AddDate(1970, 1, 1)
}

func mustDateTime(s string) time.Time {
	t, err := time.Parse(time.DateTime, s)
	if err != nil {
		panic(err)
	}
	return t
}
