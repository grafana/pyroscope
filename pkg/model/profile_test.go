package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	profilev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
)

func Test_extractMappingFilename(t *testing.T) {
	assert.Equal(t, "app", extractMappingFilename(`app`))
	assert.Equal(t, "app", extractMappingFilename(`./app`))
	assert.Equal(t, "app", extractMappingFilename(`/usr/bin/app`))
	assert.Equal(t, "app", extractMappingFilename(`../../../app`))
	assert.Equal(t, "app", extractMappingFilename(`/usr/bin/app\`))
	assert.Equal(t, "app", extractMappingFilename(`/usr/bin/app\\`))
	assert.Equal(t, "my awesome app", extractMappingFilename(`/usr/bin/my awesome app`))
	assert.Equal(t, "app", extractMappingFilename(`/usr/bin/my\ awesome\ app`))

	assert.Equal(t, "app.exe", extractMappingFilename(`C:\\app.exe`))
	assert.Equal(t, "app.exe", extractMappingFilename(`C:\\./app.exe`))
	assert.Equal(t, "app.exe", extractMappingFilename(`./app.exe`))
	assert.Equal(t, "app.exe", extractMappingFilename(`./../app.exe`))
	assert.Equal(t, "app.exe", extractMappingFilename(`C:\\build\app.exe`))
	assert.Equal(t, "My App.exe", extractMappingFilename(`C:\\build\My App.exe`))
	assert.Equal(t, "Not My App.exe", extractMappingFilename(`C:\\build\Not My App.exe`))
	assert.Equal(t, "app.exe", extractMappingFilename(`\\app.exe`))
	assert.Equal(t, "app.exe", extractMappingFilename(`\\build\app.exe`))

	assert.Equal(t, "bin", extractMappingFilename(`/usr/bin/`))
	assert.Equal(t, "build", extractMappingFilename(`\\build\`))

	assert.Equal(t, "", extractMappingFilename(""))
	assert.Equal(t, "", extractMappingFilename(`[vdso]`))
	assert.Equal(t, "", extractMappingFilename(`[vsyscall]`))
	assert.Equal(t, "", extractMappingFilename(`//anon`))
	assert.Equal(t, "not a path actually", extractMappingFilename(`not a path actually`))
}

func Test_symbolsPartitionKeyForProfile(t *testing.T) {
	tests := []struct {
		name           string
		partitionLabel string
		labels         Labels
		profile        *profilev1.Profile
		expected       string
	}{
		{
			partitionLabel: "",
			profile:        &profilev1.Profile{Mapping: []*profilev1.Mapping{}},
			expected:       "unknown",
		},
		{
			partitionLabel: "",
			profile:        &profilev1.Profile{Mapping: []*profilev1.Mapping{}},
			labels:         Labels{{Name: LabelNameServiceName, Value: "service_foo"}},
			expected:       "service_foo",
		},
		{
			partitionLabel: "",
			profile: &profilev1.Profile{
				Mapping:     []*profilev1.Mapping{{Filename: 1}},
				StringTable: []string{"", "filename"},
			},
			expected: "filename",
		},
		{
			partitionLabel: "partition",
			profile:        &profilev1.Profile{},
			labels:         Labels{{Name: "partition", Value: "partitionValue"}},
			expected:       "partitionValue",
		},
		{ // partition label is specified but not found: mapping is ignored.
			partitionLabel: "partition",
			profile: &profilev1.Profile{
				Mapping:     []*profilev1.Mapping{{Filename: 1}},
				StringTable: []string{"", "valid_filename"},
			},
			expected: "unknown",
		},
		{
			partitionLabel: "partition",
			profile: &profilev1.Profile{
				Mapping:     []*profilev1.Mapping{{Filename: 1}},
				StringTable: []string{"", "valid_filename"},
			},
			expected: "partitionValue",
			labels:   Labels{{Name: "partition", Value: "partitionValue"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := symbolsPartitionKeyForProfile(tt.labels, tt.partitionLabel, tt.profile)
			assert.Equal(t, tt.expected, actual)
		})
	}
}

func Test_ParseProfileTypeSelector(t *testing.T) {
	tests := []struct {
		Name    string
		ID      string
		Want    *typesv1.ProfileType
		WantErr string
	}{
		{
			Name: "block_contentions_count",
			ID:   "block:contentions:count:contentions:count",
			Want: &typesv1.ProfileType{
				Name:       "block",
				ID:         "block:contentions:count:contentions:count",
				SampleType: "contentions",
				SampleUnit: "count",
				PeriodType: "contentions",
				PeriodUnit: "count",
			},
		},
		{
			Name: "mutex_delay_nanoseconds",
			ID:   "mutex:delay:nanoseconds:contentions:count",
			Want: &typesv1.ProfileType{
				Name:       "mutex",
				ID:         "mutex:delay:nanoseconds:contentions:count",
				SampleType: "delay",
				SampleUnit: "nanoseconds",
				PeriodType: "contentions",
				PeriodUnit: "count",
			},
		},
		{
			Name: "memory_alloc_space_bytes",
			ID:   "memory:alloc_space:bytes:space:bytes",
			Want: &typesv1.ProfileType{
				Name:       "memory",
				ID:         "memory:alloc_space:bytes:space:bytes",
				SampleType: "alloc_space",
				SampleUnit: "bytes",
				PeriodType: "space",
				PeriodUnit: "bytes",
			},
		},
		{
			Name:    "too_few_parts",
			ID:      "memory:alloc_space:bytes:space",
			WantErr: `profile-type selection must be of the form <name>:<sample-type>:<sample-unit>:<period-type>:<period-unit>(:delta), got(4): "memory:alloc_space:bytes:space"`,
		},
		{
			Name:    "too_many_parts",
			ID:      "memory:alloc_space:bytes:space:bytes:extra:part",
			WantErr: `profile-type selection must be of the form <name>:<sample-type>:<sample-unit>:<period-type>:<period-unit>(:delta), got(7): "memory:alloc_space:bytes:space:bytes:extra:part"`,
		},
		{
			Name:    "empty_string",
			ID:      "",
			WantErr: `profile-type selection must be of the form <name>:<sample-type>:<sample-unit>:<period-type>:<period-unit>(:delta), got(1): ""`,
		},
		{
			Name: "valid_with_delta",
			ID:   "cpu:samples:count:cpu:nanoseconds:delta",
			Want: &typesv1.ProfileType{
				Name:       "cpu",
				ID:         "cpu:samples:count:cpu:nanoseconds:delta",
				SampleType: "samples",
				SampleUnit: "count",
				PeriodType: "cpu",
				PeriodUnit: "nanoseconds",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.Name, func(t *testing.T) {
			got, err := ParseProfileTypeSelector(tt.ID)
			if tt.WantErr != "" {
				require.Error(t, err)
				require.EqualError(t, err, tt.WantErr)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.Want, got)
			}
		})
	}
}

func Test_ParseProfileTypeSelector_ValidProfileTypes(t *testing.T) {
	// Shamelessly copied from: https://github.com/grafana/profiles-drilldown/blob/4e261a8744034bddefdec757d5d2e1d8dc0ec2bb/src/shared/infrastructure/profile-metrics/profile-metrics.json#L93
	var validProfileTypes = map[string]*typesv1.ProfileType{
		"block:contentions:count:contentions:count": {
			ID:         "block:contentions:count:contentions:count",
			Name:       "block",
			SampleType: "contentions",
			SampleUnit: "count",
			PeriodType: "contentions",
			PeriodUnit: "count",
		},
		"block:delay:nanoseconds:contentions:count": {
			ID:         "block:delay:nanoseconds:contentions:count",
			Name:       "block",
			SampleType: "delay",
			SampleUnit: "nanoseconds",
			PeriodType: "contentions",
			PeriodUnit: "count",
		},
		"goroutine:goroutine:count:goroutine:count": {
			ID:         "goroutine:goroutine:count:goroutine:count",
			Name:       "goroutine",
			SampleType: "goroutine",
			SampleUnit: "count",
			PeriodType: "goroutine",
			PeriodUnit: "count",
		},
		"goroutines:goroutine:count:goroutine:count": {
			ID:         "goroutines:goroutine:count:goroutine:count",
			Name:       "goroutines",
			SampleType: "goroutine",
			SampleUnit: "count",
			PeriodType: "goroutine",
			PeriodUnit: "count",
		},
		"memory:alloc_in_new_tlab_bytes:bytes::": {
			ID:         "memory:alloc_in_new_tlab_bytes:bytes::",
			Name:       "memory",
			SampleType: "alloc_in_new_tlab_bytes",
			SampleUnit: "bytes",
			PeriodType: "",
			PeriodUnit: "",
		},
		"memory:alloc_in_new_tlab_objects:count::": {
			ID:         "memory:alloc_in_new_tlab_objects:count::",
			Name:       "memory",
			SampleType: "alloc_in_new_tlab_objects",
			SampleUnit: "count",
			PeriodType: "",
			PeriodUnit: "",
		},
		"memory:alloc_objects:count:space:bytes": {
			ID:         "memory:alloc_objects:count:space:bytes",
			Name:       "memory",
			SampleType: "alloc_objects",
			SampleUnit: "count",
			PeriodType: "space",
			PeriodUnit: "bytes",
		},
		"memory:alloc_space:bytes:space:bytes": {
			ID:         "memory:alloc_space:bytes:space:bytes",
			Name:       "memory",
			SampleType: "alloc_space",
			SampleUnit: "bytes",
			PeriodType: "space",
			PeriodUnit: "bytes",
		},
		"memory:inuse_objects:count:space:bytes": {
			ID:         "memory:inuse_objects:count:space:bytes",
			Name:       "memory",
			SampleType: "inuse_objects",
			SampleUnit: "count",
			PeriodType: "space",
			PeriodUnit: "bytes",
		},
		"memory:inuse_space:bytes:space:bytes": {
			ID:         "memory:inuse_space:bytes:space:bytes",
			Name:       "memory",
			SampleType: "inuse_space",
			SampleUnit: "bytes",
			PeriodType: "space",
			PeriodUnit: "bytes",
		},
		"mutex:contentions:count:contentions:count": {
			ID:         "mutex:contentions:count:contentions:count",
			Name:       "mutex",
			SampleType: "contentions",
			SampleUnit: "count",
			PeriodType: "contentions",
			PeriodUnit: "count",
		},
		"mutex:delay:nanoseconds:contentions:count": {
			ID:         "mutex:delay:nanoseconds:contentions:count",
			Name:       "mutex",
			SampleType: "delay",
			SampleUnit: "nanoseconds",
			PeriodType: "contentions",
			PeriodUnit: "count",
		},
		"process_cpu:alloc_samples:count:cpu:nanoseconds": {
			ID:         "process_cpu:alloc_samples:count:cpu:nanoseconds",
			Name:       "process_cpu",
			SampleType: "alloc_samples",
			SampleUnit: "count",
			PeriodType: "cpu",
			PeriodUnit: "nanoseconds",
		},
		"process_cpu:alloc_size:bytes:cpu:nanoseconds": {
			ID:         "process_cpu:alloc_size:bytes:cpu:nanoseconds",
			Name:       "process_cpu",
			SampleType: "alloc_size",
			SampleUnit: "bytes",
			PeriodType: "cpu",
			PeriodUnit: "nanoseconds",
		},
		"process_cpu:cpu:nanoseconds:cpu:nanoseconds": {
			ID:         "process_cpu:cpu:nanoseconds:cpu:nanoseconds",
			Name:       "process_cpu",
			SampleType: "cpu",
			SampleUnit: "nanoseconds",
			PeriodType: "cpu",
			PeriodUnit: "nanoseconds",
		},
		"process_cpu:exception:count:cpu:nanoseconds": {
			ID:         "process_cpu:exception:count:cpu:nanoseconds",
			Name:       "process_cpu",
			SampleType: "exception",
			SampleUnit: "count",
			PeriodType: "cpu",
			PeriodUnit: "nanoseconds",
		},
		"process_cpu:lock_count:count:cpu:nanoseconds": {
			ID:         "process_cpu:lock_count:count:cpu:nanoseconds",
			Name:       "process_cpu",
			SampleType: "lock_count",
			SampleUnit: "count",
			PeriodType: "cpu",
			PeriodUnit: "nanoseconds",
		},
		"process_cpu:lock_time:nanoseconds:cpu:nanoseconds": {
			ID:         "process_cpu:lock_time:nanoseconds:cpu:nanoseconds",
			Name:       "process_cpu",
			SampleType: "lock_time",
			SampleUnit: "nanoseconds",
			PeriodType: "cpu",
			PeriodUnit: "nanoseconds",
		},
		"process_cpu:samples:count::milliseconds": {
			ID:         "process_cpu:samples:count::milliseconds",
			Name:       "process_cpu",
			SampleType: "samples",
			SampleUnit: "count",
			PeriodType: "",
			PeriodUnit: "milliseconds",
		},
		"process_cpu:samples:count:cpu:nanoseconds": {
			ID:         "process_cpu:samples:count:cpu:nanoseconds",
			Name:       "process_cpu",
			SampleType: "samples",
			SampleUnit: "count",
			PeriodType: "cpu",
			PeriodUnit: "nanoseconds",
		},
	}

	for id, want := range validProfileTypes {
		t.Run(id, func(t *testing.T) {
			got, err := ParseProfileTypeSelector(id)
			require.NoError(t, err)
			require.Equal(t, want, got)
		})
	}
}
