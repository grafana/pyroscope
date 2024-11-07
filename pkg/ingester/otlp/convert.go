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

	functionOffset := uint64(len(dst.Function)) + 1
	dst.Location = []*googleProfile.Location{}
	locationMappingIndexAddressMap := make(map[uint64]uint64)
	// Convert locations and mappings
	for i, loc := range src.Location {
		gl := convertLocationBack(loc)
		gl.Id = uint64(i + 1)
		if len(gl.Line) == 0 {
			gl.Line = append(gl.Line, &googleProfile.Line{
				FunctionId: loc.MappingIndex + functionOffset,
			})
		}
		dst.Location = append(dst.Location, gl)
		locationMappingIndexAddressMap[loc.MappingIndex] = loc.Address
	}

	for _, m := range src.Mapping {
		address, _ := locationMappingIndexAddressMap[m.Id]
		addressStr := fmt.Sprintf("%s 0x%x", dst.StringTable[m.Filename], address)
		dst.StringTable = append(dst.StringTable, addressStr)
		// i == 0 function_id = functionOffset
		id := uint64(len(dst.Function)) + 1
		dst.Function = append(dst.Function, &googleProfile.Function{
			Id:   id,
			Name: int64(len(dst.StringTable) - 1),
		})
	}

	// Convert samples
	for _, sample := range src.Sample {
		gs := convertSampleBack(sample, src.LocationIndices)
		dst.Sample = append(dst.Sample, gs)
	}

	if len(dst.SampleType) == 0 {
		dst.StringTable = append(dst.StringTable, "samples")
		dst.StringTable = append(dst.StringTable, "ms")
		dst.SampleType = []*googleProfile.ValueType{{
			Type: int64(len(dst.StringTable) - 2),
			Unit: int64(len(dst.StringTable) - 1),
		}}
		dst.DefaultSampleType = int64(len(dst.StringTable) - 2)
	}

	//b, _ := json.MarshalIndent(src, "", "  ")
	//fmt.Println("src:")
	//_, _ = fmt.Fprintln(os.Stdout, string(b))
	//b, _ = json.MarshalIndent(dst, "", "  ")
	//fmt.Println("dst:")
	//_, _ = fmt.Fprintln(os.Stdout, string(b))

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

func convertSampleBack(os *otelProfile.Sample, locationIndexes []int64) *googleProfile.Sample {
	gs := &googleProfile.Sample{
		Value: os.Value,
	}

	if len(gs.Value) == 0 {
		gs.Value = []int64{int64(len(os.TimestampsUnixNano))}
	}

	for i := os.LocationsStartIndex; i < os.LocationsStartIndex+os.LocationsLength; i++ {
		gs.LocationId = append(gs.LocationId, uint64(locationIndexes[i]+1))
	}

	return gs
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
