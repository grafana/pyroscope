package otlp

import (
	googleProfile "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	otelProfile "github.com/grafana/pyroscope/api/otlp/profiles/v1experimental"
	"google.golang.org/protobuf/proto"
)

func OprofToPprof(p otelProfile.Profile) ([]byte, error) {
	dst := ConvertOtelToGoogle(&p)
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

	// attribute_table
	// attribute_units
	// link_table

	// Create maps to store converted items to avoid duplication
	locationMap := make(map[int64]*googleProfile.Location) // Map OpenTelemetry location index to Google location
	functionMap := make(map[int64]*googleProfile.Function) // Map OpenTelemetry function index to Google function

	// Convert locations and mappings
	for i, loc := range src.Location {
		gl := convertLocationBack(loc)
		locationMap[int64(i)] = gl
		dst.Location = append(dst.Location, gl)
	}

	for _, funcItem := range src.Function {
		gf := convertFunctionBack(funcItem)
		functionMap[int64(funcItem.Id)] = gf
		dst.Function = append(dst.Function, gf)
	}

	// Convert samples
	for _, sample := range src.Sample {
		gs := convertSampleBack(sample, locationMap)
		dst.Sample = append(dst.Sample, gs)
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
		Id:        ol.Id,
		MappingId: ol.MappingIndex,
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
		FunctionId: ol.FunctionIndex,
		Line:       ol.Line,
	}
}

func convertFunctionBack(of *otelProfile.Function) *googleProfile.Function {
	return &googleProfile.Function{
		Id:         of.Id,
		Name:       of.Name,
		SystemName: of.SystemName,
		Filename:   of.Filename,
		StartLine:  of.StartLine,
	}
}

func convertSampleBack(os *otelProfile.Sample, locationMap map[int64]*googleProfile.Location) *googleProfile.Sample {
	gs := &googleProfile.Sample{
		Value: os.Value,
	}

	for _, idx := range os.LocationIndex {
		if loc, exists := locationMap[int64(idx)]; exists {
			gs.LocationId = append(gs.LocationId, loc.Id)
		}
	}

	// // Convert attributes to labels
	// for _, attrIdx := range os.Attributes {
	// 	if int(attrIdx) < len(op.AttributeTable) {
	// 		attr := op.AttributeTable[attrIdx]
	// 		label := &googleProfile.Label{
	// 			Key: stringTable[attr.Key],
	// 		}

	// 		// Determine if the attribute value is a string or number
	// 		switch v := attr.Value.GetValue().(type) {
	// 		case *opentelemetry_proto_common_v1.AnyValue_StringValue:
	// 			label.Str = v.StringValue
	// 		case *opentelemetry_proto_common_v1.AnyValue_IntValue:
	// 			label.Num = v.IntValue
	// 		}

	// 		gs.Label = append(gs.Label, label)
	// 	}
	// }

	return gs
}

// convertMappingsBack converts a slice of OpenTelemetry Mapping entries to Google Mapping entries.
func convertMappingsBack(otelMappings []*otelProfile.Mapping) []*googleProfile.Mapping {
	googleMappings := make([]*googleProfile.Mapping, len(otelMappings))
	for i, om := range otelMappings {
		googleMappings[i] = &googleProfile.Mapping{
			Id:              om.Id, // Assuming direct mapping of IDs
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
