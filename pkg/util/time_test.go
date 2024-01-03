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

	type timeRange struct {
		start, end time.Time
		resolution time.Duration
	}

	expected := []timeRange{
		{start: mustTime("15:04:01"), end: mustTime("15:05:00")},
		{start: mustTime("15:05:00"), end: mustTime("16:00:00")},
		{start: mustTime("16:00:00"), end: mustTime("20:00:00")},
		{start: mustTime("20:00:00"), end: mustTime("20:05:00")},
		{start: mustTime("20:05:00"), end: mustTime("20:09:59")},
	}

	actual := make([]timeRange, 0, len(expected))
	ForResolutions(start, end, resolutions, func(start, end time.Time) {
		actual = append(actual, timeRange{
			start: start,
			end:   end,
		})
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
