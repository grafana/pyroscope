package otlp

import (
	"fmt"
	"time"

	googleProfile "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	otelProfile "github.com/grafana/pyroscope/api/otlp/profiles/v1experimental"

	"google.golang.org/protobuf/proto"
)

func OprofToPprof(p *otelProfile.Profile) ([]byte, error) {
	dst := ConvertOtelToGoogle(p)
	return proto.Marshal(dst)
}

// ConvertOtelToGoogle converts an OpenTelemetry profile to a Google profile.
func ConvertOtelToGoogle(src *otelProfile.Profile) *googleProfile.Profile {
	dst := &googleProfile.Profile{
		SampleType:        convertSampleTypesBack(src.SampleType),
		StringTable:       src.StringTable[:],
		TimeNanos:         src.TimeNanos,
		DurationNanos:     src.DurationNanos,
		PeriodType:        convertValueTypeBack(src.PeriodType),
		Period:            src.Period,
		DefaultSampleType: src.DefaultSampleType,
		DropFrames:        src.DropFrames,
		KeepFrames:        src.KeepFrames,
		Comment:           src.Comment,
		Mapping:           convertMappingsBack(src.Mapping),
	}
	stringmap := make(map[string]int)
	addstr := func(s string) int64 {
		if _, ok := stringmap[s]; !ok {
			stringmap[s] = len(dst.StringTable)
			dst.StringTable = append(dst.StringTable, s)
		}
		return int64(stringmap[s])
	}
	addstr("")

	if dst.TimeNanos == 0 {
		dst.TimeNanos = time.Now().UnixNano()
	}
	if dst.DurationNanos == 0 {
		dst.DurationNanos = (time.Second * 10).Nanoseconds()
	}

	// attribute_table
	// attribute_units
	// link_table

	dst.Function = []*googleProfile.Function{}
	for i, funcItem := range src.Function {
		gf := convertFunctionBack(funcItem)
		gf.Id = uint64(i + 1)
		dst.Function = append(dst.Function, gf)
	}
	funcmap := map[string]uint64{}
	addfunc := func(s string) uint64 {
		if _, ok := funcmap[s]; !ok {
			funcmap[s] = uint64(len(dst.Function)) + 1
			dst.Function = append(dst.Function, &googleProfile.Function{
				Id:   funcmap[s],
				Name: addstr(s),
			})
		}
		return funcmap[s]
	}

	dst.Location = []*googleProfile.Location{}
	// Convert locations and mappings
	for i, loc := range src.Location {
		gl := convertLocationBack(loc)
		gl.Id = uint64(i + 1)
		if len(gl.Line) == 0 {
			m := src.Mapping[loc.MappingIndex]
			gl.Line = append(gl.Line, &googleProfile.Line{
				FunctionId: addfunc(fmt.Sprintf("%s 0x%x", src.StringTable[m.Filename], loc.Address)),
			})
		}
		dst.Location = append(dst.Location, gl)
	}

	// Convert samples
	for _, sample := range src.Sample {
		gs := convertSampleBack(src, sample, src.LocationIndices, addstr)
		dst.Sample = append(dst.Sample, gs)
	}

	if len(dst.SampleType) == 0 {
		dst.SampleType = []*googleProfile.ValueType{{
			Type: addstr("samples"),
			Unit: addstr("ms"),
		}}
		dst.DefaultSampleType = addstr("samples")
	}

	return dst
}

func convertSampleTypesBack(ost []*otelProfile.ValueType) []*googleProfile.ValueType {
	var gst []*googleProfile.ValueType
	for _, st := range ost {
		gst = append(gst, &googleProfile.ValueType{
			Type: st.Type,
			Unit: st.Unit,
		})
	}
	return gst
}

func convertValueTypeBack(ovt *otelProfile.ValueType) *googleProfile.ValueType {
	if ovt == nil {
		return nil
	}
	return &googleProfile.ValueType{
		Type: ovt.Type,
		Unit: ovt.Unit,
	}
}

func convertLocationBack(ol *otelProfile.Location) *googleProfile.Location {
	gl := &googleProfile.Location{
		MappingId: ol.MappingIndex + 1,
		Address:   ol.Address,
		Line:      make([]*googleProfile.Line, len(ol.Line)),
		IsFolded:  ol.IsFolded,
	}
	for i, line := range ol.Line {
		gl.Line[i] = convertLineBack(line)
	}
	return gl
}

// convertLineBack converts an OpenTelemetry Line to a Google Line.
func convertLineBack(ol *otelProfile.Line) *googleProfile.Line {
	return &googleProfile.Line{
		FunctionId: ol.FunctionIndex + 1,
		Line:       ol.Line,
	}
}

func convertFunctionBack(of *otelProfile.Function) *googleProfile.Function {
	return &googleProfile.Function{
		Name:       of.Name,
		SystemName: of.SystemName,
		Filename:   of.Filename,
		StartLine:  of.StartLine,
	}
}

func convertSampleBack(p *otelProfile.Profile, os *otelProfile.Sample, locationIndexes []int64, addstr func(s string) int64) *googleProfile.Sample {
	gs := &googleProfile.Sample{
		Value: os.Value,
	}

	if len(gs.Value) == 0 {
		gs.Value = []int64{int64(len(os.TimestampsUnixNano))}
	}
	convertSampleAttributesToLabelsBack(p, os, gs, addstr)

	for i := os.LocationsStartIndex; i < os.LocationsStartIndex+os.LocationsLength; i++ {
		gs.LocationId = append(gs.LocationId, uint64(locationIndexes[i]+1))
	}

	return gs
}

func convertSampleAttributesToLabelsBack(p *otelProfile.Profile, os *otelProfile.Sample, gs *googleProfile.Sample, addstr func(s string) int64) {
	gs.Label = make([]*googleProfile.Label, 0, len(os.Attributes))
	for _, attribute := range os.Attributes {
		att := p.AttributeTable[attribute]
		if att.Value.GetStringValue() != "" {
			gs.Label = append(gs.Label, &googleProfile.Label{
				Key: addstr(att.Key),
				Str: addstr(att.Value.GetStringValue()),
			})
		}
	}
}

// convertMappingsBack converts a slice of OpenTelemetry Mapping entries to Google Mapping entries.
func convertMappingsBack(otelMappings []*otelProfile.Mapping) []*googleProfile.Mapping {
	googleMappings := make([]*googleProfile.Mapping, len(otelMappings))
	for i, om := range otelMappings {
		googleMappings[i] = &googleProfile.Mapping{
			Id:              uint64(i + 1), // Assuming direct mapping of IDs
			MemoryStart:     om.MemoryStart,
			MemoryLimit:     om.MemoryLimit,
			FileOffset:      om.FileOffset,
			Filename:        om.Filename, // Assume direct use; may need conversion if using indices
			BuildId:         om.BuildId,
			HasFunctions:    om.HasFunctions,
			HasFilenames:    om.HasFilenames,
			HasLineNumbers:  om.HasLineNumbers,
			HasInlineFrames: om.HasInlineFrames,
		}
	}
	return googleMappings
}
