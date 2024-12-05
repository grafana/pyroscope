package otlp

import (
	"testing"

	"github.com/stretchr/testify/assert"

	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	v1 "github.com/grafana/pyroscope/api/otlp/common/v1"
)

func TestGetServiceNameFromAttributes(t *testing.T) {
	tests := []struct {
		name     string
		attrs    []v1.KeyValue
		expected string
	}{
		{
			name:     "empty attributes",
			attrs:    []v1.KeyValue{},
			expected: "unknown",
		},
		{
			name: "service name present",
			attrs: []v1.KeyValue{
				{
					Key: "service.name",
					Value: v1.AnyValue{
						Value: &v1.AnyValue_StringValue{
							StringValue: "test-service",
						},
					},
				},
			},
			expected: "test-service",
		},
		{
			name: "service name empty",
			attrs: []v1.KeyValue{
				{
					Key: "service.name",
					Value: v1.AnyValue{
						Value: &v1.AnyValue_StringValue{
							StringValue: "",
						},
					},
				},
			},
			expected: "unknown",
		},
		{
			name: "service name among other attributes",
			attrs: []v1.KeyValue{
				{
					Key: "host.name",
					Value: v1.AnyValue{
						Value: &v1.AnyValue_StringValue{
							StringValue: "host1",
						},
					},
				},
				{
					Key: "service.name",
					Value: v1.AnyValue{
						Value: &v1.AnyValue_StringValue{
							StringValue: "test-service",
						},
					},
				},
			},
			expected: "test-service",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getServiceNameFromAttributes(tt.attrs)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestAppendAttributesUnique(t *testing.T) {
	tests := []struct {
		name          string
		existingAttrs []*typesv1.LabelPair
		newAttrs      []v1.KeyValue
		processedKeys map[string]bool
		expected      []*typesv1.LabelPair
	}{
		{
			name:          "empty attributes",
			existingAttrs: []*typesv1.LabelPair{},
			newAttrs:      []v1.KeyValue{},
			processedKeys: make(map[string]bool),
			expected:      []*typesv1.LabelPair{},
		},
		{
			name: "new unique attributes",
			existingAttrs: []*typesv1.LabelPair{
				{Name: "existing", Value: "value"},
			},
			newAttrs: []v1.KeyValue{
				{
					Key: "new",
					Value: v1.AnyValue{
						Value: &v1.AnyValue_StringValue{
							StringValue: "newvalue",
						},
					},
				},
			},
			processedKeys: map[string]bool{"existing": true},
			expected: []*typesv1.LabelPair{
				{Name: "existing", Value: "value"},
				{Name: "new", Value: "newvalue"},
			},
		},
		{
			name: "duplicate attributes",
			existingAttrs: []*typesv1.LabelPair{
				{Name: "key1", Value: "value1"},
			},
			newAttrs: []v1.KeyValue{
				{
					Key: "key1",
					Value: v1.AnyValue{
						Value: &v1.AnyValue_StringValue{
							StringValue: "value2",
						},
					},
				},
			},
			processedKeys: map[string]bool{"key1": true},
			expected: []*typesv1.LabelPair{
				{Name: "key1", Value: "value1"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := appendAttributesUnique(tt.existingAttrs, tt.newAttrs, tt.processedKeys)
			assert.Equal(t, tt.expected, result)
		})
	}
}
