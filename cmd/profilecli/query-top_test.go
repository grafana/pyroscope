package main

import (
	"testing"

	"github.com/stretchr/testify/require"

	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
)

func TestSumPointsAfter(t *testing.T) {
	const startMs = int64(1_784_130_300_000) // window start
	const stepMs = int64(3_600_000)

	// SelectSeries with step = window size returns two points: the boundary
	// point at `start` holding pre-window data, and the point at `end`
	// holding the actual window total.
	points := []*typesv1.Point{
		{Timestamp: startMs, Value: 8_778_052_796_311},
		{Timestamp: startMs + stepMs, Value: 4_005_116_752_500},
	}
	require.Equal(t, float64(4_005_116_752_500), sumPointsAfter(points, startMs))

	t.Run("all points within window", func(t *testing.T) {
		points := []*typesv1.Point{
			{Timestamp: startMs + 1, Value: 1},
			{Timestamp: startMs + 2, Value: 2},
		}
		require.Equal(t, float64(3), sumPointsAfter(points, startMs))
	})

	t.Run("no points", func(t *testing.T) {
		require.Equal(t, float64(0), sumPointsAfter(nil, startMs))
	})
}
