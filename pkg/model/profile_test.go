package model

import (
	"testing"

	"github.com/stretchr/testify/assert"

	profilev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
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
