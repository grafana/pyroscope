package otlp

import (
	"fmt"
	"time"

	googleProfile "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	otelProfile "github.com/grafana/pyroscope/api/otlp/profiles/v1experimental"
)

const serviceNameKey = "service.name"

// ConvertOtelToGoogle converts an OpenTelemetry profile to a Google profile.
func ConvertOtelToGoogle(src *otelProfile.Profile) map[string]*googleProfile.Profile {
	svc2Profile := make(map[string]*profileBuilder)
	for _, sample := range src.Sample {
		svc := serviceNameFromSample(src, sample)
		p, ok := svc2Profile[svc]
		if !ok {
			p = newProfileBuilder(src)
			svc2Profile[svc] = p
		}
		p.convertSampleBack(sample)
	}

	result := make(map[string]*googleProfile.Profile)
	for svc, p := range svc2Profile {
		result[svc] = p.dst
	}

	return result
}

type profileBuilder struct {
	src                     *otelProfile.Profile
	dst                     *googleProfile.Profile
	stringMap               map[string]int64
	functionMap             map[*otelProfile.Function]uint64
	unsymbolziedFuncNameMap map[string]uint64
	locationMap             map[*otelProfile.Location]uint64
	mappingMap              map[*otelProfile.Mapping]uint64
}

func newProfileBuilder(src *otelProfile.Profile) *profileBuilder {
	res := &profileBuilder{
		src:                     src,
		stringMap:               make(map[string]int64),
		functionMap:             make(map[*otelProfile.Function]uint64),
		locationMap:             make(map[*otelProfile.Location]uint64),
		mappingMap:              make(map[*otelProfile.Mapping]uint64),
		unsymbolziedFuncNameMap: make(map[string]uint64),
		dst: &googleProfile.Profile{
			TimeNanos:         src.TimeNanos,
			DurationNanos:     src.DurationNanos,
			Period:            src.Period,
			DefaultSampleType: src.DefaultSampleType,
			DropFrames:        src.DropFrames,
			KeepFrames:        src.KeepFrames,
			Comment:           src.Comment,
		},
	}
	res.addstr("")
	res.dst.SampleType = res.convertSampleTypesBack(src.SampleType)
	res.dst.PeriodType = res.convertValueTypeBack(src.PeriodType)
	if len(res.dst.SampleType) == 0 {
		res.dst.SampleType = []*googleProfile.ValueType{{
			Type: res.addstr("samples"),
			Unit: res.addstr("ms"),
		}}
		res.dst.DefaultSampleType = res.addstr("samples")
	}
	if res.dst.TimeNanos == 0 {
		res.dst.TimeNanos = time.Now().UnixNano()
	}
	if res.dst.DurationNanos == 0 {
		res.dst.DurationNanos = (time.Second * 10).Nanoseconds()
	}
	return res
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

func serviceNameFromSample(p *otelProfile.Profile, sample *otelProfile.Sample) string {
	for _, attributeIndex := range sample.Attributes {
		attribute := p.AttributeTable[attributeIndex]
		if attribute.Key == serviceNameKey {
			return attribute.Value.GetStringValue()
		}
	}
	return ""
}

func (p *profileBuilder) convertSampleTypesBack(ost []*otelProfile.ValueType) []*googleProfile.ValueType {
	var gst []*googleProfile.ValueType
	for _, st := range ost {
		gst = append(gst, &googleProfile.ValueType{
			Type: p.addstr(p.src.StringTable[st.Type]),
			Unit: p.addstr(p.src.StringTable[st.Unit]),
		})
	}
	return gst
}

func (p *profileBuilder) convertValueTypeBack(ovt *otelProfile.ValueType) *googleProfile.ValueType {
	if ovt == nil {
		return nil
	}
	return &googleProfile.ValueType{
		Type: p.addstr(p.src.StringTable[ovt.Type]),
		Unit: p.addstr(p.src.StringTable[ovt.Unit]),
	}
}

func (p *profileBuilder) convertLocationBack(ol *otelProfile.Location) uint64 {
	if i, ok := p.locationMap[ol]; ok {
		return i
	}
	om := p.src.Mapping[ol.MappingIndex]
	gl := &googleProfile.Location{
		MappingId: p.convertMappingBack(om),
		Address:   ol.Address,
		Line:      make([]*googleProfile.Line, len(ol.Line)),
		IsFolded:  ol.IsFolded,
	}
	for i, line := range ol.Line {
		gl.Line[i] = p.convertLineBack(line)
	}

	if len(gl.Line) == 0 {
		gl.Line = append(gl.Line, &googleProfile.Line{
			FunctionId: p.addfunc(fmt.Sprintf("%s 0x%x", p.src.StringTable[om.Filename], ol.Address)),
		})
	}

	p.dst.Location = append(p.dst.Location, gl)
	gl.Id = uint64(len(p.dst.Location))
	p.locationMap[ol] = gl.Id
	return gl.Id
}

// convertLineBack converts an OpenTelemetry Line to a Google Line.
func (p *profileBuilder) convertLineBack(ol *otelProfile.Line) *googleProfile.Line {
	return &googleProfile.Line{
		FunctionId: p.convertFunctionBack(p.src.Function[ol.FunctionIndex]),
		Line:       ol.Line,
	}
}

func (p *profileBuilder) convertFunctionBack(of *otelProfile.Function) uint64 {
	if i, ok := p.functionMap[of]; ok {
		return i
	}
	gf := &googleProfile.Function{
		Name:       p.addstr(p.src.StringTable[of.Name]),
		SystemName: p.addstr(p.src.StringTable[of.SystemName]),
		Filename:   p.addstr(p.src.StringTable[of.Filename]),
		StartLine:  of.StartLine,
	}
	p.dst.Function = append(p.dst.Function, gf)
	gf.Id = uint64(len(p.dst.Function))
	p.functionMap[of] = gf.Id
	return gf.Id
}

func (p *profileBuilder) convertSampleBack(os *otelProfile.Sample) *googleProfile.Sample {
	gs := &googleProfile.Sample{
		Value: os.Value,
	}

	if len(gs.Value) == 0 {
		gs.Value = []int64{int64(len(os.TimestampsUnixNano))}
	}
	p.convertSampleAttributesToLabelsBack(os, gs)

	for i := os.LocationsStartIndex; i < os.LocationsStartIndex+os.LocationsLength; i++ {
		gs.LocationId = append(gs.LocationId, p.convertLocationBack(p.src.Location[p.src.LocationIndices[i]]))
	}

	p.dst.Sample = append(p.dst.Sample, gs)

	return gs
}

func (p *profileBuilder) convertSampleAttributesToLabelsBack(os *otelProfile.Sample, gs *googleProfile.Sample) {
	gs.Label = make([]*googleProfile.Label, 0, len(os.Attributes))
	for _, attribute := range os.Attributes {
		att := p.src.AttributeTable[attribute]
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
func (p *profileBuilder) convertMappingBack(om *otelProfile.Mapping) uint64 {
	if i, ok := p.mappingMap[om]; ok {
		return i
	}

	gm := &googleProfile.Mapping{
		MemoryStart:     om.MemoryStart,
		MemoryLimit:     om.MemoryLimit,
		FileOffset:      om.FileOffset,
		Filename:        p.addstr(p.src.StringTable[om.Filename]),
		BuildId:         p.addstr(p.src.StringTable[om.BuildId]),
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
