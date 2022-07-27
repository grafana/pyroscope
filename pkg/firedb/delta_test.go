package firedb

import (
	"testing"

	"github.com/google/uuid"
	"github.com/prometheus/common/model"
	"github.com/samber/lo"
	"github.com/stretchr/testify/require"

	schemav1 "github.com/grafana/fire/pkg/firedb/schemas/v1"
	commonv1 "github.com/grafana/fire/pkg/gen/common/v1"
	profilev1 "github.com/grafana/fire/pkg/gen/google/v1"
	firemodel "github.com/grafana/fire/pkg/model"
)

func TestComputeDelta(t *testing.T) {
	delta := newDeltaProfiles()

	profile := newMemoryProfileBuilder(1)
	profile.ForStacktrace("a", "b", "c").AddMemorySample(1, 10, 1, 1)
	profile.ForStacktrace("a", "b", "c", "d").AddMemorySample(1, 10, 1, 1)

	_, deltaLabels := delta.computeDelta(profile.ToModel())

	require.Len(t, deltaLabels, 2)

	// profile = newMemoryProfileBuilder(2)
	// profile.ForStacktrace("a", "b", "c").AddMemorySample(2, 20, 2, 2)
	// profile.ForStacktrace("a", "b", "c", "d").AddMemorySample(3, 30, 2, 2)

	// err = head.Ingest(ctx, profile.Profile, profile.UUID, profile.labels...)
	// require.NoError(t, err)
}

type memoryProfileBuilder struct {
	*profilev1.Profile
	uuid.UUID
	labels []*commonv1.LabelPair
}

func newMemoryProfileBuilder(ts int64) *memoryProfileBuilder {
	return &memoryProfileBuilder{
		Profile: &profilev1.Profile{
			TimeNanos: ts,
			SampleType: []*profilev1.ValueType{
				{
					Unit: 4,
					Type: 3,
				},
				{
					Unit: 2,
					Type: 5,
				},
				{
					Unit: 4,
					Type: 6,
				},
				{
					Unit: 2,
					Type: 7,
				},
			},
			Mapping: []*profilev1.Mapping{
				{Id: 1, HasFunctions: true},
			},
			StringTable:       []string{"", "space", "bytes", "alloc_objects", "count", "alloc_space", "inuse_objects", "inuse_space"},
			DefaultSampleType: 5,
			PeriodType: &profilev1.ValueType{
				Unit: 2,
				Type: 1,
			},
		},
		UUID: uuid.New(),
		labels: []*commonv1.LabelPair{
			{
				Name:  "job",
				Value: "foo",
			},
			{
				Name:  model.MetricNameLabel,
				Value: "memory",
			},
		},
	}
}

func (m *memoryProfileBuilder) ForStacktrace(stacktraces ...string) *stacktraceBuilder {
	namePositions := lo.Map(stacktraces, func(stacktrace string, i int) int64 {
		for i, n := range m.StringTable {
			if n == stacktrace {
				return int64(i)
			}
		}
		m.StringTable = append(m.StringTable, stacktrace)
		return int64(len(m.StringTable) - 1)
	})

	// search functions
	functionIds := lo.Map(namePositions, func(namePos int64, i int) uint64 {
		for _, f := range m.Function {
			if f.Name == namePos {
				return f.Id
			}
		}
		f := &profilev1.Function{
			Name: namePos,
			Id:   uint64(len(m.Function)) + 1,
		}
		m.Function = append(m.Function, f)
		return f.Id
	})
	// search locations
	locationIDs := lo.Map(functionIds, func(functionId uint64, i int) uint64 {
		for _, l := range m.Location {
			if l.Line[0].FunctionId == functionId {
				return l.Id
			}
		}
		l := &profilev1.Location{
			MappingId: uint64(1),
			Line: []*profilev1.Line{
				{
					FunctionId: functionId,
				},
			},
			Id: uint64(len(m.Location)) + 1,
		}
		m.Location = append(m.Location, l)
		return l.Id
	})
	return &stacktraceBuilder{
		locationID:           locationIDs,
		memoryProfileBuilder: m,
	}
}

func (m *memoryProfileBuilder) ToModel() (*schemav1.Profile, []firemodel.Labels) {
	res := &schemav1.Profile{
		ID:         m.UUID,
		SeriesRefs: nil,

		DropFrames:        m.DropFrames,
		KeepFrames:        m.KeepFrames,
		TimeNanos:         m.TimeNanos,
		DurationNanos:     m.DurationNanos,
		Period:            m.Period,
		DefaultSampleType: m.DefaultSampleType,
		Comment:           m.Comment,
	}
	return res, nil
}

type stacktraceBuilder struct {
	locationID []uint64
	*memoryProfileBuilder
}

func (s *stacktraceBuilder) AddMemorySample(allocObjs int64, allocSpace int64, inuseObjs int64, inuseSpace int64) {
	s.Profile.Sample = append(s.Profile.Sample, &profilev1.Sample{
		LocationId: s.locationID,
		Value:      []int64{allocObjs, allocSpace, inuseObjs, inuseSpace},
	})
}
