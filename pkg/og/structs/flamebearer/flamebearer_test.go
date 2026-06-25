package flamebearer

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/grafana/pyroscope/v2/pkg/og/storage/metadata"
	"github.com/grafana/pyroscope/v2/pkg/og/storage/segment"
	"github.com/grafana/pyroscope/v2/pkg/og/storage/tree"
)

var (
	startTime     = int64(1635508310)
	durationDelta = int64(10)
	samples       = []uint64{1}
	watermarks    = map[int]int64{1: 1}
	maxNodes      = 1024
	spyName       = "spy-name"
	sampleRate    = uint32(10)
	units         = metadata.Units("units")
)

func TestFlamebearerProfile(t *testing.T) {
	t.Run("single sets all attributes correctly", func(t *testing.T) {
		tr := tree.New()
		tr.Insert([]byte("a;b"), uint64(1))
		tr.Insert([]byte("a;c"), uint64(2))

		timeline := &segment.Timeline{
			StartTime:               startTime,
			Samples:                 samples,
			DurationDeltaNormalized: durationDelta,
			Watermarks:              watermarks,
		}

		p := NewProfile(ProfileConfig{
			Name:     "name",
			Tree:     tr,
			MaxNodes: maxNodes,
			Timeline: timeline,
			Metadata: metadata.Metadata{
				SpyName:    spyName,
				SampleRate: sampleRate,
				Units:      units,
			},
		})

		require.Equal(t, []string{"total", "a", "c", "b"}, p.Flamebearer.Names)
		require.Equal(t, [][]int{
			{0, 3, 0, 0},
			{0, 3, 0, 1},
			{0, 1, 1, 3, 0, 2, 2, 2},
		}, p.Flamebearer.Levels)
		require.Equal(t, 3, p.Flamebearer.NumTicks)
		require.Equal(t, 2, p.Flamebearer.MaxSelf)

		require.Equal(t, "name", p.Metadata.Name)
		require.Equal(t, "single", p.Metadata.Format)
		require.Equal(t, spyName, p.Metadata.SpyName)
		require.Equal(t, sampleRate, p.Metadata.SampleRate)
		require.Equal(t, units, p.Metadata.Units)

		require.Equal(t, startTime, p.Timeline.StartTime)
		require.Equal(t, samples, p.Timeline.Samples)
		require.Equal(t, durationDelta, p.Timeline.DurationDelta)
		require.Equal(t, watermarks, p.Timeline.Watermarks)

		require.Zero(t, p.LeftTicks)
		require.Zero(t, p.RightTicks)

		require.NoError(t, p.Validate())
	})
}

func TestFlamebearerProfileValidation(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(*FlamebearerProfile)
		wantErr string
	}{
		{
			name: "unsupported version",
			setup: func(fb *FlamebearerProfile) {
				fb.Version = 2
			},
			wantErr: "unsupported version 2",
		},
		{
			name:    "unsupported format",
			setup:   func(_ *FlamebearerProfile) {},
			wantErr: "unsupported format ",
		},
		{
			name: "no names",
			setup: func(fb *FlamebearerProfile) {
				fb.Metadata.Format = "single"
			},
			wantErr: "a profile must have at least one symbol name",
		},
		{
			name: "no levels",
			setup: func(fb *FlamebearerProfile) {
				fb.Metadata.Format = "single"
				fb.Flamebearer.Names = []string{"name"}
			},
			wantErr: "a profile must have at least one profiling level",
		},
		{
			name: "invalid level size for single",
			setup: func(fb *FlamebearerProfile) {
				fb.Metadata.Format = "single"
				fb.Flamebearer.Names = []string{"name"}
				fb.Flamebearer.Levels = [][]int{{0, 0, 0, 0, 0, 0, 0}}
			},
			wantErr: "a profile level should have a multiple of 4 values, but there's a level with 7 values",
		},
		{
			name: "invalid level size for double",
			setup: func(fb *FlamebearerProfile) {
				fb.Metadata.Format = "double"
				fb.Flamebearer.Names = []string{"name"}
				fb.Flamebearer.Levels = [][]int{{0, 0, 0, 0}}
			},
			wantErr: "a profile level should have a multiple of 7 values, but there's a level with 4 values",
		},
		{
			name: "invalid name index single",
			setup: func(fb *FlamebearerProfile) {
				fb.Metadata.Format = "single"
				fb.Flamebearer.Names = []string{"name"}
				fb.Flamebearer.Levels = [][]int{{0, 0, 0, 1}}
			},
			wantErr: "invalid name index 1, it should be smaller than 1",
		},
		{
			name: "invalid name index double",
			setup: func(fb *FlamebearerProfile) {
				fb.Metadata.Format = "double"
				fb.Flamebearer.Names = []string{"name"}
				fb.Flamebearer.Levels = [][]int{{0, 0, 0, 0, 0, 0, 1}}
			},
			wantErr: "invalid name index 1, it should be smaller than 1",
		},
		{
			name: "negative name index",
			setup: func(fb *FlamebearerProfile) {
				fb.Metadata.Format = "single"
				fb.Flamebearer.Names = []string{"name"}
				fb.Flamebearer.Levels = [][]int{{0, 0, 0, -1}}
			},
			wantErr: "invalid name index -1, it should be a non-negative value",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var fb FlamebearerProfile
			tt.setup(&fb)
			require.EqualError(t, fb.Validate(), tt.wantErr)
		})
	}

	t.Run("valid single profile", func(t *testing.T) {
		var fb FlamebearerProfile
		fb.Metadata.Format = "single"
		fb.Flamebearer.Names = []string{"name"}
		fb.Flamebearer.Levels = [][]int{{0, 0, 0, 0}}
		require.NoError(t, fb.Validate())
	})

	t.Run("valid double profile", func(t *testing.T) {
		var fb FlamebearerProfile
		fb.Metadata.Format = "double"
		fb.Flamebearer.Names = []string{"name"}
		fb.Flamebearer.Levels = [][]int{{0, 0, 0, 0, 0, 0, 0}}
		require.NoError(t, fb.Validate())
	})
}
