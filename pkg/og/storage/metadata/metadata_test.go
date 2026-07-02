package metadata

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsValidUnit(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		unit  string
		valid bool
	}{
		// Positive cases
		{name: "samples", unit: string(SamplesUnits), valid: true},
		{name: "objects", unit: string(ObjectsUnits), valid: true},
		{name: "goroutines", unit: string(GoroutinesUnits), valid: true},
		{name: "bytes", unit: string(BytesUnits), valid: true},
		{name: "lock_nanoseconds", unit: string(LockNanosecondsUnits), valid: true},
		{name: "lock_samples", unit: string(LockSamplesUnits), valid: true},

		// Negative cases
		{name: "invalid random", unit: "invalid", valid: false},
		{name: "empty string", unit: "", valid: false},
		{name: "garbage string", unit: "random_string", valid: false},

		// Edge cases
		{name: "case sensitive SAMPLES", unit: "SAMPLES", valid: false},
		{name: "case sensitive Samples", unit: "Samples", valid: false},
		{name: "leading whitespace", unit: " samples", valid: false},
		{name: "trailing whitespace", unit: "samples ", valid: false},
		{name: "tab character", unit: "samples\t", valid: false},
		{name: "newline injection", unit: "samples\n", valid: false},
		{name: "special chars hyphen", unit: "lock-samples", valid: false},
		{name: "numeric only", unit: "123", valid: false},
		{name: "boolean true", unit: "true", valid: false},
		{name: "very long string", unit: string(make([]byte, 1024)), valid: false},
		{name: "unicode", unit: "échantillons", valid: false},
		{name: "just underscore", unit: "_", valid: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.valid, IsValidUnit(tt.unit))
		})
	}
}

func TestIsValidAggregationType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		aggType string
		valid   bool
	}{
		// Positive cases
		{name: "sum", aggType: string(SumAggregationType), valid: true},
		{name: "average", aggType: string(AverageAggregationType), valid: true},

		// Negative cases
		{name: "invalid random", aggType: "invalid", valid: false},
		{name: "empty string", aggType: "", valid: false},
		{name: "garbage string", aggType: "median", valid: false},

		// Edge cases
		{name: "case sensitive SUM", aggType: "SUM", valid: false},
		{name: "case sensitive Sum", aggType: "Sum", valid: false},
		{name: "leading whitespace", aggType: " sum", valid: false},
		{name: "trailing whitespace", aggType: "sum ", valid: false},
		{name: "newline injection", aggType: "sum\n", valid: false},
		{name: "hyphen variant", aggType: "sum-avg", valid: false},
		{name: "numeric", aggType: "0", valid: false},
		{name: "very long string", aggType: string(make([]byte, 2048)), valid: false},
		{name: "just underscore", aggType: "_", valid: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.valid, IsValidAggregationType(tt.aggType))
		})
	}
}

