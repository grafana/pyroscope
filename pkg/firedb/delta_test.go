package firedb

import (
	"sort"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	schemav1 "github.com/grafana/fire/pkg/firedb/schemas/v1"
	profilev1 "github.com/grafana/fire/pkg/gen/google/v1"
	"github.com/grafana/fire/pkg/pprof"
	"github.com/grafana/fire/pkg/pprof/testhelper"
)

func TestComputeDelta(t *testing.T) {
	delta := newDeltaProfiles()
	builder := testhelper.NewProfileBuilder(1).MemoryProfile()
	builder.ForStacktrace("a", "b", "c").AddSamples(1, 2, 3, 4)
	builder.ForStacktrace("a", "b", "c", "d").AddSamples(1, 2, 3, 4)
	delta.computeDelta(newProfileSchema(builder.Profile), nil)
}

func newProfileSchema(p *profilev1.Profile) *schemav1.Profile {
	ps := &schemav1.Profile{
		ID:                uuid.New(),
		TimeNanos:         p.TimeNanos,
		Comments:          p.Comment,
		DurationNanos:     p.DurationNanos,
		DropFrames:        p.DropFrames,
		KeepFrames:        p.KeepFrames,
		Period:            p.Period,
		DefaultSampleType: p.DefaultSampleType,
	}
	// todo build labels for profiles and externals labels.
	hashes := pprof.StacktracesHasher{}.Hashes(p.Sample)
	ps.Samples = make([]*schemav1.Sample, len(p.Sample))
	for i, s := range p.Sample {
		ps.Samples[i] = &schemav1.Sample{
			StacktraceID: hashes[i],
			Values:       copySlice(s.Value),
			Labels:       copySlice(s.Label),
		}
	}
	return ps
}

func TestDeltaSample(t *testing.T) {
	idx := []int{0, 1}
	new := []*schemav1.Sample{
		{StacktraceID: 2, Values: []int64{1, 2, 3, 4}},
		{StacktraceID: 3, Values: []int64{1, 2, 3, 4}},
	}
	highest := deltaSamples([]*schemav1.Sample{}, new, idx)
	require.Equal(t, 2, len(highest))
	require.Equal(t, []*schemav1.Sample{
		{StacktraceID: 2, Values: []int64{1, 2, 3, 4}},
		{StacktraceID: 3, Values: []int64{1, 2, 3, 4}},
	}, highest)
	require.Equal(t, highest, new)

	new = []*schemav1.Sample{
		{StacktraceID: 2, Values: []int64{1, 2, 3, 4}},
		{StacktraceID: 3, Values: []int64{1, 2, 3, 4}},
	}
	highest = deltaSamples(highest, new, idx)
	require.Equal(t, 2, len(highest))
	require.Equal(t, []*schemav1.Sample{
		{StacktraceID: 2, Values: []int64{1, 2, 3, 4}},
		{StacktraceID: 3, Values: []int64{1, 2, 3, 4}},
	}, highest)
	require.Equal(t, []*schemav1.Sample{
		{StacktraceID: 2, Values: []int64{0, 0, 3, 4}},
		{StacktraceID: 3, Values: []int64{0, 0, 3, 4}},
	}, new)

	new = []*schemav1.Sample{
		{StacktraceID: 3, Values: []int64{6, 2, 3, 4}},
		{StacktraceID: 5, Values: []int64{1, 5, 3, 4}},
	}
	highest = deltaSamples(highest, new, idx)
	require.Equal(t, []*schemav1.Sample{
		{StacktraceID: 2, Values: []int64{1, 2, 3, 4}},
		{StacktraceID: 3, Values: []int64{6, 2, 3, 4}},
		{StacktraceID: 5, Values: []int64{1, 5, 3, 4}},
	}, highest)
	require.Equal(t, []*schemav1.Sample{
		{StacktraceID: 3, Values: []int64{5, 0, 3, 4}},
		{StacktraceID: 5, Values: []int64{1, 5, 3, 4}},
	}, new)

	new = []*schemav1.Sample{
		{StacktraceID: 3, Values: []int64{1, 2, 3, 4}},
		{StacktraceID: 5, Values: []int64{0, 5, 3, 4}},
	}
	highest = deltaSamples(highest, new, idx)
	require.Equal(t, []*schemav1.Sample{
		{StacktraceID: 2, Values: []int64{1, 2, 3, 4}},
		{StacktraceID: 3, Values: []int64{6, 2, 3, 4}},
		{StacktraceID: 5, Values: []int64{1, 5, 3, 4}},
	}, highest)
	require.Equal(t, []*schemav1.Sample{
		{StacktraceID: 3, Values: []int64{0, 0, 3, 4}},
		{StacktraceID: 5, Values: []int64{0, 0, 3, 4}},
	}, new)

	new = []*schemav1.Sample{
		{StacktraceID: 0, Values: []int64{10, 20, 3, 4}},
		{StacktraceID: 1, Values: []int64{2, 3, 3, 4}},
		{StacktraceID: 7, Values: []int64{1, 1, 3, 4}},
	}
	highest = deltaSamples(highest, new, idx)
	sort.Slice(highest, func(i, j int) bool {
		return highest[i].StacktraceID < highest[j].StacktraceID
	})
	require.Equal(t, []*schemav1.Sample{
		{StacktraceID: 0, Values: []int64{10, 20, 3, 4}},
		{StacktraceID: 1, Values: []int64{2, 3, 3, 4}},
		{StacktraceID: 2, Values: []int64{1, 2, 3, 4}},
		{StacktraceID: 3, Values: []int64{6, 2, 3, 4}},
		{StacktraceID: 5, Values: []int64{1, 5, 3, 4}},
		{StacktraceID: 7, Values: []int64{1, 1, 3, 4}},
	}, highest)
	require.Equal(t, []*schemav1.Sample{
		{StacktraceID: 0, Values: []int64{10, 20, 3, 4}},
		{StacktraceID: 1, Values: []int64{2, 3, 3, 4}},
		{StacktraceID: 7, Values: []int64{1, 1, 3, 4}},
	}, new)
}
