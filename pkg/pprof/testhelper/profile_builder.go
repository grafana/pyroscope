package testhelper

import (
	"fmt"
	"sort"

	"github.com/google/uuid"
	"github.com/prometheus/common/model"
	"github.com/samber/lo"

	profilev1 "github.com/grafana/phlare/api/gen/proto/go/google/v1"
	typesv1 "github.com/grafana/phlare/api/gen/proto/go/types/v1"
	phlaremodel "github.com/grafana/phlare/pkg/model"
	schemav1 "github.com/grafana/phlare/pkg/phlaredb/schemas/v1"
)

type ProfileBuilder struct {
	*profilev1.Profile
	strings map[string]int
	uuid.UUID
	Labels []*typesv1.LabelPair
}

func NewProfileBuilder(ts int64) *ProfileBuilder {
	p := &ProfileBuilder{
		Profile: &profilev1.Profile{
			TimeNanos: ts,
			Mapping: []*profilev1.Mapping{
				{Id: 1, HasFunctions: true},
			},
		},
		UUID: uuid.New(),
		Labels: []*typesv1.LabelPair{
			{
				Name:  "job",
				Value: "foo",
			},
		},
		strings: map[string]int{},
	}
	p.addString("")
	return p
}

func (m *ProfileBuilder) MemoryProfile() *ProfileBuilder {
	m.Profile.PeriodType = &profilev1.ValueType{
		Unit: m.addString("bytes"),
		Type: m.addString("space"),
	}
	m.Profile.SampleType = []*profilev1.ValueType{
		{
			Unit: m.addString("count"),
			Type: m.addString("alloc_objects"),
		},
		{
			Unit: m.addString("bytes"),
			Type: m.addString("alloc_space"),
		},
		{
			Unit: m.addString("count"),
			Type: m.addString("inuse_objects"),
		},
		{
			Unit: m.addString("bytes"),
			Type: m.addString("inuse_space"),
		},
	}
	m.Profile.DefaultSampleType = m.addString("alloc_space")

	m.Labels = append(m.Labels, &typesv1.LabelPair{
		Name:  model.MetricNameLabel,
		Value: "memory",
	})

	return m
}

func (m *ProfileBuilder) WithLabels(lv ...string) *ProfileBuilder {
	for i := 0; i < len(lv); i += 2 {
		m.Labels = append(m.Labels, &typesv1.LabelPair{
			Name:  lv[i],
			Value: lv[i+1],
		})
	}
	sort.Sort(phlaremodel.Labels(m.Labels))
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

func (m *ProfileBuilder) AddSampleType(typ, unit string) {
	m.Profile.SampleType = append(m.Profile.SampleType, &profilev1.ValueType{
		Type: m.addString(typ),
		Unit: m.addString(unit),
	})
}

func (m *ProfileBuilder) CustomProfile(name, typ, unit, periodType, periodUnit string) {
	m.AddSampleType(typ, unit)
	m.Profile.DefaultSampleType = m.addString(typ)

	m.Profile.PeriodType = &profilev1.ValueType{
		Type: m.addString(periodType),
		Unit: m.addString(periodUnit),
	}

	m.Labels = append(m.Labels, &typesv1.LabelPair{
		Name:  model.MetricNameLabel,
		Value: name,
	})
}

func (m *ProfileBuilder) CPUProfile() *ProfileBuilder {
	m.CustomProfile("process_cpu", "cpu", "nanoseconds", "cpu", "nanoseconds")
	return m
}

func (m *ProfileBuilder) ForStacktraceString(stacktraces ...string) *StacktraceBuilder {
	namePositions := lo.Map(stacktraces, func(stacktrace string, i int) int64 {
		return m.addString(stacktrace)
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

func (m *ProfileBuilder) addString(s string) int64 {
	i, ok := m.strings[s]
	if !ok {
		i = len(m.strings)
		m.strings[s] = i
		m.StringTable = append(m.StringTable, s)
	}
	return int64(i)
}

func (m *ProfileBuilder) ToModel() (*schemav1.Profile, []phlaremodel.Labels) {
	res := &schemav1.Profile{
		ID: m.UUID,

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
