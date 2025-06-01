package otlp

import (
	"fmt"
	"time"

	otelProfile "go.opentelemetry.io/proto/otlp/profiles/v1development"

	googleProfile "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	pyromodel "github.com/grafana/pyroscope/pkg/model"
)

const serviceNameKey = "service.name"

type convertedProfile struct {
	profile *googleProfile.Profile
	name    *typesv1.LabelPair
}

// ConvertOtelToGoogle converts an OpenTelemetry profile to a Google profile.
func ConvertOtelToGoogle(src *otelProfile.Profile, dictionary *otelProfile.ProfilesDictionary) (map[string]convertedProfile, error) {
	svc2Profile := make(map[string]*profileBuilder)
	for _, sample := range src.Sample {
		svc := serviceNameFromSample(sample, dictionary)
		p, ok := svc2Profile[svc]
		if !ok {
			p = newProfileBuilder(src, dictionary)
			svc2Profile[svc] = p
		}
		if _, err := p.convertSampleBack(sample, dictionary); err != nil {
			return nil, err
		}
	}

	result := make(map[string]convertedProfile)
	for svc, p := range svc2Profile {
		result[svc] = convertedProfile{p.dst, p.name}
	}

	return result, nil
}

type sampleConversionType int

const (
	sampleConversionTypeNone           sampleConversionType = 0
	sampleConversionTypeSamplesToNanos sampleConversionType = 1
	sampleConversionTypeSumEvents      sampleConversionType = 2
)

type profileBuilder struct {
	src                     *otelProfile.Profile
	dst                     *googleProfile.Profile
	stringMap               map[string]int64
	functionMap             map[*otelProfile.Function]uint64
	unsymbolziedFuncNameMap map[string]uint64
	locationMap             map[*otelProfile.Location]uint64
	mappingMap              map[*otelProfile.Mapping]uint64

	sampleProcessingTypes []sampleConversionType
	name                  *typesv1.LabelPair
}

func newProfileBuilder(src *otelProfile.Profile, dictionary *otelProfile.ProfilesDictionary) *profileBuilder {
	res := &profileBuilder{
		src:                     src,
		stringMap:               make(map[string]int64),
		functionMap:             make(map[*otelProfile.Function]uint64),
		locationMap:             make(map[*otelProfile.Location]uint64),
		mappingMap:              make(map[*otelProfile.Mapping]uint64),
		unsymbolziedFuncNameMap: make(map[string]uint64),
		dst: &googleProfile.Profile{
			TimeNanos:     src.TimeNanos,
			DurationNanos: src.DurationNanos,
			Period:        src.Period,
		},
	}
	res.addstr("")
	res.dst.SampleType = res.convertSampleTypesBack(src.SampleType, dictionary)
	res.dst.PeriodType = res.convertValueTypeBack(src.PeriodType, dictionary)
	defaultSampleType := src.SampleType[src.DefaultSampleTypeIndex]
	res.dst.DefaultSampleType = res.addstr(dictionary.StringTable[defaultSampleType.TypeStrindex])
	if len(res.dst.SampleType) == 0 {
		res.dst.SampleType = []*googleProfile.ValueType{{
			Type: res.addstr("samples"),
			Unit: res.addstr("ms"),
		}}
		res.dst.DefaultSampleType = res.addstr("samples")
	}
	res.sampleProcessingTypes = make([]sampleConversionType, len(res.dst.SampleType))
	for i := 0; i < len(res.dst.SampleType); i++ {
		profileType := res.profileType(i)
		if profileType == "samples:count:cpu:nanoseconds" {
			res.dst.SampleType[i] = &googleProfile.ValueType{
				Type: res.addstr("cpu"),
				Unit: res.addstr("nanoseconds"),
			}
			if len(res.dst.SampleType) == 1 {
				res.name = &typesv1.LabelPair{
					Name:  pyromodel.LabelNameProfileName,
					Value: "process_cpu",
				}
			}
			res.sampleProcessingTypes[i] = sampleConversionTypeSamplesToNanos
		}
		// Identify off cpu profiles
		if profileType == "events:nanoseconds::" && len(res.dst.SampleType) == 1 {
			res.sampleProcessingTypes[i] = sampleConversionTypeSumEvents
			res.name = &typesv1.LabelPair{
				Name:  pyromodel.LabelNameProfileName,
				Value: pyromodel.ProfileNameOffCpu,
			}
		}
	}
	if res.name == nil {
		res.name = &typesv1.LabelPair{
			Name:  pyromodel.LabelNameProfileName,
			Value: "process_cpu", // guess
		}
	}

	if res.dst.TimeNanos == 0 {
		res.dst.TimeNanos = time.Now().UnixNano()
	}
	if res.dst.DurationNanos == 0 {
		res.dst.DurationNanos = (time.Second * 10).Nanoseconds()
	}
	return res
}

func (p *profileBuilder) profileType(idx int) string {
	var (
		periodType, periodUnit string
	)
	if p.dst.PeriodType != nil && p.dst.Period != 0 {
		periodType = p.dst.StringTable[p.dst.PeriodType.Type]
		periodUnit = p.dst.StringTable[p.dst.PeriodType.Unit]
	}
	return fmt.Sprintf("%s:%s:%s:%s",
		p.dst.StringTable[p.dst.SampleType[idx].Type],
		p.dst.StringTable[p.dst.SampleType[idx].Unit],
		periodType,
		periodUnit,
	)
}

func (p *profileBuilder) addstr(s string) int64 {
	if i, ok := p.stringMap[s]; ok {
		return i
	}
	idx := int64(len(p.dst.StringTable))
	p.stringMap[s] = idx
	p.dst.StringTable = append(p.dst.StringTable, s)
	return idx
}

func (p *profileBuilder) addfunc(s string) uint64 {
	if i, ok := p.unsymbolziedFuncNameMap[s]; ok {
		return i
	}
	idx := uint64(len(p.dst.Function)) + 1
	p.unsymbolziedFuncNameMap[s] = idx
	gf := &googleProfile.Function{
		Id:   idx,
		Name: p.addstr(s),
	}
	p.dst.Function = append(p.dst.Function, gf)
	return idx
}

func serviceNameFromSample(sample *otelProfile.Sample, dictionary *otelProfile.ProfilesDictionary) string {
	for _, attributeIndex := range sample.AttributeIndices {
		attribute := dictionary.AttributeTable[attributeIndex]
		if attribute.Key == serviceNameKey {
			return attribute.Value.GetStringValue()
		}
	}
	return ""
}

func (p *profileBuilder) convertSampleTypesBack(ost []*otelProfile.ValueType, dictionary *otelProfile.ProfilesDictionary) []*googleProfile.ValueType {
	var gst []*googleProfile.ValueType
	for _, st := range ost {
		gst = append(gst, &googleProfile.ValueType{
			Type: p.addstr(dictionary.StringTable[st.TypeStrindex]),
			Unit: p.addstr(dictionary.StringTable[st.UnitStrindex]),
		})
	}
	return gst
}

func (p *profileBuilder) convertValueTypeBack(ovt *otelProfile.ValueType, dictionary *otelProfile.ProfilesDictionary) *googleProfile.ValueType {
	if ovt == nil {
		return nil
	}
	return &googleProfile.ValueType{
		Type: p.addstr(dictionary.StringTable[ovt.TypeStrindex]),
		Unit: p.addstr(dictionary.StringTable[ovt.UnitStrindex]),
	}
}

func (p *profileBuilder) convertLocationBack(ol *otelProfile.Location, dictionary *otelProfile.ProfilesDictionary) (uint64, error) {
	if i, ok := p.locationMap[ol]; ok {
		return i, nil
	}
	if ol.MappingIndex == nil {
		return 0, fmt.Errorf("invalid location address=%x: mapping index is required", ol.Address)
	}
	lmi := *ol.MappingIndex
	om := dictionary.MappingTable[lmi]
	gl := &googleProfile.Location{
		MappingId: p.convertMappingBack(om, dictionary),
		Address:   ol.Address,
		Line:      make([]*googleProfile.Line, len(ol.Line)),
		IsFolded:  ol.IsFolded,
	}
	for i, line := range ol.Line {
		gl.Line[i] = p.convertLineBack(line, dictionary)
	}

	if len(gl.Line) == 0 {
		gl.Line = append(gl.Line, &googleProfile.Line{
			FunctionId: p.addfunc(fmt.Sprintf("%s 0x%x", dictionary.StringTable[om.FilenameStrindex], ol.Address)),
		})
	}

	p.dst.Location = append(p.dst.Location, gl)
	gl.Id = uint64(len(p.dst.Location))
	p.locationMap[ol] = gl.Id
	return gl.Id, nil
}

// convertLineBack converts an OpenTelemetry Line to a Google Line.
func (p *profileBuilder) convertLineBack(ol *otelProfile.Line, dictionary *otelProfile.ProfilesDictionary) *googleProfile.Line {
	return &googleProfile.Line{
		FunctionId: p.convertFunctionBack(dictionary.FunctionTable[ol.FunctionIndex], dictionary),
		Line:       ol.Line,
	}
}

func (p *profileBuilder) convertFunctionBack(of *otelProfile.Function, dictionary *otelProfile.ProfilesDictionary) uint64 {
	if i, ok := p.functionMap[of]; ok {
		return i
	}
	gf := &googleProfile.Function{
		Name:       p.addstr(dictionary.StringTable[of.NameStrindex]),
		SystemName: p.addstr(dictionary.StringTable[of.SystemNameStrindex]),
		Filename:   p.addstr(dictionary.StringTable[of.FilenameStrindex]),
		StartLine:  of.StartLine,
	}
	p.dst.Function = append(p.dst.Function, gf)
	gf.Id = uint64(len(p.dst.Function))
	p.functionMap[of] = gf.Id
	return gf.Id
}

func (p *profileBuilder) convertSampleBack(os *otelProfile.Sample, dictionary *otelProfile.ProfilesDictionary) (*googleProfile.Sample, error) {
	gs := &googleProfile.Sample{
		Value: os.Value,
	}
	if len(gs.Value) == 0 {
		return nil, fmt.Errorf("sample value is required")
	}

	for i, typ := range p.sampleProcessingTypes {
		switch typ {
		case sampleConversionTypeSamplesToNanos:
			gs.Value[i] *= p.src.Period
		case sampleConversionTypeSumEvents:
			// For off-CPU profiles, aggregate all sample values into a single sum
			// since pprof cannot represent variable-length sample values
			sum := int64(0)
			for _, v := range gs.Value {
				sum += v
			}
			gs.Value = []int64{sum}
		}
	}
	if p.dst.Period != 0 && p.dst.PeriodType != nil && len(gs.Value) != len(p.dst.SampleType) {
		return nil, fmt.Errorf("sample values length mismatch %d %d", len(gs.Value), len(p.dst.SampleType))
	}

	p.convertSampleAttributesToLabelsBack(os, dictionary, gs)

	for i := os.LocationsStartIndex; i < os.LocationsStartIndex+os.LocationsLength; i++ {
		loc, err := p.convertLocationBack(dictionary.LocationTable[p.src.LocationIndices[i]], dictionary)
		if err != nil {
			return nil, err
		}
		gs.LocationId = append(gs.LocationId, loc)
	}

	p.dst.Sample = append(p.dst.Sample, gs)

	return gs, nil
}

func (p *profileBuilder) convertSampleAttributesToLabelsBack(os *otelProfile.Sample, dictionary *otelProfile.ProfilesDictionary, gs *googleProfile.Sample) {
	gs.Label = make([]*googleProfile.Label, 0, len(os.AttributeIndices))
	for _, attribute := range os.AttributeIndices {
		att := dictionary.AttributeTable[attribute]
		if att.Key == serviceNameKey {
			continue
		}
		if att.Value.GetStringValue() != "" {
			gs.Label = append(gs.Label, &googleProfile.Label{
				Key: p.addstr(att.Key),
				Str: p.addstr(att.Value.GetStringValue()),
			})
		}
	}
}

// convertMappingsBack converts a slice of OpenTelemetry Mapping entries to Google Mapping entries.
func (p *profileBuilder) convertMappingBack(om *otelProfile.Mapping, dictionary *otelProfile.ProfilesDictionary) uint64 {
	if i, ok := p.mappingMap[om]; ok {
		return i
	}

	buildID := ""
	for _, attributeIndex := range om.AttributeIndices {
		attr := dictionary.AttributeTable[attributeIndex]
		if attr.Key == "process.executable.build_id.gnu" {
			buildID = attr.Value.GetStringValue()
		}
	}
	gm := &googleProfile.Mapping{
		MemoryStart:     om.MemoryStart,
		MemoryLimit:     om.MemoryLimit,
		FileOffset:      om.FileOffset,
		Filename:        p.addstr(dictionary.StringTable[om.FilenameStrindex]),
		BuildId:         p.addstr(buildID),
		HasFunctions:    om.HasFunctions,
		HasFilenames:    om.HasFilenames,
		HasLineNumbers:  om.HasLineNumbers,
		HasInlineFrames: om.HasInlineFrames,
	}
	p.dst.Mapping = append(p.dst.Mapping, gm)
	gm.Id = uint64(len(p.dst.Mapping))
	p.mappingMap[om] = gm.Id
	return gm.Id
}
