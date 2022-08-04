package firedb

import (
	"sort"
	"testing"

	"github.com/google/uuid"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"

	schemav1 "github.com/grafana/fire/pkg/firedb/schemas/v1"
	commonv1 "github.com/grafana/fire/pkg/gen/common/v1"
	profilev1 "github.com/grafana/fire/pkg/gen/google/v1"
	firemodel "github.com/grafana/fire/pkg/model"
	"github.com/grafana/fire/pkg/pprof"
	"github.com/grafana/fire/pkg/pprof/testhelper"
)

func TestComputeDelta(t *testing.T) {
	delta := newDeltaProfiles()
	builder := testhelper.NewProfileBuilder(1).MemoryProfile()
	builder.ForStacktrace("a", "b", "c").AddSamples(1, 2, 3, 4)
	builder.ForStacktrace("a", "b", "c", "d").AddSamples(1, 2, 3, 4)

	newProfile, newLabels := delta.computeDelta(newProfileSchema(builder.Profile, "memory"))
	require.Equal(t, 2, len(newProfile.Samples))
	require.Equal(t, 2, len(newLabels))
	require.Equal(t, 2, len(newProfile.SeriesRefs))
	require.Equal(t, []int64{3, 4}, newProfile.Samples[0].Values)
	require.Equal(t, []int64{3, 4}, newProfile.Samples[1].Values)
	require.Equal(t, "memory:inuse_objects:count:space:bytes", newLabels[0].Get(firemodel.LabelNameProfileType))
	require.Equal(t, "memory:inuse_space:bytes:space:bytes", newLabels[1].Get(firemodel.LabelNameProfileType))

	newProfile, newLabels = delta.computeDelta(newProfileSchema(builder.Profile, "memory"))
	require.Equal(t, 2, len(newProfile.Samples))
	require.Equal(t, 4, len(newProfile.SeriesRefs))
	require.Equal(t, 4, len(newLabels))
	require.Equal(t, []int64{0, 0, 3, 4}, newProfile.Samples[0].Values)
	require.Equal(t, []int64{0, 0, 3, 4}, newProfile.Samples[1].Values)
	require.Equal(t, "memory:alloc_objects:count:space:bytes", newLabels[0].Get(firemodel.LabelNameProfileType))
	require.Equal(t, "memory:alloc_space:bytes:space:bytes", newLabels[1].Get(firemodel.LabelNameProfileType))
	require.Equal(t, "memory:inuse_objects:count:space:bytes", newLabels[2].Get(firemodel.LabelNameProfileType))
	require.Equal(t, "memory:inuse_space:bytes:space:bytes", newLabels[3].Get(firemodel.LabelNameProfileType))
}

func newProfileSchema(p *profilev1.Profile, name string) (*schemav1.Profile, []firemodel.Labels) {
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
	labels, seriesRefs := labelsForProfile(p, &commonv1.LabelPair{Name: model.MetricNameLabel, Value: name})
	hashes := pprof.StacktracesHasher{}.Hashes(p.Sample)
	ps.Samples = make([]*schemav1.Sample, len(p.Sample))
	for i, s := range p.Sample {
		ps.Samples[i] = &schemav1.Sample{
			StacktraceID: hashes[i],
			Values:       copySlice(s.Value),
			Labels:       copySlice(s.Label),
		}
	}
	ps.SeriesRefs = seriesRefs
	return ps, labels
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

	t.Run("same stacktraces, matching counter samples, matching gauge samples", func(t *testing.T) {
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
	})

	t.Run("same stacktraces, matching counter samples, empty gauge samples", func(t *testing.T) {
		new = []*schemav1.Sample{
			{StacktraceID: 2, Values: []int64{1, 2, 0, 0}},
			{StacktraceID: 3, Values: []int64{1, 2, 0, 0}},
		}
		highest = deltaSamples(highest, new, idx)
		require.Equal(t, 2, len(highest))
		require.Equal(t, []*schemav1.Sample{
			{StacktraceID: 2, Values: []int64{1, 2, 3, 4}},
			{StacktraceID: 3, Values: []int64{1, 2, 3, 4}},
		}, highest)
		require.Equal(t, []*schemav1.Sample{
			{StacktraceID: 2, Values: []int64{0, 0, 0, 0}},
			{StacktraceID: 3, Values: []int64{0, 0, 0, 0}},
		}, new)
	})

	t.Run("new stacktrace, and increase counter in existing stacktrace", func(t *testing.T) {
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
	})

	t.Run("same stacktraces, counter samples resetting", func(t *testing.T) {
		new = []*schemav1.Sample{
			{StacktraceID: 3, Values: []int64{1, 2, 3, 4}},
			{StacktraceID: 5, Values: []int64{0, 5, 3, 4}},
		}
		highest = deltaSamples(highest, new, idx)
		require.Equal(t, []*schemav1.Sample{
			{StacktraceID: 2, Values: []int64{1, 2, 3, 4}},
			{StacktraceID: 3, Values: []int64{1, 2, 3, 4}},
			{StacktraceID: 5, Values: []int64{0, 5, 3, 4}},
		}, highest)
		require.Equal(t, []*schemav1.Sample{
			{StacktraceID: 3, Values: []int64{1, 0, 3, 4}},
			{StacktraceID: 5, Values: []int64{0, 0, 3, 4}},
		}, new)
	})

	t.Run("two new stacktraces, raise counters of existing stacktrace", func(t *testing.T) {
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
			{StacktraceID: 3, Values: []int64{1, 2, 3, 4}},
			{StacktraceID: 5, Values: []int64{0, 5, 3, 4}},
			{StacktraceID: 7, Values: []int64{1, 1, 3, 4}},
		}, highest)
		require.Equal(t, []*schemav1.Sample{
			{StacktraceID: 0, Values: []int64{10, 20, 3, 4}},
			{StacktraceID: 1, Values: []int64{2, 3, 3, 4}},
			{StacktraceID: 7, Values: []int64{1, 1, 3, 4}},
		}, new)
	})
}
