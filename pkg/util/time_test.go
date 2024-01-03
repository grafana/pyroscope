package util

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func Test_ForResolutions(t *testing.T) {
	start := mustTime("15:04:01")
	end := mustTime("20:09:59")

	resolutions := []time.Duration{
		time.Hour,
		time.Minute * 5,
	}

	expected := []TimeRange{
		{Start: mustTime("15:04:01"), End: mustTime("15:05:00").Add(-time.Millisecond), Resolution: -1},
		{Start: mustTime("15:05:00"), End: mustTime("16:00:00").Add(-time.Millisecond), Resolution: time.Minute * 5},
		{Start: mustTime("16:00:00"), End: mustTime("20:00:00").Add(-time.Millisecond), Resolution: time.Hour},
		{Start: mustTime("20:00:00"), End: mustTime("20:05:00").Add(-time.Millisecond), Resolution: time.Minute * 5},
		{Start: mustTime("20:05:00"), End: mustTime("20:09:59"), Resolution: -1},
	}

	actual := make([]TimeRange, 0, len(expected))
	SplitTimeRangeByResolution(start, end, resolutions, func(r TimeRange) {
		actual = append(actual, r)
	})

	assert.Equal(t, expected, actual)
}

func mustTime(s string) time.Time {
	t, err := time.Parse(time.TimeOnly, s)
	if err != nil {
		panic(err)
	}
	return t.AddDate(1970, 1, 1)
}
