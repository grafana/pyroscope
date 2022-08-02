package testhelper

import (
	"fmt"

	"github.com/google/uuid"
	"github.com/prometheus/common/model"
	"github.com/samber/lo"

	schemav1 "github.com/grafana/fire/pkg/firedb/schemas/v1"
	commonv1 "github.com/grafana/fire/pkg/gen/common/v1"
	profilev1 "github.com/grafana/fire/pkg/gen/google/v1"
	firemodel "github.com/grafana/fire/pkg/model"
)

type ProfileBuilder struct {
	*profilev1.Profile
	uuid.UUID
	Labels []*commonv1.LabelPair
}

func NewProfileBuilder(ts int64) *ProfileBuilder {
	return &ProfileBuilder{
		Profile: &profilev1.Profile{
			TimeNanos: ts,
			Mapping: []*profilev1.Mapping{
				{Id: 1, HasFunctions: true},
			},
		},
		UUID: uuid.New(),
		Labels: []*commonv1.LabelPair{
			{
				Name:  "job",
				Value: "foo",
			},
		},
	}
}

func (m *ProfileBuilder) MemoryProfile() *ProfileBuilder {
	m.Profile.SampleType = []*profilev1.ValueType{
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
	}
	m.Profile.StringTable = []string{"", "space", "bytes", "alloc_objects", "count", "alloc_space", "inuse_objects", "inuse_space"}
	m.Profile.DefaultSampleType = 5
	m.Profile.PeriodType = &profilev1.ValueType{
		Unit: 2,
		Type: 1,
	}
	m.Labels = append(m.Labels, &commonv1.LabelPair{
		Name:  model.MetricNameLabel,
		Value: "memory",
	})

	return m
}

func (m *ProfileBuilder) Name() string {
	for _, lbl := range m.Labels {
		if lbl.Name == model.MetricNameLabel {
			return lbl.Value
		}
	}
	return ""
}

func (m *ProfileBuilder) CPUProfile() *ProfileBuilder {
	m.Profile.SampleType = []*profilev1.ValueType{
		{
			Unit: 2,
			Type: 1,
		},
	}
	m.Profile.StringTable = []string{"", "cpu", "nanoseconds"}
	m.Profile.DefaultSampleType = 1
	m.Profile.PeriodType = &profilev1.ValueType{
		Unit: 2,
		Type: 1,
	}
	m.Labels = append(m.Labels, &commonv1.LabelPair{
		Name:  model.MetricNameLabel,
		Value: "process_cpu",
	})

	return m
}

func (m *ProfileBuilder) ForStacktrace(stacktraces ...string) *StacktraceBuilder {
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
	return &StacktraceBuilder{
		locationID:     locationIDs,
		ProfileBuilder: m,
	}
}

func (m *ProfileBuilder) ToModel() (*schemav1.Profile, []firemodel.Labels) {
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

type StacktraceBuilder struct {
	locationID []uint64
	*ProfileBuilder
}

func (s *StacktraceBuilder) AddSamples(samples ...int64) {
	if exp, act := len(s.Profile.SampleType), len(samples); exp != act {
		panic(fmt.Sprintf("profile expects %d sample(s), there was actually %d sample(s) given.", exp, act))
	}
	s.Profile.Sample = append(s.Profile.Sample, &profilev1.Sample{
		LocationId: s.locationID,
		Value:      samples,
	})
}
