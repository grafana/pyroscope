package model

import (
	"math/rand"
	"sort"
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

func TestLabels_SessionID_Order(t *testing.T) {
	const serviceNameLabel = "__service_name__"
	input := []Labels{
		{
			{Name: LabelNameSessionID, Value: "session-a"},
			{Name: LabelNameProfileType, Value: "cpu"},
			{Name: serviceNameLabel, Value: "service-name"},
		}, {
			{Name: LabelNameSessionID, Value: "session-b"},
			{Name: LabelNameProfileType, Value: "cpu"},
			{Name: serviceNameLabel, Value: "service-name"},
		},
	}

	for _, x := range input {
		sort.Sort(x)
	}
	sort.Slice(input, func(i, j int) bool {
		return CompareLabelPairs(input[i], input[j]) < 0
	})

	expectedOrder := []Labels{
		{
			{Name: LabelNameProfileType, Value: "cpu"},
			{Name: serviceNameLabel, Value: "service-name"},
			{Name: LabelNameSessionID, Value: "session-a"},
		}, {
			{Name: LabelNameProfileType, Value: "cpu"},
			{Name: serviceNameLabel, Value: "service-name"},
			{Name: LabelNameSessionID, Value: "session-b"},
		},
	}

	assert.Equal(t, expectedOrder, input)
}

func TestLabels_IsAllowedForIngestion(t *testing.T) {
	type testCase struct {
		labelName string
		allowed   bool
	}

	testCases := []testCase{
		{labelName: LabelNameSessionID, allowed: true},
		{labelName: "some_label", allowed: true},
		{labelName: LabelNameProfileType},
	}

	for _, tc := range testCases {
		allowed := IsLabelAllowedForIngestion(tc.labelName)
		assert.Equalf(t, tc.allowed, allowed, "%q", tc.labelName)
	}
}

func Test_SessionID_Parse(t *testing.T) {
	n := rand.Uint64()
	s := SessionID(n)
	p, err := ParseSessionID(s.String())
	assert.NoError(t, err)
	assert.Equal(t, SessionID(n), p)

	_, err = ParseSessionID("not-a-session-id") // Matches expected length.
	assert.NotNil(t, err)

	_, err = ParseSessionID("not-a-session-id-either")
	assert.NotNil(t, err)
}
