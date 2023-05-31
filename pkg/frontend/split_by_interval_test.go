package frontend

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/grafana/phlare/pkg/iter"
)

func Test_TimeIntervalIterator(t *testing.T) {
	type testCase struct {
		description string
		inputRange  TimeInterval
		interval    time.Duration
		alignment   time.Duration
		expected    []TimeInterval
	}

	testCases := []testCase{
		{
			description: "misaligned time range",
			inputRange:  TimeInterval{time.Unix(0, 1), time.Unix(0, 3602)},
			interval:    900,
			expected: []TimeInterval{
				{time.Unix(0, 1), time.Unix(0, 899)},
				{time.Unix(0, 900), time.Unix(0, 1799)},
				{time.Unix(0, 1800), time.Unix(0, 2699)},
				{time.Unix(0, 2700), time.Unix(0, 3599)},
				{time.Unix(0, 3600), time.Unix(0, 3602)},
			},
		},
		{
			description: "misaligned time range with aligned interval",
			inputRange:  TimeInterval{time.UnixMilli(1684840541938), time.UnixMilli(1684848292171)},
			interval:    time.Minute * 15,
			alignment:   time.Second * 15,
			expected: []TimeInterval{
				{Start: time.Unix(0, 1684840541938000000), End: time.Unix(0, 1684841441937999999)},
				{Start: time.Unix(0, 1684841441938000000), End: time.Unix(0, 1684842341937999999)},
				{Start: time.Unix(0, 1684842341938000000), End: time.Unix(0, 1684843241937999999)},
				{Start: time.Unix(0, 1684843241938000000), End: time.Unix(0, 1684844141937999999)},
				{Start: time.Unix(0, 1684844141938000000), End: time.Unix(0, 1684845041937999999)},
				{Start: time.Unix(0, 1684845041938000000), End: time.Unix(0, 1684845941937999999)},
				{Start: time.Unix(0, 1684845941938000000), End: time.Unix(0, 1684846841937999999)},
				{Start: time.Unix(0, 1684846841938000000), End: time.Unix(0, 1684847741937999999)},
				{Start: time.Unix(0, 1684847741938000000), End: time.Unix(0, 1684848292171000000)},
			},
		},
		{
			description: "round range",
			inputRange:  TimeInterval{time.Unix(0, 0), time.Unix(0, 3600)},
			interval:    900,
			expected: []TimeInterval{
				{time.Unix(0, 0), time.Unix(0, 899)},
				{time.Unix(0, 900), time.Unix(0, 1799)},
				{time.Unix(0, 1800), time.Unix(0, 2699)},
				{time.Unix(0, 2700), time.Unix(0, 3600)},
			},
		},
		{
			description: "exact range",
			inputRange:  TimeInterval{time.Unix(0, 900), time.Unix(0, 1800)},
			interval:    900,
			expected:    []TimeInterval{{time.Unix(0, 900), time.Unix(0, 1800)}},
		},
		{
			description: "range less than interval",
			inputRange:  TimeInterval{time.Unix(0, 1), time.Unix(0, 501)},
			interval:    900,
			expected: []TimeInterval{
				{time.Unix(0, 1), time.Unix(0, 501)},
			},
		},
		{
			description: "range less than interval with alignment",
			inputRange:  TimeInterval{time.Unix(0, 1), time.Unix(0, 501)},
			interval:    900,
			alignment:   37,
			expected: []TimeInterval{
				{time.Unix(0, 1), time.Unix(0, 501)},
			},
		},
		{
			description: "range less than alignment",
			inputRange:  TimeInterval{time.Unix(0, 1), time.Unix(0, 501)},
			alignment:   900,
			interval:    90,
			expected: []TimeInterval{
				{time.Unix(0, 1), time.Unix(0, 501)},
			},
		},
		{
			description: "zero range",
			interval:    900,
		},
		{
			description: "zero range with alignment",
			interval:    900,
			alignment:   37,
		},
		{
			description: "zero interval",
			inputRange:  TimeInterval{time.Unix(0, 1), time.Unix(0, 501)},
			expected: []TimeInterval{
				{time.Unix(0, 1), time.Unix(0, 501)},
			},
		},
		{
			description: "zero interval with alignment",
			inputRange:  TimeInterval{time.Unix(0, 1), time.Unix(0, 501)},
			alignment:   37,
			expected: []TimeInterval{
				{time.Unix(0, 1), time.Unix(0, 501)},
			},
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.description, func(t *testing.T) {
			actual, err := iter.Slice[TimeInterval](NewTimeIntervalIterator(tc.inputRange.Start, tc.inputRange.End, tc.interval,
				WithAlignment(tc.alignment)))
			require.NoError(t, err)
			require.Equal(t, tc.expected, actual)
		})
	}
}

func Test_TimeIntervalIterator_MillisecondsTruncation(t *testing.T) {
	actual, err := iter.Slice[TimeInterval](NewTimeIntervalIterator(
		time.UnixMilli(51),
		time.UnixMilli(211),
		50*time.Millisecond))

	require.NoError(t, err)
	require.Len(t, actual, 4)
	require.Equal(t, int64(51), actual[0].Start.UnixMilli())
	require.Equal(t, int64(99), actual[0].End.UnixMilli())
	require.Equal(t, int64(100), actual[1].Start.UnixMilli())
	require.Equal(t, int64(149), actual[1].End.UnixMilli())
	require.Equal(t, int64(150), actual[2].Start.UnixMilli())
	require.Equal(t, int64(199), actual[2].End.UnixMilli())
	require.Equal(t, int64(200), actual[3].Start.UnixMilli())
	require.Equal(t, int64(211), actual[3].End.UnixMilli())
}
