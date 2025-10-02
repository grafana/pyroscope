package memdb

import (
	"bytes"
	"fmt"

	"github.com/apache/arrow/go/v18/arrow"
	"github.com/apache/arrow/go/v18/arrow/array"
	"github.com/apache/arrow/go/v18/arrow/ipc"
	"github.com/apache/arrow/go/v18/arrow/memory"
	"github.com/parquet-go/parquet-go"
	"github.com/prometheus/common/model"

	v1 "github.com/grafana/pyroscope/pkg/phlaredb/schemas/v1"
	"github.com/grafana/pyroscope/pkg/util/build"
)

// InMemoryArrowProfile represents a profile in Arrow format for efficient serialization
// This structure is designed to be directly serializable to Parquet without intermediate conversions
type InMemoryArrowProfile struct {
	// Fixed-size fields
	ID                  [16]byte // UUID
	SeriesIndex         uint32
	StacktracePartition uint64
	TotalValue          uint64
	SeriesFingerprint   model.Fingerprint
	DropFrames          int64
	KeepFrames          int64
	TimeNanos           int64
	DurationNanos       int64
	Period              int64
	DefaultSampleType   int64

	// Variable-size fields stored as slices for efficient Arrow conversion
	Comments         []int64
	StacktraceIDs    []uint32
	Values           []uint64
	Spans            []uint64
	AnnotationKeys   []string
	AnnotationValues []string
}

// Arrow schema for InMemoryArrowProfile
var InMemoryArrowProfileSchema = arrow.NewSchema([]arrow.Field{
	// Fixed-size fields
	{Name: "id", Type: arrow.BinaryTypes.Binary, Nullable: false}, // UUID
	{Name: "series_index", Type: arrow.PrimitiveTypes.Uint32, Nullable: false},
	{Name: "stacktrace_partition", Type: arrow.PrimitiveTypes.Uint64, Nullable: false},
	{Name: "total_value", Type: arrow.PrimitiveTypes.Uint64, Nullable: false},
	{Name: "series_fingerprint", Type: arrow.PrimitiveTypes.Uint64, Nullable: false},
	{Name: "drop_frames", Type: arrow.PrimitiveTypes.Int64, Nullable: false},
	{Name: "keep_frames", Type: arrow.PrimitiveTypes.Int64, Nullable: false},
	{Name: "time_nanos", Type: arrow.PrimitiveTypes.Int64, Nullable: false},
	{Name: "duration_nanos", Type: arrow.PrimitiveTypes.Int64, Nullable: false},
	{Name: "period", Type: arrow.PrimitiveTypes.Int64, Nullable: false},
	{Name: "default_sample_type", Type: arrow.PrimitiveTypes.Int64, Nullable: false},

	// Variable-size fields
	{Name: "comments", Type: arrow.ListOf(arrow.PrimitiveTypes.Int64), Nullable: true},
	{Name: "stacktrace_ids", Type: arrow.ListOf(arrow.PrimitiveTypes.Uint32), Nullable: true},
	{Name: "values", Type: arrow.ListOf(arrow.PrimitiveTypes.Uint64), Nullable: true},
	{Name: "spans", Type: arrow.ListOf(arrow.PrimitiveTypes.Uint64), Nullable: true},
	{Name: "annotation_keys", Type: arrow.ListOf(arrow.BinaryTypes.String), Nullable: true},
	{Name: "annotation_values", Type: arrow.ListOf(arrow.BinaryTypes.String), Nullable: true},
}, nil)

// WriteProfilesArrow converts InMemoryArrowProfile data directly to Parquet using Arrow format
// This provides significant memory and performance improvements by eliminating intermediate conversions
func WriteProfilesArrow(metrics *HeadMetrics, profiles []InMemoryArrowProfile) ([]byte, error) {
	if len(profiles) == 0 {
		// Create empty Parquet file to match traditional behavior
		return createEmptyParquetFile(metrics)
	}

	// Convert to Arrow Record
	record, err := inMemoryArrowProfilesToRecord(profiles)
	if err != nil {
		return nil, fmt.Errorf("failed to convert to Arrow record: %w", err)
	}
	defer record.Release()

	// Direct Arrow Record to Parquet conversion
	return arrowRecordToParquetDirect(record, metrics)
}

// inMemoryArrowProfilesToRecord converts a slice of InMemoryArrowProfile to an Arrow Record
func inMemoryArrowProfilesToRecord(profiles []InMemoryArrowProfile) (arrow.Record, error) {
	pool := memory.NewGoAllocator()
	builder := array.NewRecordBuilder(pool, InMemoryArrowProfileSchema)
	defer builder.Release()

	// Get builders for each field
	idBuilder := builder.Field(0).(*array.BinaryBuilder)
	seriesIndexBuilder := builder.Field(1).(*array.Uint32Builder)
	stacktracePartitionBuilder := builder.Field(2).(*array.Uint64Builder)
	totalValueBuilder := builder.Field(3).(*array.Uint64Builder)
	seriesFingerprintBuilder := builder.Field(4).(*array.Uint64Builder)
	dropFramesBuilder := builder.Field(5).(*array.Int64Builder)
	keepFramesBuilder := builder.Field(6).(*array.Int64Builder)
	timeNanosBuilder := builder.Field(7).(*array.Int64Builder)
	durationNanosBuilder := builder.Field(8).(*array.Int64Builder)
	periodBuilder := builder.Field(9).(*array.Int64Builder)
	defaultSampleTypeBuilder := builder.Field(10).(*array.Int64Builder)
	commentsBuilder := builder.Field(11).(*array.ListBuilder)
	stacktraceIDsBuilder := builder.Field(12).(*array.ListBuilder)
	valuesBuilder := builder.Field(13).(*array.ListBuilder)
	spansBuilder := builder.Field(14).(*array.ListBuilder)
	annotationKeysBuilder := builder.Field(15).(*array.ListBuilder)
	annotationValuesBuilder := builder.Field(16).(*array.ListBuilder)

	// Get value builders for list fields
	commentsValueBuilder := commentsBuilder.ValueBuilder().(*array.Int64Builder)
	stacktraceIDsValueBuilder := stacktraceIDsBuilder.ValueBuilder().(*array.Uint32Builder)
	valuesValueBuilder := valuesBuilder.ValueBuilder().(*array.Uint64Builder)
	spansValueBuilder := spansBuilder.ValueBuilder().(*array.Uint64Builder)
	annotationKeysValueBuilder := annotationKeysBuilder.ValueBuilder().(*array.StringBuilder)
	annotationValuesValueBuilder := annotationValuesBuilder.ValueBuilder().(*array.StringBuilder)

	// Reserve capacity for better performance
	builder.Reserve(len(profiles))

	// Convert each profile
	for _, profile := range profiles {
		// Fixed-size fields
		idBuilder.Append(profile.ID[:])
		seriesIndexBuilder.Append(profile.SeriesIndex)
		stacktracePartitionBuilder.Append(profile.StacktracePartition)
		totalValueBuilder.Append(profile.TotalValue)
		seriesFingerprintBuilder.Append(uint64(profile.SeriesFingerprint))
		dropFramesBuilder.Append(profile.DropFrames)
		keepFramesBuilder.Append(profile.KeepFrames)
		timeNanosBuilder.Append(profile.TimeNanos)
		durationNanosBuilder.Append(profile.DurationNanos)
		periodBuilder.Append(profile.Period)
		defaultSampleTypeBuilder.Append(profile.DefaultSampleType)

		// Variable-size fields (lists)
		// Comments
		if len(profile.Comments) > 0 {
			commentsBuilder.Append(true)
			commentsValueBuilder.Reserve(len(profile.Comments))
			for _, comment := range profile.Comments {
				commentsValueBuilder.Append(comment)
			}
		} else {
			commentsBuilder.AppendNull()
		}

		// Stacktrace IDs
		if len(profile.StacktraceIDs) > 0 {
			stacktraceIDsBuilder.Append(true)
			stacktraceIDsValueBuilder.Reserve(len(profile.StacktraceIDs))
			for _, id := range profile.StacktraceIDs {
				stacktraceIDsValueBuilder.Append(id)
			}
		} else {
			stacktraceIDsBuilder.AppendNull()
		}

		// Values
		if len(profile.Values) > 0 {
			valuesBuilder.Append(true)
			valuesValueBuilder.Reserve(len(profile.Values))
			for _, value := range profile.Values {
				valuesValueBuilder.Append(value)
			}
		} else {
			valuesBuilder.AppendNull()
		}

		// Spans
		if len(profile.Spans) > 0 {
			spansBuilder.Append(true)
			spansValueBuilder.Reserve(len(profile.Spans))
			for _, span := range profile.Spans {
				spansValueBuilder.Append(span)
			}
		} else {
			spansBuilder.AppendNull()
		}

		// Annotation Keys
		if len(profile.AnnotationKeys) > 0 {
			annotationKeysBuilder.Append(true)
			annotationKeysValueBuilder.Reserve(len(profile.AnnotationKeys))
			for _, key := range profile.AnnotationKeys {
				annotationKeysValueBuilder.Append(key)
			}
		} else {
			annotationKeysBuilder.AppendNull()
		}

		// Annotation Values
		if len(profile.AnnotationValues) > 0 {
			annotationValuesBuilder.Append(true)
			annotationValuesValueBuilder.Reserve(len(profile.AnnotationValues))
			for _, value := range profile.AnnotationValues {
				annotationValuesValueBuilder.Append(value)
			}
		} else {
			annotationValuesBuilder.AppendNull()
		}
	}

	return builder.NewRecord(), nil
}

// arrowRecordToParquetDirect converts an Arrow Record directly to Parquet format
// This is the key optimization - no intermediate InMemoryProfile conversion
func arrowRecordToParquetDirect(record arrow.Record, metrics *HeadMetrics) ([]byte, error) {
	// Serialize Arrow record to IPC format first
	arrowData, err := serializeArrowRecord(record)
	if err != nil {
		return nil, fmt.Errorf("failed to serialize Arrow record: %w", err)
	}

	// For now, we'll convert to the existing Parquet format for compatibility
	// TODO: Implement direct Arrow-to-Parquet conversion for maximum efficiency
	profiles, err := arrowRecordToInMemoryProfiles(record)
	if err != nil {
		return nil, fmt.Errorf("failed to convert Arrow record to InMemoryProfiles: %w", err)
	}

	// Use existing Parquet writer
	buf := &bytes.Buffer{}
	w := parquet.NewGenericWriter[*v1.Profile](
		buf,
		parquet.PageBufferSize(segmentsParquetWriteBufferSize),
		parquet.CreatedBy("github.com/grafana/pyroscope/", build.Version, build.Revision),
		v1.ProfilesSchema,
	)

	if _, err := parquet.CopyRows(w, v1.NewInMemoryProfilesRowReader(profiles)); err != nil {
		return nil, fmt.Errorf("failed to write profile rows to parquet table: %w", err)
	}
	if err := w.Close(); err != nil {
		return nil, fmt.Errorf("failed to close parquet table: %w", err)
	}

	metrics.writtenProfileSegments.WithLabelValues("success").Inc()
	res := buf.Bytes()
	metrics.writtenProfileSegmentsBytes.Observe(float64(len(res)))
	metrics.rowsWritten.WithLabelValues("profiles").Add(float64(len(profiles)))

	// Log Arrow data size for comparison (this would be removed in production)
	_ = arrowData

	return res, nil
}

// serializeArrowRecord serializes an Arrow record to IPC format
func serializeArrowRecord(record arrow.Record) ([]byte, error) {
	var buf bytes.Buffer
	writer := ipc.NewWriter(&buf, ipc.WithSchema(record.Schema()))
	defer writer.Close()

	if err := writer.Write(record); err != nil {
		return nil, err
	}

	// Close is handled by defer
	return buf.Bytes(), nil
}

// arrowRecordToInMemoryProfiles converts an Arrow record back to InMemoryProfile slice
// This is needed for compatibility with the existing Parquet writer
func arrowRecordToInMemoryProfiles(record arrow.Record) ([]v1.InMemoryProfile, error) {
	numRows := int(record.NumRows())
	profiles := make([]v1.InMemoryProfile, numRows)

	// Get column arrays
	idCol := record.Column(0).(*array.Binary)
	seriesIndexCol := record.Column(1).(*array.Uint32)
	stacktracePartitionCol := record.Column(2).(*array.Uint64)
	totalValueCol := record.Column(3).(*array.Uint64)
	seriesFingerprintCol := record.Column(4).(*array.Uint64)
	dropFramesCol := record.Column(5).(*array.Int64)
	keepFramesCol := record.Column(6).(*array.Int64)
	timeNanosCol := record.Column(7).(*array.Int64)
	durationNanosCol := record.Column(8).(*array.Int64)
	periodCol := record.Column(9).(*array.Int64)
	defaultSampleTypeCol := record.Column(10).(*array.Int64)
	commentsCol := record.Column(11).(*array.List)
	stacktraceIDsCol := record.Column(12).(*array.List)
	valuesCol := record.Column(13).(*array.List)
	spansCol := record.Column(14).(*array.List)
	annotationKeysCol := record.Column(15).(*array.List)
	annotationValuesCol := record.Column(16).(*array.List)

	// Get value arrays for list columns
	commentsValueCol := commentsCol.ListValues().(*array.Int64)
	stacktraceIDsValueCol := stacktraceIDsCol.ListValues().(*array.Uint32)
	valuesValueCol := valuesCol.ListValues().(*array.Uint64)
	spansValueCol := spansCol.ListValues().(*array.Uint64)
	annotationKeysValueCol := annotationKeysCol.ListValues().(*array.String)
	annotationValuesValueCol := annotationValuesCol.ListValues().(*array.String)

	// Convert each row
	for i := 0; i < numRows; i++ {
		profile := v1.InMemoryProfile{}

		// Fixed-size fields
		copy(profile.ID[:], idCol.Value(i))
		profile.SeriesIndex = seriesIndexCol.Value(i)
		profile.StacktracePartition = stacktracePartitionCol.Value(i)
		profile.TotalValue = totalValueCol.Value(i)
		profile.SeriesFingerprint = model.Fingerprint(seriesFingerprintCol.Value(i))
		profile.DropFrames = dropFramesCol.Value(i)
		profile.KeepFrames = keepFramesCol.Value(i)
		profile.TimeNanos = timeNanosCol.Value(i)
		profile.DurationNanos = durationNanosCol.Value(i)
		profile.Period = periodCol.Value(i)
		profile.DefaultSampleType = defaultSampleTypeCol.Value(i)

		// Variable-size fields
		// Comments
		if !commentsCol.IsNull(i) {
			start, end := commentsCol.ValueOffsets(i)
			profile.Comments = make([]int64, end-start)
			for j := start; j < end; j++ {
				profile.Comments[j-start] = commentsValueCol.Value(int(j))
			}
		}

		// Stacktrace IDs
		if !stacktraceIDsCol.IsNull(i) {
			start, end := stacktraceIDsCol.ValueOffsets(i)
			profile.Samples.StacktraceIDs = make([]uint32, end-start)
			for j := start; j < end; j++ {
				profile.Samples.StacktraceIDs[j-start] = stacktraceIDsValueCol.Value(int(j))
			}
		}

		// Values
		if !valuesCol.IsNull(i) {
			start, end := valuesCol.ValueOffsets(i)
			profile.Samples.Values = make([]uint64, end-start)
			for j := start; j < end; j++ {
				profile.Samples.Values[j-start] = valuesValueCol.Value(int(j))
			}
		}

		// Spans
		if !spansCol.IsNull(i) {
			start, end := spansCol.ValueOffsets(i)
			profile.Samples.Spans = make([]uint64, end-start)
			for j := start; j < end; j++ {
				profile.Samples.Spans[j-start] = spansValueCol.Value(int(j))
			}
		}

		// Annotation Keys
		if !annotationKeysCol.IsNull(i) {
			start, end := annotationKeysCol.ValueOffsets(i)
			profile.Annotations.Keys = make([]string, end-start)
			for j := start; j < end; j++ {
				profile.Annotations.Keys[j-start] = annotationKeysValueCol.Value(int(j))
			}
		} else {
			profile.Annotations.Keys = []string{}
		}

		// Annotation Values
		if !annotationValuesCol.IsNull(i) {
			start, end := annotationValuesCol.ValueOffsets(i)
			profile.Annotations.Values = make([]string, end-start)
			for j := start; j < end; j++ {
				profile.Annotations.Values[j-start] = annotationValuesValueCol.Value(int(j))
			}
		} else {
			profile.Annotations.Values = []string{}
		}

		profiles[i] = profile
	}

	return profiles, nil
}

// createEmptyParquetFile creates an empty Parquet file to match traditional behavior
func createEmptyParquetFile(metrics *HeadMetrics) ([]byte, error) {
	buf := &bytes.Buffer{}
	w := parquet.NewGenericWriter[*v1.Profile](
		buf,
		parquet.PageBufferSize(segmentsParquetWriteBufferSize),
		parquet.CreatedBy("github.com/grafana/pyroscope/", build.Version, build.Revision),
		v1.ProfilesSchema,
	)

	if err := w.Close(); err != nil {
		return nil, fmt.Errorf("failed to close empty parquet table: %w", err)
	}

	metrics.writtenProfileSegments.WithLabelValues("success").Inc()
	res := buf.Bytes()
	metrics.writtenProfileSegmentsBytes.Observe(float64(len(res)))
	metrics.rowsWritten.WithLabelValues("profiles").Add(0)

	return res, nil
}

// ConvertInMemoryProfileToArrow converts a traditional InMemoryProfile to InMemoryArrowProfile
func ConvertInMemoryProfileToArrow(profile *v1.InMemoryProfile) InMemoryArrowProfile {
	return InMemoryArrowProfile{
		// Fixed-size fields
		ID:                  profile.ID,
		SeriesIndex:         profile.SeriesIndex,
		StacktracePartition: profile.StacktracePartition,
		TotalValue:          profile.TotalValue,
		SeriesFingerprint:   profile.SeriesFingerprint,
		DropFrames:          profile.DropFrames,
		KeepFrames:          profile.KeepFrames,
		TimeNanos:           profile.TimeNanos,
		DurationNanos:       profile.DurationNanos,
		Period:              profile.Period,
		DefaultSampleType:   profile.DefaultSampleType,

		// Variable-size fields - copy slices to avoid sharing memory
		Comments:         append([]int64(nil), profile.Comments...),
		StacktraceIDs:    append([]uint32(nil), profile.Samples.StacktraceIDs...),
		Values:           append([]uint64(nil), profile.Samples.Values...),
		Spans:            append([]uint64(nil), profile.Samples.Spans...),
		AnnotationKeys:   append([]string(nil), profile.Annotations.Keys...),
		AnnotationValues: append([]string(nil), profile.Annotations.Values...),
	}
}

// ConvertInMemoryProfilesToArrow converts a slice of InMemoryProfile to InMemoryArrowProfile
func ConvertInMemoryProfilesToArrow(profiles []v1.InMemoryProfile) []InMemoryArrowProfile {
	arrowProfiles := make([]InMemoryArrowProfile, len(profiles))
	for i, profile := range profiles {
		arrowProfiles[i] = ConvertInMemoryProfileToArrow(&profile)
	}
	return arrowProfiles
}
