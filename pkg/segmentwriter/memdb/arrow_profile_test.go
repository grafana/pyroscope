package memdb

import (
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	v1 "github.com/grafana/pyroscope/pkg/phlaredb/schemas/v1"
)

func TestWriteProfilesArrow(t *testing.T) {
	tests := []struct {
		name     string
		profiles []InMemoryArrowProfile
	}{
		{
			name:     "empty profiles",
			profiles: []InMemoryArrowProfile{},
		},
		{
			name: "single profile",
			profiles: []InMemoryArrowProfile{
				createTestInMemoryArrowProfile(1, 10),
			},
		},
		{
			name: "multiple profiles",
			profiles: []InMemoryArrowProfile{
				createTestInMemoryArrowProfile(1, 10),
				createTestInMemoryArrowProfile(2, 20),
				createTestInMemoryArrowProfile(3, 30),
			},
		},
		{
			name: "profile with empty samples",
			profiles: []InMemoryArrowProfile{
				createTestInMemoryArrowProfileWithEmptySamples(1),
			},
		},
		{
			name: "profile with annotations",
			profiles: []InMemoryArrowProfile{
				createTestInMemoryArrowProfileWithAnnotations(1, 5),
			},
		},
		{
			name: "profile with comments",
			profiles: []InMemoryArrowProfile{
				createTestInMemoryArrowProfileWithComments(1),
			},
		},
		{
			name: "profile with spans",
			profiles: []InMemoryArrowProfile{
				createTestInMemoryArrowProfileWithSpans(1, 5),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			metrics := NewHeadMetricsWithPrefix(prometheus.NewRegistry(), "")

			// Test Arrow-based implementation
			arrowResult, err := WriteProfilesArrow(metrics, tt.profiles)
			require.NoError(t, err, "WriteProfilesArrow should not return error")

			// Test traditional WriteProfiles function (which now uses Arrow internally)
			traditionalResult, err := WriteProfiles(metrics, tt.profiles)
			require.NoError(t, err, "WriteProfiles should not return error")

			// Both should produce valid Parquet data
			assert.NotNil(t, arrowResult, "Arrow result should not be nil")
			assert.NotNil(t, traditionalResult, "Traditional result should not be nil")

			// For empty profiles, both should return valid Parquet data (empty file)
			if len(tt.profiles) == 0 {
				assert.True(t, len(arrowResult) > 0, "Arrow result should contain Parquet data even for empty profiles")
				assert.True(t, len(traditionalResult) > 0, "Traditional result should contain Parquet data even for empty profiles")
				return
			}

			// Both results should be valid Parquet data
			assert.True(t, len(arrowResult) > 0, "Arrow result should contain data")
			assert.True(t, len(traditionalResult) > 0, "Traditional result should contain data")

			// Verify that both produce valid Parquet files
			assertValidParquetData(t, arrowResult, "Arrow result")
			assertValidParquetData(t, traditionalResult, "Traditional result")
		})
	}
}

func TestInMemoryArrowProfilesToRecord(t *testing.T) {
	profiles := []InMemoryArrowProfile{
		createTestInMemoryArrowProfile(1, 10),
		createTestInMemoryArrowProfile(2, 20),
	}

	record, err := inMemoryArrowProfilesToRecord(profiles)
	require.NoError(t, err)
	defer record.Release()

	// Verify record properties
	assert.Equal(t, int64(2), record.NumRows(), "Record should have 2 rows")
	assert.Equal(t, 17, len(record.Schema().Fields()), "Record should have 17 fields")

	// Verify schema matches expected schema
	assert.Equal(t, InMemoryArrowProfileSchema, record.Schema(), "Record schema should match expected schema")
}

func TestArrowRecordToInMemoryProfiles(t *testing.T) {
	originalProfiles := []InMemoryArrowProfile{
		createTestInMemoryArrowProfile(1, 10),
		createTestInMemoryArrowProfile(2, 20),
		createTestInMemoryArrowProfileWithAnnotations(3, 5),
	}

	// Convert to Arrow record
	record, err := inMemoryArrowProfilesToRecord(originalProfiles)
	require.NoError(t, err)
	defer record.Release()

	// Convert back to InMemoryProfile (for compatibility with existing Parquet writer)
	convertedProfiles, err := arrowRecordToInMemoryProfiles(record)
	require.NoError(t, err)

	// Verify round-trip conversion
	assert.Equal(t, len(originalProfiles), len(convertedProfiles), "Should have same number of profiles")

	for i, original := range originalProfiles {
		converted := convertedProfiles[i]
		assertInMemoryProfileEqual(t, original, converted, "Profile %d should be equal after round-trip", i)
	}
}

func TestArrowRecordToParquetDirect(t *testing.T) {
	profiles := []InMemoryArrowProfile{
		createTestInMemoryArrowProfile(1, 10),
		createTestInMemoryArrowProfile(2, 20),
	}

	// Convert to Arrow record
	record, err := inMemoryArrowProfilesToRecord(profiles)
	require.NoError(t, err)
	defer record.Release()

	// Convert to Parquet
	metrics := NewHeadMetricsWithPrefix(prometheus.NewRegistry(), "")
	parquetData, err := arrowRecordToParquetDirect(record, metrics)
	require.NoError(t, err)

	// Verify Parquet data
	assert.NotNil(t, parquetData, "Parquet data should not be nil")
	assert.True(t, len(parquetData) > 0, "Parquet data should contain content")
	assertValidParquetData(t, parquetData, "Arrow-to-Parquet result")
}

func TestSerializeArrowRecord(t *testing.T) {
	profiles := []InMemoryArrowProfile{
		createTestInMemoryArrowProfile(1, 10),
	}

	record, err := inMemoryArrowProfilesToRecord(profiles)
	require.NoError(t, err)
	defer record.Release()

	// Serialize to IPC format
	arrowData, err := serializeArrowRecord(record)
	require.NoError(t, err)

	// Verify serialized data
	assert.NotNil(t, arrowData, "Arrow data should not be nil")
	assert.True(t, len(arrowData) > 0, "Arrow data should contain content")

	// Verify it's valid Arrow IPC data (basic check)
	assert.True(t, len(arrowData) > 8, "Arrow IPC data should have header")
}

func TestConvertInMemoryProfileToArrow(t *testing.T) {
	original := createTestInMemoryProfile(1, 10)
	arrowProfile := ConvertInMemoryProfileToArrow(&original)

	// Verify conversion
	assert.Equal(t, original.ID, uuid.UUID(arrowProfile.ID), "ID should be equal")
	assert.Equal(t, original.SeriesIndex, arrowProfile.SeriesIndex, "SeriesIndex should be equal")
	assert.Equal(t, original.StacktracePartition, arrowProfile.StacktracePartition, "StacktracePartition should be equal")
	assert.Equal(t, original.TotalValue, arrowProfile.TotalValue, "TotalValue should be equal")
	assert.Equal(t, original.SeriesFingerprint, arrowProfile.SeriesFingerprint, "SeriesFingerprint should be equal")
	assert.Equal(t, original.DropFrames, arrowProfile.DropFrames, "DropFrames should be equal")
	assert.Equal(t, original.KeepFrames, arrowProfile.KeepFrames, "KeepFrames should be equal")
	assert.Equal(t, original.TimeNanos, arrowProfile.TimeNanos, "TimeNanos should be equal")
	assert.Equal(t, original.DurationNanos, arrowProfile.DurationNanos, "DurationNanos should be equal")
	assert.Equal(t, original.Period, arrowProfile.Period, "Period should be equal")
	assert.Equal(t, original.DefaultSampleType, arrowProfile.DefaultSampleType, "DefaultSampleType should be equal")
	assert.Equal(t, original.Comments, arrowProfile.Comments, "Comments should be equal")
	assert.Equal(t, original.Samples.StacktraceIDs, arrowProfile.StacktraceIDs, "StacktraceIDs should be equal")
	assert.Equal(t, original.Samples.Values, arrowProfile.Values, "Values should be equal")
	assert.Equal(t, original.Samples.Spans, arrowProfile.Spans, "Spans should be equal")
	// Handle nil vs empty slice differences for annotations
	if len(original.Annotations.Keys) == 0 && len(arrowProfile.AnnotationKeys) == 0 {
		// Both are effectively empty, consider equal
	} else {
		assert.Equal(t, original.Annotations.Keys, arrowProfile.AnnotationKeys, "Annotation Keys should be equal")
	}
	if len(original.Annotations.Values) == 0 && len(arrowProfile.AnnotationValues) == 0 {
		// Both are effectively empty, consider equal
	} else {
		assert.Equal(t, original.Annotations.Values, arrowProfile.AnnotationValues, "Annotation Values should be equal")
	}
}

func TestConvertInMemoryProfilesToArrow(t *testing.T) {
	originals := []v1.InMemoryProfile{
		createTestInMemoryProfile(1, 10),
		createTestInMemoryProfile(2, 20),
		createTestInMemoryProfileWithAnnotations(3, 5),
	}

	arrowProfiles := ConvertInMemoryProfilesToArrow(originals)

	// Verify conversion
	assert.Equal(t, len(originals), len(arrowProfiles), "Should have same number of profiles")

	for i, original := range originals {
		arrowProfile := arrowProfiles[i]
		assert.Equal(t, original.ID, uuid.UUID(arrowProfile.ID), "Profile %d ID should be equal", i)
		assert.Equal(t, original.SeriesIndex, arrowProfile.SeriesIndex, "Profile %d SeriesIndex should be equal", i)
		assert.Equal(t, original.Samples.StacktraceIDs, arrowProfile.StacktraceIDs, "Profile %d StacktraceIDs should be equal", i)
		assert.Equal(t, original.Samples.Values, arrowProfile.Values, "Profile %d Values should be equal", i)
	}
}

// Helper functions for creating test data

func createTestInMemoryArrowProfile(id int, numSamples int) InMemoryArrowProfile {
	profile := InMemoryArrowProfile{
		ID:                  uuid.New(),
		SeriesIndex:         uint32(id),
		StacktracePartition: uint64(id * 1000),
		TotalValue:          uint64(numSamples * 100),
		SeriesFingerprint:   model.Fingerprint(id * 10000),
		DropFrames:          int64(id * 10),
		KeepFrames:          int64(id * 20),
		TimeNanos:           int64(id * 1000000000),
		DurationNanos:       int64(id * 1000000),
		Period:              int64(id * 1000),
		DefaultSampleType:   int64(id),
		Comments:            []int64{},
		StacktraceIDs:       make([]uint32, numSamples),
		Values:              make([]uint64, numSamples),
		Spans:               []uint64{},
		AnnotationKeys:      []string{},
		AnnotationValues:    []string{},
	}

	// Fill samples
	for i := 0; i < numSamples; i++ {
		profile.StacktraceIDs[i] = uint32(id*1000 + i)
		profile.Values[i] = uint64(id*100 + i)
	}

	return profile
}

func createTestInMemoryArrowProfileWithEmptySamples(id int) InMemoryArrowProfile {
	profile := createTestInMemoryArrowProfile(id, 0)
	profile.StacktraceIDs = []uint32{}
	profile.Values = []uint64{}
	profile.Spans = []uint64{}
	return profile
}

func createTestInMemoryArrowProfileWithAnnotations(id int, numAnnotations int) InMemoryArrowProfile {
	profile := createTestInMemoryArrowProfile(id, 5)

	profile.AnnotationKeys = make([]string, numAnnotations)
	profile.AnnotationValues = make([]string, numAnnotations)

	for i := 0; i < numAnnotations; i++ {
		profile.AnnotationKeys[i] = fmt.Sprintf("key_%d_%d", id, i)
		profile.AnnotationValues[i] = fmt.Sprintf("value_%d_%d", id, i)
	}

	return profile
}

func createTestInMemoryArrowProfileWithComments(id int) InMemoryArrowProfile {
	profile := createTestInMemoryArrowProfile(id, 3)
	profile.Comments = []int64{int64(id * 100), int64(id * 200), int64(id * 300)}
	return profile
}

func createTestInMemoryArrowProfileWithSpans(id int, numSpans int) InMemoryArrowProfile {
	profile := createTestInMemoryArrowProfile(id, numSpans)
	profile.Spans = make([]uint64, numSpans)

	for i := 0; i < numSpans; i++ {
		profile.Spans[i] = uint64(id*10000 + i)
	}

	return profile
}

// Helper functions for assertions

func assertInMemoryProfileEqual(t *testing.T, expected InMemoryArrowProfile, actual v1.InMemoryProfile, msgAndArgs ...interface{}) {
	assert.Equal(t, uuid.UUID(expected.ID), actual.ID, "ID should be equal")
	assert.Equal(t, expected.SeriesIndex, actual.SeriesIndex, "SeriesIndex should be equal")
	assert.Equal(t, expected.StacktracePartition, actual.StacktracePartition, "StacktracePartition should be equal")
	assert.Equal(t, expected.TotalValue, actual.TotalValue, "TotalValue should be equal")
	assert.Equal(t, expected.SeriesFingerprint, actual.SeriesFingerprint, "SeriesFingerprint should be equal")
	assert.Equal(t, expected.DropFrames, actual.DropFrames, "DropFrames should be equal")
	assert.Equal(t, expected.KeepFrames, actual.KeepFrames, "KeepFrames should be equal")
	assert.Equal(t, expected.TimeNanos, actual.TimeNanos, "TimeNanos should be equal")
	assert.Equal(t, expected.DurationNanos, actual.DurationNanos, "DurationNanos should be equal")
	assert.Equal(t, expected.Period, actual.Period, "Period should be equal")
	assert.Equal(t, expected.DefaultSampleType, actual.DefaultSampleType, "DefaultSampleType should be equal")
	// Handle nil vs empty slice differences
	if len(expected.Comments) == 0 && actual.Comments == nil {
		// Both are effectively empty, consider equal
	} else {
		assert.Equal(t, expected.Comments, actual.Comments, "Comments should be equal")
	}

	// Samples
	assert.Equal(t, expected.StacktraceIDs, actual.Samples.StacktraceIDs, "StacktraceIDs should be equal")
	assert.Equal(t, expected.Values, actual.Samples.Values, "Values should be equal")
	// Handle nil vs empty slice differences for Spans
	if len(expected.Spans) == 0 && actual.Samples.Spans == nil {
		// Both are effectively empty, consider equal
	} else {
		assert.Equal(t, expected.Spans, actual.Samples.Spans, "Spans should be equal")
	}

	// Annotations - handle nil vs empty slice differences
	if len(expected.AnnotationKeys) == 0 && actual.Annotations.Keys == nil {
		// Both are effectively empty, consider equal
	} else {
		assert.Equal(t, expected.AnnotationKeys, actual.Annotations.Keys, "Annotation Keys should be equal")
	}
	if len(expected.AnnotationValues) == 0 && actual.Annotations.Values == nil {
		// Both are effectively empty, consider equal
	} else {
		assert.Equal(t, expected.AnnotationValues, actual.Annotations.Values, "Annotation Values should be equal")
	}
}

func assertValidParquetData(t *testing.T, data []byte, description string) {
	// Basic validation that this looks like Parquet data
	// Parquet files start with "PAR1" magic number
	if len(data) < 4 {
		t.Errorf("%s: data too short to be valid Parquet", description)
		return
	}

	// Check for Parquet magic number at the end (Parquet files end with "PAR1")
	if len(data) >= 8 {
		magic := string(data[len(data)-4:])
		if magic != "PAR1" {
			t.Errorf("%s: does not end with Parquet magic number 'PAR1', got '%s'", description, magic)
		}
	}
}

// Helper functions for creating traditional InMemoryProfile test data

func createTestInMemoryProfile(id int, numSamples int) v1.InMemoryProfile {
	profile := v1.InMemoryProfile{
		ID:                  uuid.New(),
		SeriesIndex:         uint32(id),
		StacktracePartition: uint64(id * 1000),
		TotalValue:          uint64(numSamples * 100),
		SeriesFingerprint:   model.Fingerprint(id * 10000),
		DropFrames:          int64(id * 10),
		KeepFrames:          int64(id * 20),
		TimeNanos:           int64(id * 1000000000),
		DurationNanos:       int64(id * 1000000),
		Period:              int64(id * 1000),
		DefaultSampleType:   int64(id),
		Samples:             v1.NewSamples(numSamples),
		Annotations: v1.Annotations{
			Keys:   make([]string, 0),
			Values: make([]string, 0),
		},
	}

	// Fill samples
	for i := 0; i < numSamples; i++ {
		profile.Samples.StacktraceIDs[i] = uint32(id*1000 + i)
		profile.Samples.Values[i] = uint64(id*100 + i)
	}

	return profile
}

func createTestInMemoryProfileWithAnnotations(id int, numAnnotations int) v1.InMemoryProfile {
	profile := createTestInMemoryProfile(id, 5)

	profile.Annotations.Keys = make([]string, numAnnotations)
	profile.Annotations.Values = make([]string, numAnnotations)

	for i := 0; i < numAnnotations; i++ {
		profile.Annotations.Keys[i] = fmt.Sprintf("key_%d_%d", id, i)
		profile.Annotations.Values[i] = fmt.Sprintf("value_%d_%d", id, i)
	}

	return profile
}
