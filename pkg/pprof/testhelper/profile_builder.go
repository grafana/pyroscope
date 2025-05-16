package testhelper

import (
	"fmt"
	"sort"

	"github.com/google/uuid"
	"github.com/prometheus/common/model"
	"github.com/samber/lo"

	profilev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	phlaremodel "github.com/grafana/pyroscope/pkg/model"
)

type ProfileBuilder struct {
	*profilev1.Profile
	strings map[string]int

	uuid.UUID
	Labels      []*typesv1.LabelPair
	Annotations []*typesv1.ProfileAnnotation

	externalFunctionID2LocationId map[uint32]uint64
	externalSampleID2SampleIndex  map[sampleID]uint32
}

type sampleID struct {
	locationsID uint64
	labelsID    uint64
}

// NewProfileBuilder creates a new ProfileBuilder with the given nanoseconds timestamp.
func NewProfileBuilder(ts int64) *ProfileBuilder {
	return NewProfileBuilderWithLabels(ts, []*typesv1.LabelPair{
		{
			Name:  "job",
			Value: "foo",
		},
	})
}

// NewProfileBuilderWithLabels creates a new ProfileBuilder with the given nanoseconds timestamp and labels.
func NewProfileBuilderWithLabels(ts int64, labels []*typesv1.LabelPair) *ProfileBuilder {
	profile := new(profilev1.Profile)
	profile.TimeNanos = ts
	profile.Mapping = append(profile.Mapping, &profilev1.Mapping{
		Id: 1, HasFunctions: true,
	})
	p := &ProfileBuilder{
		Profile: profile,
		UUID:    uuid.New(),
		Labels:  labels,
		strings: map[string]int{},

		externalFunctionID2LocationId: map[uint32]uint64{},
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
Outer:
	for i := 0; i < len(lv); i += 2 {
		for _, lbl := range m.Labels {
			if lbl.Name == lv[i] {
				lbl.Value = lv[i+1]
				continue Outer
			}
		}
		m.Labels = append(m.Labels, &typesv1.LabelPair{
			Name:  lv[i],
			Value: lv[i+1],
		})
	}
	sort.Sort(phlaremodel.Labels(m.Labels))
	return m
}

func (m *ProfileBuilder) WithAnnotations(annotationValues ...string) *ProfileBuilder {
	for _, a := range annotationValues {
		m.Annotations = append(m.Annotations, &typesv1.ProfileAnnotation{
			Key:   "throttled",
			Value: a,
		})
	}
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

func (m *ProfileBuilder) MetricName(name string) {
	m.Labels = append(m.Labels, &typesv1.LabelPair{
		Name:  model.MetricNameLabel,
		Value: name,
	})
}

func (m *ProfileBuilder) PeriodType(periodType string, periodUnit string) {
	m.Profile.PeriodType = &profilev1.ValueType{
		Type: m.addString(periodType),
		Unit: m.addString(periodUnit),
	}
}

func (m *ProfileBuilder) CustomProfile(name, typ, unit, periodType, periodUnit string) {
	m.AddSampleType(typ, unit)
	m.Profile.DefaultSampleType = m.addString(typ)

	m.PeriodType(periodType, periodUnit)

	m.MetricName(name)
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

func (m *ProfileBuilder) AddString(s string) int64 {
	return m.addString(s)
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

func (m *ProfileBuilder) FindLocationByExternalID(externalID uint32) (uint64, bool) {
	loc, ok := m.externalFunctionID2LocationId[externalID]
	return loc, ok
}

func (m *ProfileBuilder) AddExternalFunction(frame string, externalFunctionID uint32) uint64 {
	fname := m.addString(frame)
	funcID := uint64(len(m.Function)) + 1
	m.Function = append(m.Function, &profilev1.Function{
		Id:   funcID,
		Name: fname,
	})
	locID := uint64(len(m.Location)) + 1
	m.Location = append(m.Location, &profilev1.Location{
		Id:        locID,
		MappingId: uint64(1),
		Line:      []*profilev1.Line{{FunctionId: funcID}},
	})
	m.externalFunctionID2LocationId[externalFunctionID] = locID
	return locID
}

func (m *ProfileBuilder) AddExternalSample(locs []uint64, values []int64, externalSampleID uint32) {
	m.AddExternalSampleWithLabels(locs, values, nil, uint64(externalSampleID), 0)
}

func (m *ProfileBuilder) FindExternalSample(externalSampleID uint32) *profilev1.Sample {
	return m.FindExternalSampleWithLabels(uint64(externalSampleID), 0)
}

func (m *ProfileBuilder) AddExternalSampleWithLabels(locs []uint64, values []int64, labels phlaremodel.Labels, locationsID, labelsID uint64) {
	sample := &profilev1.Sample{
		LocationId: locs,
		Value:      values,
	}
	if m.externalSampleID2SampleIndex == nil {
		m.externalSampleID2SampleIndex = map[sampleID]uint32{}
	}
	m.externalSampleID2SampleIndex[sampleID{locationsID: locationsID, labelsID: labelsID}] = uint32(len(m.Profile.Sample))
	m.Profile.Sample = append(m.Profile.Sample, sample)
	if len(labels) > 0 {
		sample.Label = make([]*profilev1.Label, 0, len(labels))
		for _, label := range labels {
			sample.Label = append(sample.Label, &profilev1.Label{
				Key: m.addString(label.Name),
				Str: m.addString(label.Value),
			})
		}
	}
}

func (m *ProfileBuilder) FindExternalSampleWithLabels(locationsID, labelsID uint64) *profilev1.Sample {
	sampleIndex, ok := m.externalSampleID2SampleIndex[sampleID{locationsID: locationsID, labelsID: labelsID}]
	if !ok {
		return nil
	}
	sample := m.Profile.Sample[sampleIndex]
	return sample
}

type StacktraceBuilder struct {
	locationID []uint64
	*ProfileBuilder
}

func (s *StacktraceBuilder) AddSamples(samples ...int64) *ProfileBuilder {
	if exp, act := len(s.Profile.SampleType), len(samples); exp != act {
		panic(fmt.Sprintf("profile expects %d sample(s), there was actually %d sample(s) given.", exp, act))
	}
	s.Profile.Sample = append(s.Profile.Sample, &profilev1.Sample{
		LocationId: s.locationID,
		Value:      samples,
	})
	return s.ProfileBuilder
}
