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

	profiles, labels := newProfileSchema(builder.Profile, "memory")

	profile := delta.computeDelta(profiles[0], labels[0])
	require.Nil(t, profile)
	profile = delta.computeDelta(profiles[1], labels[1])
	require.Nil(t, profile)
	profile = delta.computeDelta(profiles[2], labels[2])
	require.NotNil(t, profile)
	require.Equal(t, 2, len(profile.Samples))
	require.Equal(t, int64(3), profile.Samples[0].Value)
	require.Equal(t, int64(3), profile.Samples[1].Value)
	profile = delta.computeDelta(profiles[3], labels[3])
	require.NotNil(t, profile)
	require.Equal(t, 2, len(profile.Samples))
	require.Equal(t, int64(4), profile.Samples[0].Value)
	require.Equal(t, int64(4), profile.Samples[1].Value)

	profiles, labels = newProfileSchema(builder.Profile, "memory")
	profile = delta.computeDelta(profiles[0], labels[0])
	require.NotNil(t, profile)
	require.Equal(t, 0, len(profile.Samples))
	profile = delta.computeDelta(profiles[1], labels[1])
	require.NotNil(t, profile)
	require.Equal(t, 0, len(profile.Samples))
	profile = delta.computeDelta(profiles[2], labels[2])
	require.NotNil(t, profile)
	require.Equal(t, 2, len(profile.Samples))
	require.Equal(t, int64(3), profile.Samples[0].Value)
	require.Equal(t, int64(3), profile.Samples[1].Value)
	profile = delta.computeDelta(profiles[3], labels[3])
	require.NotNil(t, profile)
	require.Equal(t, 2, len(profile.Samples))
	require.Equal(t, int64(4), profile.Samples[0].Value)
	require.Equal(t, int64(4), profile.Samples[1].Value)
}

func newProfileSchema(p *profilev1.Profile, name string) ([]*schemav1.Profile, []firemodel.Labels) {
	var (
		labels, seriesRefs = labelsForProfile(p, &commonv1.LabelPair{Name: model.MetricNameLabel, Value: name})
		ps                 = make([]*schemav1.Profile, len(labels))
	)
	for idxType := range labels {
		ps[idxType] = &schemav1.Profile{
			ID:                uuid.New(),
			TimeNanos:         p.TimeNanos,
			Comments:          p.Comment,
			DurationNanos:     p.DurationNanos,
			DropFrames:        p.DropFrames,
			KeepFrames:        p.KeepFrames,
			Period:            p.Period,
			DefaultSampleType: p.DefaultSampleType,
		}
		hashes := pprof.StacktracesHasher{}.Hashes(p.Sample)
		ps[idxType].Samples = make([]*schemav1.Sample, len(p.Sample))
		for i, s := range p.Sample {
			ps[idxType].Samples[i] = &schemav1.Sample{
				StacktraceID: hashes[i],
				Value:        s.Value[idxType],
				Labels:       copySlice(s.Label),
			}
		}
		ps[idxType].SeriesFingerprint = seriesRefs[idxType]
	}
	return ps, labels
}

func TestDeltaSample(t *testing.T) {
	new := []*schemav1.Sample{
		{StacktraceID: 2, Value: 1},
		{StacktraceID: 3, Value: 1},
	}
	highest := deltaSamples([]*schemav1.Sample{}, new)
	require.Equal(t, 2, len(highest))
	require.Equal(t, []*schemav1.Sample{
		{StacktraceID: 2, Value: 1},
		{StacktraceID: 3, Value: 1},
	}, highest)
	require.Equal(t, highest, new)

	t.Run("same stacktraces, matching counter samples, matching gauge samples", func(t *testing.T) {
		new = []*schemav1.Sample{
			{StacktraceID: 2, Value: 1},
			{StacktraceID: 3, Value: 1},
		}
		highest = deltaSamples(highest, new)
		require.Equal(t, 2, len(highest))
		require.Equal(t, []*schemav1.Sample{
			{StacktraceID: 2, Value: 1},
			{StacktraceID: 3, Value: 1},
		}, highest)
		require.Equal(t, []*schemav1.Sample{
			{StacktraceID: 2, Value: 0},
			{StacktraceID: 3, Value: 0},
		}, new)
	})

	t.Run("same stacktraces, matching counter samples, empty gauge samples", func(t *testing.T) {
		new = []*schemav1.Sample{
			{StacktraceID: 2, Value: 1},
			{StacktraceID: 3, Value: 1},
		}
		highest = deltaSamples(highest, new)
		require.Equal(t, 2, len(highest))
		require.Equal(t, []*schemav1.Sample{
			{StacktraceID: 2, Value: 1},
			{StacktraceID: 3, Value: 1},
		}, highest)
		require.Equal(t, []*schemav1.Sample{
			{StacktraceID: 2, Value: 0},
			{StacktraceID: 3, Value: 0},
		}, new)
	})

	t.Run("new stacktrace, and increase counter in existing stacktrace", func(t *testing.T) {
		new = []*schemav1.Sample{
			{StacktraceID: 3, Value: 6},
			{StacktraceID: 5, Value: 1},
		}
		highest = deltaSamples(highest, new)
		require.Equal(t, []*schemav1.Sample{
			{StacktraceID: 2, Value: 1},
			{StacktraceID: 3, Value: 6},
			{StacktraceID: 5, Value: 1},
		}, highest)
		require.Equal(t, []*schemav1.Sample{
			{StacktraceID: 3, Value: 5},
			{StacktraceID: 5, Value: 1},
		}, new)
	})

	t.Run("same stacktraces, counter samples resetting", func(t *testing.T) {
		new = []*schemav1.Sample{
			{StacktraceID: 3, Value: 1},
			{StacktraceID: 5, Value: 0},
		}
		highest = deltaSamples(highest, new)
		require.Equal(t, []*schemav1.Sample{
			{StacktraceID: 2, Value: 1},
			{StacktraceID: 3, Value: 1},
			{StacktraceID: 5, Value: 0},
		}, highest)
		require.Equal(t, []*schemav1.Sample{
			{StacktraceID: 3, Value: 1},
			{StacktraceID: 5, Value: 0},
		}, new)
	})

	t.Run("two new stacktraces, raise counters of existing stacktrace", func(t *testing.T) {
		new = []*schemav1.Sample{
			{StacktraceID: 0, Value: 10},
			{StacktraceID: 1, Value: 2},
			{StacktraceID: 7, Value: 1},
		}
		highest = deltaSamples(highest, new)
		sort.Slice(highest, func(i, j int) bool {
			return highest[i].StacktraceID < highest[j].StacktraceID
		})
		require.Equal(t, []*schemav1.Sample{
			{StacktraceID: 0, Value: 10},
			{StacktraceID: 1, Value: 2},
			{StacktraceID: 2, Value: 1},
			{StacktraceID: 3, Value: 1},
			{StacktraceID: 5, Value: 0},
			{StacktraceID: 7, Value: 1},
		}, highest)
		require.Equal(t, []*schemav1.Sample{
			{StacktraceID: 0, Value: 10},
			{StacktraceID: 1, Value: 2},
			{StacktraceID: 7, Value: 1},
		}, new)
	})
}
