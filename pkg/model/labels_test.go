package model

import (
	"math/rand"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"

	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
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

func Test_LabelsBuilder_Unique(t *testing.T) {
	tests := []struct {
		name     string
		input    Labels
		add      Labels
		expected Labels
	}{
		{
			name: "duplicates in input are preserved",
			input: Labels{
				{Name: "unique", Value: "yes"},
				{Name: "unique", Value: "no"},
			},
			add: Labels{
				{Name: "foo", Value: "bar"},
				{Name: "foo", Value: "baz"},
			},
			expected: Labels{
				{Name: "foo", Value: "baz"},
				{Name: "unique", Value: "yes"},
				{Name: "unique", Value: "no"},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			builder := NewLabelsBuilder(test.input)
			for _, l := range test.add {
				builder.Set(l.Name, l.Value)
			}
			assert.Equal(t, test.expected, builder.Labels())
		})
	}
}

func TestLabels_SessionID_Order(t *testing.T) {
	input := []Labels{
		{
			{Name: LabelNameSessionID, Value: "session-a"},
			{Name: LabelNameProfileType, Value: "cpu"},
			{Name: LabelNameServiceNamePrivate, Value: "service-name"},
		}, {
			{Name: LabelNameSessionID, Value: "session-b"},
			{Name: LabelNameProfileType, Value: "cpu"},
			{Name: LabelNameServiceNamePrivate, Value: "service-name"},
		},
	}

	for _, x := range input {
		sort.Sort(LabelsEnforcedOrder(x))
	}
	sort.Slice(input, func(i, j int) bool {
		return CompareLabelPairs(input[i], input[j]) < 0
	})

	expectedOrder := []Labels{
		{
			{Name: LabelNameProfileType, Value: "cpu"},
			{Name: LabelNameServiceNamePrivate, Value: "service-name"},
			{Name: LabelNameSessionID, Value: "session-a"},
		}, {
			{Name: LabelNameProfileType, Value: "cpu"},
			{Name: LabelNameServiceNamePrivate, Value: "service-name"},
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

func TestLabels_LabelsEnforcedOrder(t *testing.T) {
	labels := []*typesv1.LabelPair{
		{Name: "foo", Value: "bar"},
		{Name: LabelNameProfileType, Value: "cpu"},
		{Name: "__request_id__", Value: "mess"},
		{Name: LabelNameServiceNamePrivate, Value: "service"},
		{Name: "Alarm", Value: "Order"},
	}

	expected := Labels{
		{Name: LabelNameProfileType, Value: "cpu"},
		{Name: LabelNameServiceNamePrivate, Value: "service"},
		{Name: "Alarm", Value: "Order"},
		{Name: "__request_id__", Value: "mess"},
		{Name: "foo", Value: "bar"},
	}

	permute(labels, func(x []*typesv1.LabelPair) {
		sort.Sort(LabelsEnforcedOrder(x))
		assert.Equal(t, LabelPairsString(expected), LabelPairsString(labels))
	})
}

func TestLabels_LabelsEnforcedOrder_BytesWithLabels(t *testing.T) {
	labels := Labels{
		{Name: LabelNameProfileType, Value: "cpu"},
		{Name: LabelNameServiceNamePrivate, Value: "service"},
		{Name: "__request_id__", Value: "mess"},
		{Name: "A_label", Value: "bad"},
		{Name: "foo", Value: "bar"},
	}
	sort.Sort(LabelsEnforcedOrder(labels))

	assert.NotEqual(t,
		labels.BytesWithLabels(nil, "A_label"),
		labels.BytesWithLabels(nil, "not_a_label"),
	)

	assert.Equal(t,
		labels.BytesWithLabels(nil, "A_label"),
		Labels{{Name: "A_label", Value: "bad"}}.BytesWithLabels(nil, "A_label"),
	)
}

func permute[T any](s []T, f func([]T)) {
	n := len(s)
	stack := make([]int, n)
	f(s)
	i := 0
	for i < n {
		if stack[i] < i {
			if i%2 == 0 {
				s[0], s[i] = s[i], s[0]
			} else {
				s[stack[i]], s[i] = s[i], s[stack[i]]
			}
			f(s)
			stack[i]++
			i = 0
		} else {
			stack[i] = 0
			i++
		}
	}
}

func TestInsert(t *testing.T) {
	tests := []struct {
		name        string
		labels      Labels
		insertName  string
		insertValue string
		expected    Labels
	}{
		{
			name:        "Insert into empty slice",
			labels:      Labels{},
			insertName:  "foo",
			insertValue: "bar",
			expected: Labels{
				{Name: "foo", Value: "bar"},
			},
		},
		{
			name: "Insert at the beginning",
			labels: Labels{
				{Name: "baz", Value: "qux"},
				{Name: "quux", Value: "corge"},
			},
			insertName:  "alice",
			insertValue: "bob",
			expected: Labels{
				{Name: "alice", Value: "bob"},
				{Name: "baz", Value: "qux"},
				{Name: "quux", Value: "corge"},
			},
		},
		{
			name: "Insert in the middle",
			labels: Labels{
				{Name: "baz", Value: "qux"},
				{Name: "quux", Value: "corge"},
			},
			insertName:  "foo",
			insertValue: "bar",
			expected: Labels{
				{Name: "baz", Value: "qux"},
				{Name: "foo", Value: "bar"},
				{Name: "quux", Value: "corge"},
			},
		},
		{
			name: "Insert at the end",
			labels: Labels{
				{Name: "baz", Value: "qux"},
				{Name: "quux", Value: "corge"},
			},
			insertName:  "xyz",
			insertValue: "123",
			expected: Labels{
				{Name: "baz", Value: "qux"},
				{Name: "quux", Value: "corge"},
				{Name: "xyz", Value: "123"},
			},
		},
		{
			name: "Update existing label",
			labels: Labels{
				{Name: "baz", Value: "qux"},
				{Name: "quux", Value: "corge"},
			},
			insertName:  "baz",
			insertValue: "updated_value",
			expected: Labels{
				{Name: "baz", Value: "updated_value"},
				{Name: "quux", Value: "corge"},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.expected, test.labels.InsertSorted(test.insertName, test.insertValue))
		})
	}
}

func Test_ServiceVersionFromLabels(t *testing.T) {
	tests := []struct {
		name            string
		input           Labels
		expectedVersion ServiceVersion
		expectedOk      bool
	}{
		{
			name: "all present",
			input: Labels{
				{Name: LabelNameServiceRepository, Value: "repo"},
				{Name: LabelNameServiceGitRef, Value: "ref"},
				{Name: LabelNameServiceRootPath, Value: "some-path"},
				{Name: "any-other-label", Value: "any-value"},
			},
			expectedVersion: ServiceVersion{
				Repository: "repo",
				GitRef:     "ref",
				RootPath:   "some-path",
			},
			expectedOk: true,
		},
		{
			name: "some present",
			input: Labels{
				{Name: LabelNameServiceRepository, Value: "repo"},
				{Name: LabelNameServiceRootPath, Value: "some-path"},
				{Name: "any-other-label", Value: "any-value"},
			},
			expectedVersion: ServiceVersion{
				Repository: "repo",
				GitRef:     "",
				RootPath:   "some-path",
			},
			expectedOk: true,
		},
		{
			name: "none present",
			input: Labels{
				{Name: "any-label", Value: "any-value"},
			},
			expectedVersion: ServiceVersion{
				Repository: "",
				GitRef:     "",
				RootPath:   "",
			},
			expectedOk: false,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			version, ok := ServiceVersionFromLabels(test.input)
			assert.Equal(t, test.expectedVersion, version)
			assert.Equal(t, test.expectedOk, ok)
		})
	}
}

func TestLabels_Subtract(t *testing.T) {
	tests := []struct {
		name     string
		labels   Labels
		input    Labels
		expected Labels
	}{
		{
			name:     "Empty sets",
			labels:   Labels{},
			input:    Labels{},
			expected: Labels{},
		},
		{
			name:   "Subtract from empty set",
			labels: Labels{},
			input: Labels{
				{Name: "foo", Value: "bar"},
				{Name: "service", Value: "api"},
			},
			expected: Labels{},
		},
		{
			name: "Subtract empty set",
			labels: Labels{
				{Name: "foo", Value: "bar"},
				{Name: "service", Value: "api"},
			},
			input: Labels{},
			expected: Labels{
				{Name: "foo", Value: "bar"},
				{Name: "service", Value: "api"},
			},
		},
		{
			name: "No overlap",
			labels: Labels{
				{Name: "app", Value: "frontend"},
				{Name: "env", Value: "prod"},
			},
			input: Labels{
				{Name: "service", Value: "api"},
				{Name: "version", Value: "1.0"},
			},
			expected: Labels{
				{Name: "app", Value: "frontend"},
				{Name: "env", Value: "prod"},
			},
		},
		{
			name: "Complete overlap",
			labels: Labels{
				{Name: "app", Value: "frontend"},
				{Name: "env", Value: "prod"},
			},
			input: Labels{
				{Name: "app", Value: "frontend"},
				{Name: "env", Value: "prod"},
			},
			expected: Labels{},
		},
		{
			name: "Partial overlap",
			labels: Labels{
				{Name: "app", Value: "frontend"},
				{Name: "env", Value: "prod"},
				{Name: "version", Value: "2.0"},
			},
			input: Labels{
				{Name: "env", Value: "prod"},
				{Name: "service", Value: "api"},
			},
			expected: Labels{
				{Name: "app", Value: "frontend"},
				{Name: "version", Value: "2.0"},
			},
		},
		{
			name: "Same name different value",
			labels: Labels{
				{Name: "env", Value: "prod"},
				{Name: "service", Value: "frontend"},
			},
			input: Labels{
				{Name: "env", Value: "dev"},
				{Name: "service", Value: "api"},
			},
			expected: Labels{
				{Name: "env", Value: "prod"},
				{Name: "service", Value: "frontend"},
			},
		},
		{
			name: "Single element sets",
			labels: Labels{
				{Name: "foo", Value: "bar"},
			},
			input: Labels{
				{Name: "foo", Value: "bar"},
			},
			expected: Labels{},
		},
		{
			name: "Larger set with multiple removals",
			labels: Labels{
				{Name: "a", Value: "1"},
				{Name: "b", Value: "2"},
				{Name: "c", Value: "3"},
				{Name: "d", Value: "4"},
				{Name: "e", Value: "5"},
			},
			input: Labels{
				{Name: "b", Value: "2"},
				{Name: "d", Value: "4"},
				{Name: "f", Value: "6"},
			},
			expected: Labels{
				{Name: "a", Value: "1"},
				{Name: "c", Value: "3"},
				{Name: "e", Value: "5"},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.expected, test.labels.Subtract(test.input))
		})
	}
}

func TestLabels_Intersect(t *testing.T) {
	tests := []struct {
		name     string
		labels   Labels
		input    Labels
		expected Labels
	}{
		{
			name:     "Empty sets",
			labels:   Labels{},
			input:    Labels{},
			expected: Labels{},
		},
		{
			name: "Intersect with empty set",
			labels: Labels{
				{Name: "foo", Value: "bar"},
				{Name: "service", Value: "api"},
			},
			input:    Labels{},
			expected: Labels{},
		},
		{
			name:   "Intersect empty set",
			labels: Labels{},
			input: Labels{
				{Name: "foo", Value: "bar"},
				{Name: "service", Value: "api"},
			},
			expected: Labels{},
		},
		{
			name: "No overlap",
			labels: Labels{
				{Name: "app", Value: "frontend"},
				{Name: "env", Value: "prod"},
			},
			input: Labels{
				{Name: "service", Value: "api"},
				{Name: "version", Value: "1.0"},
			},
			expected: Labels{},
		},
		{
			name: "Complete overlap",
			labels: Labels{
				{Name: "app", Value: "frontend"},
				{Name: "env", Value: "prod"},
			},
			input: Labels{
				{Name: "app", Value: "frontend"},
				{Name: "env", Value: "prod"},
			},
			expected: Labels{
				{Name: "app", Value: "frontend"},
				{Name: "env", Value: "prod"},
			},
		},
		{
			name: "Partial overlap",
			labels: Labels{
				{Name: "app", Value: "frontend"},
				{Name: "env", Value: "prod"},
				{Name: "version", Value: "2.0"},
			},
			input: Labels{
				{Name: "env", Value: "prod"},
				{Name: "service", Value: "api"},
			},
			expected: Labels{
				{Name: "env", Value: "prod"},
			},
		},
		{
			name: "Same name different value",
			labels: Labels{
				{Name: "env", Value: "prod"},
				{Name: "service", Value: "frontend"},
			},
			input: Labels{
				{Name: "env", Value: "dev"},
				{Name: "service", Value: "api"},
			},
			expected: Labels{},
		},
		{
			name: "Single element intersection",
			labels: Labels{
				{Name: "foo", Value: "bar"},
			},
			input: Labels{
				{Name: "foo", Value: "bar"},
			},
			expected: Labels{
				{Name: "foo", Value: "bar"},
			},
		},
		{
			name: "Multiple intersections",
			labels: Labels{
				{Name: "a", Value: "1"},
				{Name: "b", Value: "2"},
				{Name: "c", Value: "3"},
				{Name: "d", Value: "4"},
				{Name: "e", Value: "5"},
			},
			input: Labels{
				{Name: "b", Value: "2"},
				{Name: "c", Value: "3"},
				{Name: "f", Value: "6"},
				{Name: "g", Value: "7"},
			},
			expected: Labels{
				{Name: "b", Value: "2"},
				{Name: "c", Value: "3"},
			},
		},
		{
			name: "First set is subset of second",
			labels: Labels{
				{Name: "env", Value: "prod"},
				{Name: "version", Value: "1.0"},
			},
			input: Labels{
				{Name: "app", Value: "frontend"},
				{Name: "env", Value: "prod"},
				{Name: "service", Value: "api"},
				{Name: "version", Value: "1.0"},
			},
			expected: Labels{
				{Name: "env", Value: "prod"},
				{Name: "version", Value: "1.0"},
			},
		},
		{
			name: "Second set is subset of first",
			labels: Labels{
				{Name: "app", Value: "frontend"},
				{Name: "env", Value: "prod"},
				{Name: "service", Value: "api"},
				{Name: "version", Value: "1.0"},
			},
			input: Labels{
				{Name: "env", Value: "prod"},
				{Name: "version", Value: "1.0"},
			},
			expected: Labels{
				{Name: "env", Value: "prod"},
				{Name: "version", Value: "1.0"},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.expected, test.labels.Intersect(test.input))
		})
	}
}

func TestLabels_IntersectAll(t *testing.T) {
	tests := []struct {
		name      string
		labelSets []Labels
		expected  Labels
	}{
		{
			name:      "Empty input",
			labelSets: []Labels{},
			expected:  nil,
		},
		{
			name: "Single label set",
			labelSets: []Labels{
				{
					{Name: "env", Value: "prod"},
					{Name: "service", Value: "api"},
				},
			},
			expected: Labels{
				{Name: "env", Value: "prod"},
				{Name: "service", Value: "api"},
			},
		},
		{
			name: "Two sets with full overlap",
			labelSets: []Labels{
				{
					{Name: "env", Value: "prod"},
					{Name: "service", Value: "api"},
				},
				{
					{Name: "env", Value: "prod"},
					{Name: "service", Value: "api"},
				},
			},
			expected: Labels{
				{Name: "env", Value: "prod"},
				{Name: "service", Value: "api"},
			},
		},
		{
			name: "Two sets with partial overlap",
			labelSets: []Labels{
				{
					{Name: "env", Value: "prod"},
					{Name: "pod", Value: "pod-1"},
					{Name: "region", Value: "us-east"},
				},
				{
					{Name: "env", Value: "prod"},
					{Name: "pod", Value: "pod-2"},
					{Name: "region", Value: "us-east"},
				},
			},
			expected: Labels{
				{Name: "env", Value: "prod"},
				{Name: "region", Value: "us-east"},
			},
		},
		{
			name: "No common labels",
			labelSets: []Labels{
				{
					{Name: "env", Value: "prod"},
				},
				{
					{Name: "region", Value: "us-east"},
				},
			},
			expected: nil,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := IntersectAll(test.labelSets)
			assert.Equal(t, test.expected, result)
		})
	}
}

func TestLabels_WithoutLabels(t *testing.T) {
	tests := []struct {
		name     string
		labels   Labels
		remove   []string
		expected Labels
	}{
		{
			name: "Remove nothing",
			labels: Labels{
				{Name: "env", Value: "prod"},
				{Name: "service", Value: "api"},
			},
			remove: []string{},
			expected: Labels{
				{Name: "env", Value: "prod"},
				{Name: "service", Value: "api"},
			},
		},
		{
			name: "Remove single label",
			labels: Labels{
				{Name: "env", Value: "prod"},
				{Name: "pod", Value: "pod-1"},
				{Name: "service", Value: "api"},
			},
			remove: []string{"pod"},
			expected: Labels{
				{Name: "env", Value: "prod"},
				{Name: "service", Value: "api"},
			},
		},
		{
			name: "Remove multiple labels",
			labels: Labels{
				{Name: "env", Value: "prod"},
				{Name: "pod", Value: "pod-1"},
				{Name: "region", Value: "us-east"},
				{Name: "service", Value: "api"},
			},
			remove: []string{"pod", "region"},
			expected: Labels{
				{Name: "env", Value: "prod"},
				{Name: "service", Value: "api"},
			},
		},
		{
			name: "Remove non-existent label",
			labels: Labels{
				{Name: "env", Value: "prod"},
				{Name: "service", Value: "api"},
			},
			remove: []string{"nonexistent"},
			expected: Labels{
				{Name: "env", Value: "prod"},
				{Name: "service", Value: "api"},
			},
		},
		{
			name: "Remove all labels",
			labels: Labels{
				{Name: "env", Value: "prod"},
				{Name: "service", Value: "api"},
			},
			remove:   []string{"env", "service"},
			expected: Labels{},
		},
		{
			name:     "Empty labels",
			labels:   Labels{},
			remove:   []string{"env"},
			expected: Labels{},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := test.labels.WithoutLabels(test.remove...)
			assert.Equal(t, test.expected, result)
		})
	}
}
