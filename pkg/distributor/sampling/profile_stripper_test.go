package sampling

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	profilev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
)

// stripTestProfile returns a deterministic profile without sample labels,
// so it maps to a single series.
func stripTestProfile() *profilev1.Profile {
	return &profilev1.Profile{
		SampleType: []*profilev1.ValueType{
			{Type: 1, Unit: 2},
			{Type: 3, Unit: 4},
		},
		Sample: []*profilev1.Sample{
			{LocationId: []uint64{1, 2}, Value: []int64{10, 1000}},
			{LocationId: []uint64{2}, Value: []int64{5, 500}},
			// Dropped by Normalize (negative value): must not count towards totals.
			{LocationId: []uint64{1}, Value: []int64{-1, 100}},
			// Dropped by sanitization (value length mismatch): must not count either.
			{LocationId: []uint64{1}, Value: []int64{7}},
		},
		Mapping: []*profilev1.Mapping{{Id: 1, HasFunctions: true}},
		Location: []*profilev1.Location{
			{Id: 1, MappingId: 1, Line: []*profilev1.Line{{FunctionId: 1}}},
			{Id: 2, MappingId: 1, Line: []*profilev1.Line{{FunctionId: 2}}},
		},
		Function: []*profilev1.Function{
			{Id: 1, Name: 5, SystemName: 5, Filename: 6},
			{Id: 2, Name: 7, SystemName: 7, Filename: 8},
		},
		StringTable: []string{
			"",
			"samples",
			"count",
			"cpu",
			"nanoseconds",
			"func-a",
			"app.py",
			"func-b",
			"path-b",
		},
		TimeNanos:     1000000000,
		DurationNanos: 10000000000,
		PeriodType:    &profilev1.ValueType{Type: 3, Unit: 4},
		Period:        10000000,
	}
}

// strippedTestProfile is what stripTestProfile must be reduced to when it
// is sampled out: a single sample with the totals and a string table that
// holds only the sample type units.
func strippedTestProfile() *profilev1.Profile {
	return &profilev1.Profile{
		SampleType: []*profilev1.ValueType{
			{Type: 1, Unit: 2},
			{Type: 3, Unit: 4},
		},
		Sample:        []*profilev1.Sample{{Value: []int64{15, 1500}}},
		StringTable:   []string{"", "samples", "count", "cpu", "nanoseconds"},
		TimeNanos:     1000000000,
		DurationNanos: 10000000000,
		PeriodType:    &profilev1.ValueType{Type: 3, Unit: 4},
		Period:        10000000,
	}
}

func TestProfileStripper_StripToTotals(t *testing.T) {
	p := stripTestProfile()
	NewProfileStripper().StripToTotals(p)

	want := strippedTestProfile()
	require.Len(t, p.Sample, 1)
	assert.Equal(t, want.Sample[0].Value, p.Sample[0].Value)
	assert.Empty(t, p.Sample[0].LocationId)
	assert.Empty(t, p.Sample[0].Label)
	assert.Empty(t, p.Location)
	assert.Empty(t, p.Function)
	assert.Empty(t, p.Mapping)
	assert.Equal(t, want.StringTable, p.StringTable)
	assert.Equal(t, want.SampleType, p.SampleType)
	assert.Equal(t, want.PeriodType, p.PeriodType)
	assert.Equal(t, want.SizeVT(), p.SizeVT())
}

func TestProfileStripper_StripToTotals_NoSamples(t *testing.T) {
	p := stripTestProfile()
	p.Sample = nil
	NewProfileStripper().StripToTotals(p)
	assert.Empty(t, p.Sample)
	assert.Empty(t, p.Location)
}

func TestProfileStripper_StripToTotals_AllSamplesInvalid(t *testing.T) {
	p := stripTestProfile()
	p.Sample = []*profilev1.Sample{
		{LocationId: []uint64{1}, Value: []int64{-1, 100}},
		{LocationId: []uint64{1}, Value: []int64{7}},
	}
	NewProfileStripper().StripToTotals(p)
	assert.Empty(t, p.Sample)
	assert.Empty(t, p.Location)
}

func TestProfileStripper_StripToTotals_SampleLabels(t *testing.T) {
	p := stripTestProfile()
	// Indices into the extended string table: span_id=9, abc=10, def=11.
	p.StringTable = append(p.StringTable, "span_id", "abc", "def")
	p.Sample = []*profilev1.Sample{
		{LocationId: []uint64{1}, Value: []int64{1, 10}, Label: []*profilev1.Label{{Key: 9, Str: 10}}},
		{LocationId: []uint64{2}, Value: []int64{2, 20}, Label: []*profilev1.Label{{Key: 9, Str: 10}}},
		{LocationId: []uint64{1}, Value: []int64{4, 40}, Label: []*profilev1.Label{{Key: 9, Str: 11}}},
		{LocationId: []uint64{2}, Value: []int64{8, 80}},
		{LocationId: []uint64{1}, Value: []int64{16, 160}, Label: []*profilev1.Label{{Key: 9, Num: 42}}},
	}

	NewProfileStripper().StripToTotals(p)

	require.Len(t, p.Sample, 1)
	assert.Equal(t, []int64{31, 310}, p.Sample[0].Value)
	assert.Empty(t, p.Sample[0].Label)
	assert.Empty(t, p.Sample[0].LocationId)
	assert.Empty(t, p.Location)
	assert.Empty(t, p.Function)
	assert.Empty(t, p.Mapping)
	assert.Equal(t, []string{"", "samples", "count", "cpu", "nanoseconds"}, p.StringTable)
}
