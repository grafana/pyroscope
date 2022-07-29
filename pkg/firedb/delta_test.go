package firedb

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/prometheus/common/model"
	"github.com/samber/lo"
	"github.com/stretchr/testify/require"

	schemav1 "github.com/grafana/fire/pkg/firedb/schemas/v1"
	commonv1 "github.com/grafana/fire/pkg/gen/common/v1"
	profilev1 "github.com/grafana/fire/pkg/gen/google/v1"
	firemodel "github.com/grafana/fire/pkg/model"
	"github.com/grafana/fire/pkg/pprof"
)

func TestComputeDelta(t *testing.T) {
	ctx := context.Background()
	head, err := NewHead(t.TempDir())
	require.NoError(t, err)

	p1, err := pprof.Open("/Users/cyril/pprof/pprof.enterprise-logs.alloc_objects.alloc_space.inuse_objects.inuse_space.006.pb.gz")
	require.NoError(t, err)
	p1.Normalize()
	p2, err := pprof.Open("/Users/cyril/pprof/pprof.enterprise-logs.alloc_objects.alloc_space.inuse_objects.inuse_space.008.pb.gz")
	require.NoError(t, err)
	p2.Normalize()

	uniq := map[string][]*profilev1.Sample{}
	for _, s := range p1.Sample {
		key := ""
		for _, l := range s.LocationId {
			key += fmt.Sprintf("%d", l) + ":"
		}
		if _, ok := uniq[key]; !ok {
			uniq[key] = []*profilev1.Sample{s}
			continue
		}
		uniq[key] = append(uniq[key], s)
		fmt.Println("dupe found", key, s)
	}
	totalDupe := 0
	for _, v := range uniq {
		totalDupe += len(v) - 1
	}
	fmt.Println("total dupe", totalDupe)
	fmt.Println("total samples", len(p1.Sample))

	err = head.Ingest(ctx, p1.Profile, uuid.New(), &commonv1.LabelPair{Name: model.MetricNameLabel, Value: "memory"})
	require.NoError(t, err)

	err = head.Ingest(ctx, p1.Profile, uuid.New(), &commonv1.LabelPair{Name: model.MetricNameLabel, Value: "memory"})
	require.NoError(t, err)

	err = head.Ingest(ctx, p2.Profile, uuid.New(), &commonv1.LabelPair{Name: model.MetricNameLabel, Value: "memory"})
	require.NoError(t, err)
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
		Comments:          m.Comment,
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
