package arrow

import (
	"bytes"
	"fmt"
	"sort"

	"github.com/apache/arrow/go/v18/arrow"
	"github.com/apache/arrow/go/v18/arrow/array"
	"github.com/apache/arrow/go/v18/arrow/ipc"
	"github.com/apache/arrow/go/v18/arrow/memory"

	profilev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	segmentwriterv1 "github.com/grafana/pyroscope/api/gen/proto/go/segmentwriter/v1"
)

// ArrowToProfile converts Arrow format data back to a pprof Profile
func ArrowToProfile(arrowData *segmentwriterv1.ArrowProfileData, pool memory.Allocator) (*profilev1.Profile, error) {
	if pool == nil {
		pool = memory.NewGoAllocator()
	}

	// Deserialize Arrow records
	stringsRecord, err := deserializeRecord(arrowData.StringsBatch, pool)
	if err != nil {
		return nil, fmt.Errorf("failed to deserialize strings: %w", err)
	}
	defer stringsRecord.Release()

	functionsRecord, err := deserializeRecord(arrowData.FunctionsBatch, pool)
	if err != nil {
		return nil, fmt.Errorf("failed to deserialize functions: %w", err)
	}
	defer functionsRecord.Release()

	mappingsRecord, err := deserializeRecord(arrowData.MappingsBatch, pool)
	if err != nil {
		return nil, fmt.Errorf("failed to deserialize mappings: %w", err)
	}
	defer mappingsRecord.Release()

	locationsRecord, err := deserializeRecord(arrowData.LocationsBatch, pool)
	if err != nil {
		return nil, fmt.Errorf("failed to deserialize locations: %w", err)
	}
	defer locationsRecord.Release()

	samplesRecord, err := deserializeRecord(arrowData.SamplesBatch, pool)
	if err != nil {
		return nil, fmt.Errorf("failed to deserialize samples: %w", err)
	}
	defer samplesRecord.Release()

	// Create the profile
	profile := &profilev1.Profile{}

	// Convert metadata
	metadata := arrowData.Metadata
	profile.TimeNanos = metadata.TimeNanos
	profile.DurationNanos = metadata.DurationNanos
	profile.Period = metadata.Period
	profile.DropFrames = metadata.DropFrames
	profile.KeepFrames = metadata.KeepFrames
	profile.DefaultSampleType = metadata.DefaultSampleType

	// Convert sample types
	for _, st := range metadata.SampleType {
		profile.SampleType = append(profile.SampleType, &profilev1.ValueType{
			Type: st.Type,
			Unit: st.Unit,
		})
	}

	// Convert period type
	if metadata.PeriodType != nil {
		profile.PeriodType = &profilev1.ValueType{
			Type: metadata.PeriodType.Type,
			Unit: metadata.PeriodType.Unit,
		}
	}

	// Copy comments - preserve nil vs empty slice distinction
	if len(metadata.Comment) > 0 {
		profile.Comment = make([]int64, len(metadata.Comment))
		copy(profile.Comment, metadata.Comment)
	} else {
		profile.Comment = nil
	}

	// Convert string table
	profile.StringTable = extractStringTable(stringsRecord)

	// Convert mappings
	profile.Mapping = extractMappings(mappingsRecord)

	// Convert functions
	profile.Function = extractFunctions(functionsRecord)

	// Convert locations
	profile.Location = extractLocations(locationsRecord)

	// Convert samples
	profile.Sample = extractSamples(samplesRecord, len(profile.SampleType))

	return profile, nil
}

// deserializeRecord deserializes bytes back to an Arrow record
func deserializeRecord(data []byte, pool memory.Allocator) (arrow.Record, error) {
	reader, err := ipc.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer reader.Release()

	if !reader.Next() {
		return nil, fmt.Errorf("no records found")
	}

	rec := reader.Record()
	rec.Retain() // Keep the record alive after reader is released
	return rec, nil
}

// extractStringTable extracts string table from Arrow record
func extractStringTable(record arrow.Record) []string {
	valueCol := record.Column(1).(*array.String)

	strings := make([]string, record.NumRows())
	for i := 0; i < int(record.NumRows()); i++ {
		strings[i] = valueCol.Value(i)
	}

	return strings
}

// extractMappings extracts mappings from Arrow record
func extractMappings(record arrow.Record) []*profilev1.Mapping {
	mappings := make([]*profilev1.Mapping, record.NumRows())

	idCol := record.Column(0).(*array.Uint64)
	memoryStartCol := record.Column(1).(*array.Uint64)
	memoryLimitCol := record.Column(2).(*array.Uint64)
	fileOffsetCol := record.Column(3).(*array.Uint64)
	filenameCol := record.Column(4).(*array.Int64)
	buildIDCol := record.Column(5).(*array.Int64)
	hasFunctionsCol := record.Column(6).(*array.Boolean)
	hasFilenamesCol := record.Column(7).(*array.Boolean)
	hasLineNumbersCol := record.Column(8).(*array.Boolean)
	hasInlineFramesCol := record.Column(9).(*array.Boolean)

	for i := 0; i < int(record.NumRows()); i++ {
		mapping := &profilev1.Mapping{
			Id:              idCol.Value(i),
			MemoryStart:     memoryStartCol.Value(i),
			MemoryLimit:     memoryLimitCol.Value(i),
			FileOffset:      fileOffsetCol.Value(i),
			HasFunctions:    hasFunctionsCol.Value(i),
			HasFilenames:    hasFilenamesCol.Value(i),
			HasLineNumbers:  hasLineNumbersCol.Value(i),
			HasInlineFrames: hasInlineFramesCol.Value(i),
		}

		if !filenameCol.IsNull(i) {
			mapping.Filename = filenameCol.Value(i)
		}
		if !buildIDCol.IsNull(i) {
			mapping.BuildId = buildIDCol.Value(i)
		}

		mappings[i] = mapping
	}

	// Sort by ID to maintain consistent ordering
	sort.Slice(mappings, func(i, j int) bool {
		return mappings[i].Id < mappings[j].Id
	})

	return mappings
}

// extractFunctions extracts functions from Arrow record
func extractFunctions(record arrow.Record) []*profilev1.Function {
	functions := make([]*profilev1.Function, record.NumRows())

	idCol := record.Column(0).(*array.Uint64)
	nameCol := record.Column(1).(*array.Int64)
	systemNameCol := record.Column(2).(*array.Int64)
	filenameCol := record.Column(3).(*array.Int64)
	startLineCol := record.Column(4).(*array.Int64)

	for i := 0; i < int(record.NumRows()); i++ {
		function := &profilev1.Function{
			Id:   idCol.Value(i),
			Name: nameCol.Value(i),
		}

		if !systemNameCol.IsNull(i) {
			function.SystemName = systemNameCol.Value(i)
		}
		if !filenameCol.IsNull(i) {
			function.Filename = filenameCol.Value(i)
		}
		if !startLineCol.IsNull(i) {
			function.StartLine = startLineCol.Value(i)
		}

		functions[i] = function
	}

	// Sort by ID to maintain consistent ordering
	sort.Slice(functions, func(i, j int) bool {
		return functions[i].Id < functions[j].Id
	})

	return functions
}

// extractLocations extracts locations from Arrow record
func extractLocations(record arrow.Record) []*profilev1.Location {
	// Group by location ID since each location can have multiple lines (inlined functions)
	locationMap := make(map[uint64]*profilev1.Location)

	idCol := record.Column(0).(*array.Uint64)
	mappingIDCol := record.Column(1).(*array.Uint64)
	addressCol := record.Column(2).(*array.Uint64)
	isFoldedCol := record.Column(3).(*array.Boolean)
	functionIDCol := record.Column(4).(*array.Uint64)
	lineCol := record.Column(5).(*array.Int64)

	for i := 0; i < int(record.NumRows()); i++ {
		id := idCol.Value(i)

		location, exists := locationMap[id]
		if !exists {
			location = &profilev1.Location{
				Id:       id,
				IsFolded: isFoldedCol.Value(i),
			}

			if !mappingIDCol.IsNull(i) {
				location.MappingId = mappingIDCol.Value(i)
			}
			if !addressCol.IsNull(i) {
				location.Address = addressCol.Value(i)
			}

			locationMap[id] = location
		}

		// Add line information if present
		if !functionIDCol.IsNull(i) || !lineCol.IsNull(i) {
			line := &profilev1.Line{}

			if !functionIDCol.IsNull(i) {
				line.FunctionId = functionIDCol.Value(i)
			}
			if !lineCol.IsNull(i) {
				line.Line = lineCol.Value(i)
			}

			location.Line = append(location.Line, line)
		}
	}

	// Convert map to slice and sort by ID to maintain consistent ordering
	locations := make([]*profilev1.Location, 0, len(locationMap))
	for _, location := range locationMap {
		locations = append(locations, location)
	}

	// Sort by ID to maintain consistent ordering
	sort.Slice(locations, func(i, j int) bool {
		return locations[i].Id < locations[j].Id
	})

	return locations
}

// extractSamples extracts samples from Arrow record
func extractSamples(record arrow.Record, numValueTypes int) []*profilev1.Sample {
	// Track data by sample ID with proper ordering
	type sampleData struct {
		sample    *profilev1.Sample
		locations []struct {
			id    uint64
			index uint32
		}
		labels []struct {
			label *profilev1.Label
			order int // Track the order we encountered labels
		}
		sampleID uint64
	}

	sampleMap := make(map[uint64]*sampleData)

	sampleIDCol := record.Column(0).(*array.Uint64)
	locationIDCol := record.Column(1).(*array.Uint64)
	locationIndexCol := record.Column(2).(*array.Uint32)
	valueIndexCol := record.Column(3).(*array.Uint32)
	valueCol := record.Column(4).(*array.Int64)
	labelKeyCol := record.Column(5).(*array.Int64)
	labelStrCol := record.Column(6).(*array.Int64)
	labelNumCol := record.Column(7).(*array.Int64)
	labelNumUnitCol := record.Column(8).(*array.Int64)

	labelOrder := 0
	for i := 0; i < int(record.NumRows()); i++ {
		sampleID := sampleIDCol.Value(i)

		data, exists := sampleMap[sampleID]
		if !exists {
			data = &sampleData{
				sample: &profilev1.Sample{
					Value: make([]int64, numValueTypes),
				},
				sampleID: sampleID,
			}
			sampleMap[sampleID] = data
		}

		locationID := locationIDCol.Value(i)
		valueIndex := valueIndexCol.Value(i)
		value := valueCol.Value(i)

		// Handle location/value data
		if locationID != 0 && value != 0 {
			locationIndex := locationIndexCol.Value(i)

			// Check if this location is already tracked
			found := false
			for _, loc := range data.locations {
				if loc.id == locationID && loc.index == locationIndex {
					found = true
					break
				}
			}

			if !found {
				data.locations = append(data.locations, struct {
					id    uint64
					index uint32
				}{id: locationID, index: locationIndex})
			}

			// Set the value for this value type
			if int(valueIndex) < len(data.sample.Value) {
				data.sample.Value[valueIndex] = value
			}
		}

		// Handle label data
		if !labelKeyCol.IsNull(i) {
			label := &profilev1.Label{
				Key: labelKeyCol.Value(i),
			}

			if !labelStrCol.IsNull(i) {
				label.Str = labelStrCol.Value(i)
			}
			if !labelNumCol.IsNull(i) {
				label.Num = labelNumCol.Value(i)
			}
			if !labelNumUnitCol.IsNull(i) {
				label.NumUnit = labelNumUnitCol.Value(i)
			}

			data.labels = append(data.labels, struct {
				label *profilev1.Label
				order int
			}{label: label, order: labelOrder})
			labelOrder++
		}
	}

	// Convert map to slice and restore proper ordering
	samples := make([]*profilev1.Sample, 0, len(sampleMap))
	sampleIDs := make([]uint64, 0, len(sampleMap))

	for sampleID := range sampleMap {
		sampleIDs = append(sampleIDs, sampleID)
	}

	// Sort sample IDs to maintain consistent order
	sort.Slice(sampleIDs, func(i, j int) bool {
		return sampleIDs[i] < sampleIDs[j]
	})

	for _, sampleID := range sampleIDs {
		data := sampleMap[sampleID]

		// Sort locations by their original index within the sample
		sort.Slice(data.locations, func(i, j int) bool {
			return data.locations[i].index < data.locations[j].index
		})

		// Reconstruct location IDs in proper order
		for _, loc := range data.locations {
			data.sample.LocationId = append(data.sample.LocationId, loc.id)
		}

		// Sort labels by order encountered
		sort.Slice(data.labels, func(i, j int) bool {
			return data.labels[i].order < data.labels[j].order
		})

		// Reconstruct labels in proper order
		for _, labelData := range data.labels {
			data.sample.Label = append(data.sample.Label, labelData.label)
		}

		samples = append(samples, data.sample)
	}

	return samples
}
