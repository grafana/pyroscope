package phlaredb

import (
	"testing"

	"github.com/stretchr/testify/require"

	schemav1 "github.com/grafana/pyroscope/pkg/phlaredb/schemas/v1"
	schemav1testhelper "github.com/grafana/pyroscope/pkg/phlaredb/schemas/v1/testhelper"
	"github.com/grafana/pyroscope/pkg/pprof/testhelper"
)

func TestComputeDelta(t *testing.T) {
	delta := newDeltaProfiles()
	builder := testhelper.NewProfileBuilder(1).MemoryProfile()
	builder.ForStacktraceString("a", "b", "c").AddSamples(1, 2, 3, 4)
	builder.ForStacktraceString("a", "b", "c", "d").AddSamples(1, 2, 3, 4)

	profiles, _ := schemav1testhelper.NewProfileSchema(builder.Profile, "memory")

	samples := delta.computeDelta(profiles[0])
	require.Empty(t, samples.StacktraceIDs)
	samples = delta.computeDelta(profiles[1])
	require.Empty(t, samples.StacktraceIDs)

	builder = testhelper.NewProfileBuilder(1).MemoryProfile()
	builder.ForStacktraceString("a", "b", "c").AddSamples(2, 4, 3, 4)
	builder.ForStacktraceString("a", "b", "c", "d").AddSamples(2, 4, 3, 4)

	profiles, _ = schemav1testhelper.NewProfileSchema(builder.Profile, "memory")
	samples = delta.computeDelta(profiles[0])
	require.NotEmpty(t, samples.StacktraceIDs)
	samples = delta.computeDelta(profiles[1])
	require.NotEmpty(t, samples.StacktraceIDs)
}

func TestDeltaSample(t *testing.T) {
	new := schemav1.Samples{
		StacktraceIDs: []uint32{2, 3},
		Values:        []uint64{1, 1},
	}
	highest := map[uint32]uint64{}
	_ = deltaSamples(highest, new)
	require.Equal(t, 2, len(highest))
	require.Equal(t, map[uint32]uint64{
		2: 1,
		3: 1,
	}, highest)

	t.Run("same stacktraces, matching counter samples, matching gauge samples", func(t *testing.T) {
		new = schemav1.Samples{
			StacktraceIDs: []uint32{2, 3},
			Values:        []uint64{1, 1},
		}
		_ = deltaSamples(highest, new)
		require.Equal(t, 2, len(highest))
		require.Equal(t, map[uint32]uint64{
			2: 1,
			3: 1,
		}, highest)
		require.Equal(t, schemav1.Samples{
			StacktraceIDs: []uint32{2, 3},
			Values:        []uint64{0, 0},
		}, new)
	})

	t.Run("same stacktraces, matching counter samples, empty gauge samples", func(t *testing.T) {
		new = schemav1.Samples{
			StacktraceIDs: []uint32{2, 3},
			Values:        []uint64{1, 1},
		}
		_ = deltaSamples(highest, new)
		require.Equal(t, 2, len(highest))
		require.Equal(t, map[uint32]uint64{
			2: 1,
			3: 1,
		}, highest)
		require.Equal(t, schemav1.Samples{
			StacktraceIDs: []uint32{2, 3},
			Values:        []uint64{0, 0},
		}, new)
	})

	t.Run("new stacktrace, and increase counter in existing stacktrace", func(t *testing.T) {
		new = schemav1.Samples{
			StacktraceIDs: []uint32{3, 5},
			Values:        []uint64{6, 1},
		}
		_ = deltaSamples(highest, new)
		require.Equal(t, map[uint32]uint64{
			2: 1,
			3: 6,
			5: 1,
		}, highest)
	})

	t.Run("same stacktraces, counter samples resetting", func(t *testing.T) {
		new = schemav1.Samples{
			StacktraceIDs: []uint32{3, 5},
			Values:        []uint64{0, 1},
		}
		reset := deltaSamples(highest, new)
		require.True(t, reset)
		require.Equal(t, map[uint32]uint64{
			2: 1,
			3: 6,
			5: 1,
		}, highest)
	})

	t.Run("two new stacktraces, raise counters of existing stacktrace", func(t *testing.T) {
		new = schemav1.Samples{
			StacktraceIDs: []uint32{0, 1, 7},
			Values:        []uint64{10, 2, 1},
		}

		_ = deltaSamples(highest, new)
		require.Equal(t, map[uint32]uint64{
			0: 10,
			1: 2,
			2: 1,
			3: 6,
			5: 1,
			7: 1,
		}, highest)

		require.Equal(t, schemav1.Samples{
			StacktraceIDs: []uint32{0, 1, 7},
			Values:        []uint64{10, 2, 1},
		}, new)
	})
}
