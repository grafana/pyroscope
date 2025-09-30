package arrow

import (
	"bytes"
	"fmt"

	"github.com/apache/arrow/go/v12/arrow"
	"github.com/apache/arrow/go/v12/arrow/array"
	"github.com/apache/arrow/go/v12/arrow/ipc"
	"github.com/apache/arrow/go/v12/arrow/memory"

	profilev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	segmentwriterv1 "github.com/grafana/pyroscope/api/gen/proto/go/segmentwriter/v1"
)

// ProfileToArrow converts a pprof Profile to Arrow format
func ProfileToArrow(profile *profilev1.Profile, pool memory.Allocator) (*segmentwriterv1.ArrowProfileData, error) {
	if pool == nil {
		pool = memory.NewGoAllocator()
	}

	// Extract metadata
	metadata := &segmentwriterv1.ProfileMetadata{
		TimeNanos:         profile.TimeNanos,
		DurationNanos:     profile.DurationNanos,
		Period:            profile.Period,
		DropFrames:        profile.DropFrames,
		KeepFrames:        profile.KeepFrames,
		DefaultSampleType: profile.DefaultSampleType,
	}

	// Convert sample types
	for _, st := range profile.SampleType {
		metadata.SampleType = append(metadata.SampleType, &segmentwriterv1.ValueType{
			Type: st.Type,
			Unit: st.Unit,
		})
	}

	// Convert period type
	if profile.PeriodType != nil {
		metadata.PeriodType = &segmentwriterv1.ValueType{
			Type: profile.PeriodType.Type,
			Unit: profile.PeriodType.Unit,
		}
	}

	// Copy comments
	metadata.Comment = make([]int64, len(profile.Comment))
	copy(metadata.Comment, profile.Comment)

	// Create Arrow record batches
	samplesData, err := createSamplesRecord(profile, pool)
	if err != nil {
		return nil, fmt.Errorf("failed to create samples record: %w", err)
	}
	defer samplesData.Release()

	locationsData, err := createLocationsRecord(profile, pool)
	if err != nil {
		return nil, fmt.Errorf("failed to create locations record: %w", err)
	}
	defer locationsData.Release()

	functionsData, err := createFunctionsRecord(profile, pool)
	if err != nil {
		return nil, fmt.Errorf("failed to create functions record: %w", err)
	}
	defer functionsData.Release()

	mappingsData, err := createMappingsRecord(profile, pool)
	if err != nil {
		return nil, fmt.Errorf("failed to create mappings record: %w", err)
	}
	defer mappingsData.Release()

	stringsData, err := createStringsRecord(profile, pool)
	if err != nil {
		return nil, fmt.Errorf("failed to create strings record: %w", err)
	}
	defer stringsData.Release()

	// Serialize record batches to bytes using Arrow IPC format
	samplesBatch, err := serializeRecord(samplesData)
	if err != nil {
		return nil, fmt.Errorf("failed to serialize samples: %w", err)
	}

	locationsBatch, err := serializeRecord(locationsData)
	if err != nil {
		return nil, fmt.Errorf("failed to serialize locations: %w", err)
	}

	functionsBatch, err := serializeRecord(functionsData)
	if err != nil {
		return nil, fmt.Errorf("failed to serialize functions: %w", err)
	}

	mappingsBatch, err := serializeRecord(mappingsData)
	if err != nil {
		return nil, fmt.Errorf("failed to serialize mappings: %w", err)
	}

	stringsBatch, err := serializeRecord(stringsData)
	if err != nil {
		return nil, fmt.Errorf("failed to serialize strings: %w", err)
	}

	return &segmentwriterv1.ArrowProfileData{
		Metadata:       metadata,
		SamplesBatch:   samplesBatch,
		LocationsBatch: locationsBatch,
		FunctionsBatch: functionsBatch,
		MappingsBatch:  mappingsBatch,
		StringsBatch:   stringsBatch,
	}, nil
}

// createSamplesRecord creates an Arrow record for sample data
func createSamplesRecord(profile *profilev1.Profile, pool memory.Allocator) (arrow.Record, error) {
	builder := array.NewRecordBuilder(pool, SamplesSchema)
	defer builder.Release()

	sampleIDBuilder := builder.Field(0).(*array.Uint64Builder)
	locationIDBuilder := builder.Field(1).(*array.Uint64Builder)
	locationIndexBuilder := builder.Field(2).(*array.Uint32Builder)
	valueIndexBuilder := builder.Field(3).(*array.Uint32Builder)
	valueBuilder := builder.Field(4).(*array.Int64Builder)
	labelKeyBuilder := builder.Field(5).(*array.Int64Builder)
	labelStrBuilder := builder.Field(6).(*array.Int64Builder)
	labelNumBuilder := builder.Field(7).(*array.Int64Builder)
	labelNumUnitBuilder := builder.Field(8).(*array.Int64Builder)

	sampleID := uint64(0)
	for _, sample := range profile.Sample {
		// Add location IDs for this sample
		for locIndex, locationID := range sample.LocationId {
			// Add values for this sample/location combination
			for valueIndex, value := range sample.Value {
				sampleIDBuilder.Append(sampleID)
				locationIDBuilder.Append(locationID)
				locationIndexBuilder.Append(uint32(locIndex))
				valueIndexBuilder.Append(uint32(valueIndex))
				valueBuilder.Append(value)

				// For this combination, we might not have labels, so append nulls
				labelKeyBuilder.AppendNull()
				labelStrBuilder.AppendNull()
				labelNumBuilder.AppendNull()
				labelNumUnitBuilder.AppendNull()
			}
		}

		// Add labels for this sample (separate rows)
		for _, label := range sample.Label {
			// For labels, we add one row per label with null values for location/value info
			sampleIDBuilder.Append(sampleID)
			locationIDBuilder.Append(0) // No location for label rows
			locationIndexBuilder.Append(0)
			valueIndexBuilder.Append(0)
			valueBuilder.Append(0)

			labelKeyBuilder.Append(label.Key)
			if label.Str != 0 {
				labelStrBuilder.Append(label.Str)
			} else {
				labelStrBuilder.AppendNull()
			}
			if label.Num != 0 {
				labelNumBuilder.Append(label.Num)
			} else {
				labelNumBuilder.AppendNull()
			}
			if label.NumUnit != 0 {
				labelNumUnitBuilder.Append(label.NumUnit)
			} else {
				labelNumUnitBuilder.AppendNull()
			}
		}

		sampleID++
	}

	return builder.NewRecord(), nil
}

// createLocationsRecord creates an Arrow record for location data
func createLocationsRecord(profile *profilev1.Profile, pool memory.Allocator) (arrow.Record, error) {
	builder := array.NewRecordBuilder(pool, LocationsSchema)
	defer builder.Release()

	idBuilder := builder.Field(0).(*array.Uint64Builder)
	mappingIDBuilder := builder.Field(1).(*array.Uint64Builder)
	addressBuilder := builder.Field(2).(*array.Uint64Builder)
	isFoldedBuilder := builder.Field(3).(*array.BooleanBuilder)
	functionIDBuilder := builder.Field(4).(*array.Uint64Builder)
	lineBuilder := builder.Field(5).(*array.Int64Builder)

	for _, location := range profile.Location {
		// For each line in the location (inlined functions)
		if len(location.Line) == 0 {
			// Location without line info
			idBuilder.Append(location.Id)
			if location.MappingId != 0 {
				mappingIDBuilder.Append(location.MappingId)
			} else {
				mappingIDBuilder.AppendNull()
			}
			if location.Address != 0 {
				addressBuilder.Append(location.Address)
			} else {
				addressBuilder.AppendNull()
			}
			isFoldedBuilder.Append(location.IsFolded)
			functionIDBuilder.AppendNull()
			lineBuilder.AppendNull()
		} else {
			for _, line := range location.Line {
				idBuilder.Append(location.Id)
				if location.MappingId != 0 {
					mappingIDBuilder.Append(location.MappingId)
				} else {
					mappingIDBuilder.AppendNull()
				}
				if location.Address != 0 {
					addressBuilder.Append(location.Address)
				} else {
					addressBuilder.AppendNull()
				}
				isFoldedBuilder.Append(location.IsFolded)
				if line.FunctionId != 0 {
					functionIDBuilder.Append(line.FunctionId)
				} else {
					functionIDBuilder.AppendNull()
				}
				if line.Line != 0 {
					lineBuilder.Append(line.Line)
				} else {
					lineBuilder.AppendNull()
				}
			}
		}
	}

	return builder.NewRecord(), nil
}

// createFunctionsRecord creates an Arrow record for function data
func createFunctionsRecord(profile *profilev1.Profile, pool memory.Allocator) (arrow.Record, error) {
	builder := array.NewRecordBuilder(pool, FunctionsSchema)
	defer builder.Release()

	idBuilder := builder.Field(0).(*array.Uint64Builder)
	nameBuilder := builder.Field(1).(*array.Int64Builder)
	systemNameBuilder := builder.Field(2).(*array.Int64Builder)
	filenameBuilder := builder.Field(3).(*array.Int64Builder)
	startLineBuilder := builder.Field(4).(*array.Int64Builder)

	for _, function := range profile.Function {
		idBuilder.Append(function.Id)
		nameBuilder.Append(function.Name)

		if function.SystemName != 0 {
			systemNameBuilder.Append(function.SystemName)
		} else {
			systemNameBuilder.AppendNull()
		}

		if function.Filename != 0 {
			filenameBuilder.Append(function.Filename)
		} else {
			filenameBuilder.AppendNull()
		}

		if function.StartLine != 0 {
			startLineBuilder.Append(function.StartLine)
		} else {
			startLineBuilder.AppendNull()
		}
	}

	return builder.NewRecord(), nil
}

// createMappingsRecord creates an Arrow record for mapping data
func createMappingsRecord(profile *profilev1.Profile, pool memory.Allocator) (arrow.Record, error) {
	builder := array.NewRecordBuilder(pool, MappingsSchema)
	defer builder.Release()

	idBuilder := builder.Field(0).(*array.Uint64Builder)
	memoryStartBuilder := builder.Field(1).(*array.Uint64Builder)
	memoryLimitBuilder := builder.Field(2).(*array.Uint64Builder)
	fileOffsetBuilder := builder.Field(3).(*array.Uint64Builder)
	filenameBuilder := builder.Field(4).(*array.Int64Builder)
	buildIDBuilder := builder.Field(5).(*array.Int64Builder)
	hasFunctionsBuilder := builder.Field(6).(*array.BooleanBuilder)
	hasFilenamesBuilder := builder.Field(7).(*array.BooleanBuilder)
	hasLineNumbersBuilder := builder.Field(8).(*array.BooleanBuilder)
	hasInlineFramesBuilder := builder.Field(9).(*array.BooleanBuilder)

	for _, mapping := range profile.Mapping {
		idBuilder.Append(mapping.Id)
		memoryStartBuilder.Append(mapping.MemoryStart)
		memoryLimitBuilder.Append(mapping.MemoryLimit)
		fileOffsetBuilder.Append(mapping.FileOffset)

		if mapping.Filename != 0 {
			filenameBuilder.Append(mapping.Filename)
		} else {
			filenameBuilder.AppendNull()
		}

		if mapping.BuildId != 0 {
			buildIDBuilder.Append(mapping.BuildId)
		} else {
			buildIDBuilder.AppendNull()
		}

		hasFunctionsBuilder.Append(mapping.HasFunctions)
		hasFilenamesBuilder.Append(mapping.HasFilenames)
		hasLineNumbersBuilder.Append(mapping.HasLineNumbers)
		hasInlineFramesBuilder.Append(mapping.HasInlineFrames)
	}

	return builder.NewRecord(), nil
}

// createStringsRecord creates an Arrow record for string table data
func createStringsRecord(profile *profilev1.Profile, pool memory.Allocator) (arrow.Record, error) {
	builder := array.NewRecordBuilder(pool, StringsSchema)
	defer builder.Release()

	idBuilder := builder.Field(0).(*array.Uint64Builder)
	valueBuilder := builder.Field(1).(*array.StringBuilder)

	for i, str := range profile.StringTable {
		idBuilder.Append(uint64(i))
		valueBuilder.Append(str)
	}

	return builder.NewRecord(), nil
}

// serializeRecord serializes an Arrow record to bytes using IPC format
func serializeRecord(record arrow.Record) ([]byte, error) {
	var buf bytes.Buffer

	writer := ipc.NewWriter(&buf, ipc.WithSchema(record.Schema()))
	defer writer.Close()

	if err := writer.Write(record); err != nil {
		return nil, err
	}

	if err := writer.Close(); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}
