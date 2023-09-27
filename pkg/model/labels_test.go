package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLabelsUnique(t *testing.T) {
	tests := []struct {
		name     string
		input    Labels
		expected Labels
	}{
		{
			name:     "Empty List",
			input:    Labels{},
			expected: Labels{},
		},
		{
			name: "List with One Label",
			input: Labels{
				{Name: "Name1", Value: "Value1"},
			},
			expected: Labels{
				{Name: "Name1", Value: "Value1"},
			},
		},
		{
			name: "List with Duplicate Labels",
			input: Labels{
				{Name: "Name1", Value: "Value1"},
				{Name: "Name1", Value: "Value2"},
				{Name: "Name2", Value: "Value3"},
				{Name: "Name3", Value: "Value4"},
				{Name: "Name3", Value: "Value5"},
			},
			expected: Labels{
				{Name: "Name1", Value: "Value1"},
				{Name: "Name2", Value: "Value3"},
				{Name: "Name3", Value: "Value4"},
			},
		},
		{
			name: "List with No Duplicate Labels",
			input: Labels{
				{Name: "Name1", Value: "Value1"},
				{Name: "Name2", Value: "Value2"},
				{Name: "Name3", Value: "Value3"},
			},
			expected: Labels{
				{Name: "Name1", Value: "Value1"},
				{Name: "Name2", Value: "Value2"},
				{Name: "Name3", Value: "Value3"},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := test.input.Unique()
			assert.Equal(t, test.expected, result)
		})
	}
}
